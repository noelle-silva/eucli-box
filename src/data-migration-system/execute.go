package datamigration

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	datastorage "eucli-box/src/data-storage-system"

	"eucli-box/pkg/release"
)

// Session 持有一次启动的迁移会话：版本事实、过程记录与恢复资料。
type Session struct {
	dataDir   string
	w         workspace
	target    string
	outcome   Outcome
	process   *processRecord
	steps     []Step
	stepIDs   []string
	scope     []string
	completed bool
}

// Prepare 检查数据现状并准备迁移；存在未完成过程记录时先执行恢复。
func Prepare(ctx context.Context, dataDir string, targetVersion string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := release.ValidateVersion(targetVersion); err != nil {
		return nil, migrationInvalid("target data version is invalid", err)
	}
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, migrationPrepareFailed("failed to resolve data directory", err)
	}
	w, err := newWorkspace(absolute)
	if err != nil {
		return nil, err
	}
	if err := w.ensure(); err != nil {
		return nil, err
	}
	process, exists, err := readProcessRecord(w)
	if err != nil {
		return nil, migrationRecoveryFailed("unfinished migration record is unreadable", err)
	}
	if exists {
		return recoverUnfinishedMigration(ctx, absolute, w, process, targetVersion)
	}
	version, versionExists, err := datastorage.ReadStorageVersion(ctx, absolute)
	if err != nil {
		return nil, migrationPrepareFailed("current data version cannot be read", err)
	}
	if !versionExists {
		now := time.Now().UTC()
		if err := datastorage.WriteStorageVersion(ctx, absolute, datastorage.StorageVersion{Version: targetVersion, CreatedAt: now, UpdatedAt: now}); err != nil {
			return nil, migrationPrepareFailed("failed to write initial data version", err)
		}
		session := &Session{dataDir: absolute, w: w, target: targetVersion, outcome: Outcome{
			State:  StateDataUnchanged,
			From:   targetVersion,
			To:     targetVersion,
			Detail: "首次安装，已写入目标数据版本",
		}}
		if err := session.writeStatus(ctx, targetVersion, false); err != nil {
			return nil, err
		}
		return session, nil
	}
	comparison, err := release.CompareVersions(version.Version, targetVersion)
	if err != nil {
		return nil, migrationPrepareFailed("current data version cannot be compared", err)
	}
	if comparison > 0 {
		return nil, migrationVersionTooHigh(fmt.Sprintf("data version %s is higher than program target %s", version.Version, targetVersion), nil)
	}
	if comparison == 0 {
		session := &Session{dataDir: absolute, w: w, target: targetVersion, outcome: Outcome{
			State:  StateDataUnchanged,
			From:   version.Version,
			To:     targetVersion,
			Detail: "数据版本已经是目标版本，未发生数据变化",
		}}
		if err := session.writeStatus(ctx, version.Version, false); err != nil {
			return nil, err
		}
		return session, nil
	}
	chain, err := buildChain(version.Version, targetVersion)
	if err != nil {
		return nil, err
	}
	scope := backupScope()
	runID := time.Now().UTC().Format("20060102T150405.000000000Z")
	if _, err := establishBackup(ctx, absolute, w, runID, scope); err != nil {
		return nil, err
	}
	stepIDs := make([]string, 0, len(chain))
	for _, step := range chain {
		stepIDs = append(stepIDs, step.ID)
	}
	record := newProcessRecord(version.Version, targetVersion, stepIDs, processBackupInfo{
		RunID:    runID,
		Manifest: filepath.ToSlash(filepath.Join("backup", "run-"+runID, "manifest.json")),
		Verified: true,
	})
	if err := writeProcessRecord(w, record); err != nil {
		return nil, migrationPrepareFailed("failed to persist migration process record", err)
	}
	return &Session{
		dataDir: absolute,
		w:       w,
		target:  targetVersion,
		outcome: Outcome{State: StateMigrated, From: version.Version, To: targetVersion, Detail: "迁移等待执行"},
		process: &record,
		steps:   chain,
		stepIDs: stepIDs,
		scope:   scope,
	}, nil
}

// recoverUnfinishedMigration 处理上次启动遗留的未完成迁移：先恢复，再以旧数据继续启动。
func recoverUnfinishedMigration(ctx context.Context, dataDir string, w workspace, process processRecord, targetVersion string) (*Session, error) {
	scope, err := backupScopeFor(process.StepIDs)
	if err != nil {
		return nil, migrationRecoveryFailed("cannot determine recovery scope from unfinished migration", err)
	}
	if err := restoreFromBackup(ctx, dataDir, w, process.Backup.RunID, scope); err != nil {
		session := &Session{
			dataDir: dataDir,
			w:       w,
			target:  targetVersion,
			outcome: Outcome{State: StateRecoveryFailed, From: process.FromVersion, To: targetVersion, Detail: "数据恢复失败，现场完整保留，需要人工处理"},
			process: &process,
			stepIDs: process.StepIDs,
			scope:   scope,
		}
		if statusErr := session.writeStatus(ctx, currentDataVersionFromStorage(ctx, dataDir, process.FromVersion), false); statusErr != nil {
			return nil, migrationRecoveryFailed("data recovery failed and status record could not be written", err)
		}
		return nil, migrationRecoveryFailed("data recovery failed, scene is fully retained", err)
	}
	if err := deleteProcessRecord(w); err != nil {
		return nil, migrationRecoveryFailed("data recovered but process record could not be removed", err)
	}
	if err := removeBackupRun(w, process.Backup.RunID); err != nil {
		return nil, migrationRecoveryFailed("data recovered but backup could not be cleaned", err)
	}
	session := &Session{
		dataDir: dataDir,
		w:       w,
		target:  targetVersion,
		outcome: Outcome{State: StateRecovered, From: process.FromVersion, To: targetVersion, Detail: "未完成迁移已恢复，数据回到迁移开始前"},
		stepIDs: process.StepIDs,
		scope:   scope,
	}
	if err := session.writeStatus(ctx, process.FromVersion, false); err != nil {
		return nil, err
	}
	return session, nil
}

// currentDataVersionFromStorage 尝试读取当前版本事实；读取失败时退回记录版本。
func currentDataVersionFromStorage(ctx context.Context, dataDir string, fallback string) string {
	version, exists, err := datastorage.ReadStorageVersion(ctx, dataDir)
	if err != nil || !exists {
		return fallback
	}
	return version.Version
}

// buildChain 从当前版本沿已登记步骤构造到目标版本的连续迁移链。
func buildChain(from string, target string) ([]Step, error) {
	steps := registeredSteps()
	chain := make([]Step, 0)
	current := from
	for current != target {
		comparison, err := release.CompareVersions(current, target)
		if err != nil {
			return nil, migrationStepMissing("chain versions cannot be compared", err)
		}
		if comparison > 0 {
			return nil, migrationStepMissing(fmt.Sprintf("no migration step registered from %s to %s", from, target), nil)
		}
		found := false
		for _, step := range steps {
			if step.FromVersion == current {
				chain = append(chain, step)
				current = step.ToVersion
				found = true
				break
			}
		}
		if !found {
			return nil, migrationStepMissing(fmt.Sprintf("no migration step registered from data version %s", current), nil)
		}
	}
	return chain, nil
}

// Outcome 返回本次启动的四态结果。
func (s *Session) Outcome() Outcome {
	return s.outcome
}

// Run 沿连续链执行迁移；任一级失败或核对失败自动恢复。
func (s *Session) Run(ctx context.Context) error {
	if s.process == nil || len(s.steps) == 0 {
		return nil
	}
	for index := s.process.CurrentIndex; index < len(s.steps); index++ {
		step := s.steps[index]
		if err := step.Precheck(ctx, s.dataDir); err != nil {
			return s.autoRecover(ctx, migrationStepFailed(fmt.Sprintf("step %s precheck failed", step.ID), err), step)
		}
		if err := step.Apply(ctx, s.dataDir); err != nil {
			return s.autoRecover(ctx, migrationStepFailed(fmt.Sprintf("step %s apply failed", step.ID), err), step)
		}
		if err := step.Verify(ctx, s.dataDir); err != nil {
			return s.autoRecover(ctx, migrationVerifyFailed(fmt.Sprintf("step %s verify failed", step.ID), err), step)
		}
		now := time.Now().UTC()
		nextVersion := datastorage.StorageVersion{Version: step.ToVersion, CreatedAt: now, UpdatedAt: now}
		if existing, exists, readErr := datastorage.ReadStorageVersion(ctx, s.dataDir); readErr == nil && exists && !existing.CreatedAt.IsZero() {
			nextVersion.CreatedAt = existing.CreatedAt
		}
		if err := datastorage.WriteStorageVersion(ctx, s.dataDir, nextVersion); err != nil {
			return s.autoRecover(ctx, migrationStepFailed(fmt.Sprintf("step %s version write failed", step.ID), err), step)
		}
		checkedAt := time.Now().UTC().Format(time.RFC3339Nano)
		s.process.appendVerifiedStep(step, checkedAt)
		if err := writeProcessRecord(s.w, *s.process); err != nil {
			return s.autoRecover(ctx, migrationStepFailed(fmt.Sprintf("step %s process record write failed", step.ID), err), step)
		}
	}
	s.outcome = Outcome{State: StateMigrated, From: s.process.FromVersion, To: s.target, Detail: "每级迁移均完成核对"}
	return nil
}

// autoRecover 恢复数据到迁移开始前；恢复成功后 Run 返回 nil，业务端以旧数据继续启动。
func (s *Session) autoRecover(ctx context.Context, cause error, step Step) error {
	recovered, err := s.tryRecover(ctx)
	if err != nil {
		return err
	}
	if recovered {
		return nil
	}
	return migrationRecoveryFailed(fmt.Sprintf("step %s failed and data recovery failed", step.ID), cause)
}

// Recover 把数据恢复到迁移开始前；未发生过迁移的会话是空操作。
func (s *Session) Recover(ctx context.Context) error {
	if s.process == nil {
		return nil
	}
	recovered, err := s.tryRecover(ctx)
	if err != nil {
		return err
	}
	if recovered {
		return nil
	}
	return migrationRecoveryFailed("data recovery failed, scene is fully retained", nil)
}

// tryRecover 执行恢复、写状态并清理；同步会话四态结果，返回恢复是否成功。
func (s *Session) tryRecover(ctx context.Context) (bool, error) {
	if s.process == nil {
		return true, nil
	}
	fromVersion := s.process.FromVersion
	scope := s.scope
	if len(scope) == 0 {
		var err error
		scope, err = backupScopeFor(s.process.StepIDs)
		if err != nil {
			s.outcome = Outcome{State: StateRecoveryFailed, From: fromVersion, To: s.target, Detail: "数据恢复失败，现场完整保留，需要人工处理"}
			return false, s.failRecovery(ctx, migrationRecoveryFailed("cannot determine recovery scope", err))
		}
	}
	if err := restoreFromBackup(ctx, s.dataDir, s.w, s.process.Backup.RunID, scope); err != nil {
		s.outcome = Outcome{State: StateRecoveryFailed, From: fromVersion, To: s.target, Detail: "数据恢复失败，现场完整保留，需要人工处理"}
		return false, s.failRecovery(ctx, migrationRecoveryFailed("restore from backup failed", err))
	}
	if err := deleteProcessRecord(s.w); err != nil {
		s.outcome = Outcome{State: StateRecoveryFailed, From: fromVersion, To: s.target, Detail: "数据恢复失败，现场完整保留，需要人工处理"}
		return false, s.failRecovery(ctx, migrationRecoveryFailed("process record removal failed", err))
	}
	if err := removeBackupRun(s.w, s.process.Backup.RunID); err != nil {
		s.outcome = Outcome{State: StateRecoveryFailed, From: fromVersion, To: s.target, Detail: "数据恢复失败，现场完整保留，需要人工处理"}
		return false, s.failRecovery(ctx, migrationRecoveryFailed("backup cleanup failed", err))
	}
	s.process = nil
	s.outcome = Outcome{State: StateRecovered, From: fromVersion, To: s.target, Detail: "数据已恢复到迁移开始前"}
	if err := s.writeStatus(ctx, currentDataVersionFromStorage(ctx, s.dataDir, fromVersion), false); err != nil {
		return true, migrationStatusUnknown("data recovered but status record could not be persisted", err)
	}
	return true, nil
}

// failRecovery 恢复失败：保留完整现场并写 recovery-failed 状态。
func (s *Session) failRecovery(ctx context.Context, cause error) error {
	record, statusErr := newStatusRecord(Outcome{
		State:  StateRecoveryFailed,
		From:   s.outcome.From,
		To:     s.target,
		Detail: "数据恢复失败，现场完整保留，需要人工处理",
	}, currentDataVersionFromStorage(ctx, s.dataDir, s.outcome.From), s.stepIDs, false)
	if statusErr == nil {
		statusErr = writeStatusRecord(s.w, record)
	}
	if statusErr != nil {
		return migrationRecoveryFailed("data recovery failed and status record could not be written", cause)
	}
	return migrationRecoveryFailed("data recovery failed, scene is fully retained", cause)
}

// Complete 在核心能力完成启动后写迁移成功事实并清理恢复资料。
func (s *Session) Complete(ctx context.Context) error {
	if s.process != nil {
		version, exists, err := datastorage.ReadStorageVersion(ctx, s.dataDir)
		if err != nil || !exists {
			return migrationPrepareFailed("cannot confirm migrated data version", err)
		}
		if version.Version != s.target {
			return migrationPrepareFailed(fmt.Sprintf("data version fact %s does not match migration target %s", version.Version, s.target), nil)
		}
	}
	if err := s.writeStatus(ctx, currentDataVersionFromStorage(ctx, s.dataDir, s.outcome.To), true); err != nil {
		return err
	}
	if s.process != nil {
		runID := s.process.Backup.RunID
		if err := deleteProcessRecord(s.w); err != nil {
			return migrationPrepareFailed("failed to remove process record after migration", err)
		}
		if err := removeBackupRun(s.w, runID); err != nil {
			return migrationPrepareFailed("failed to clean backup after migration", err)
		}
		s.process = nil
	}
	s.completed = true
	return nil
}

// writeStatus 写本次启动的四态结果状态文件。
func (s *Session) writeStatus(ctx context.Context, currentDataVersion string, completed bool) error {
	record, err := newStatusRecord(s.outcome, currentDataVersion, s.stepIDs, completed)
	if err != nil {
		return migrationInvalid("cannot build status record", err)
	}
	if err := writeStatusRecord(s.w, record); err != nil {
		return migrationStatusUnknown("cannot persist status record", err)
	}
	return nil
}
