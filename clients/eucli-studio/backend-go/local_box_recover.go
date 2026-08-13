package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	datamigration "eucli-box/src/data-migration-system"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

// updateFailureDecision 是启动验收失败后的唯一决策出口。
type updateFailureDecision string

const (
	decisionRecoveryStart   updateFailureDecision = "recovery-start"   // 存在未完成迁移：用当前新版做一次恢复启动
	decisionRestorePrevious updateFailureDecision = "restore-previous" // 数据未改变或已恢复：恢复上一版程序
	decisionKeepNewVersion  updateFailureDecision = "keep-new-version" // 数据已迁移到新版：不回滚（设计 6.10）
	decisionUnsafe          updateFailureDecision = "unsafe"           // 数据状态无法确认或恢复失败：不启动任何版本
)

// resolveUpdateFailure 按固定顺序查表，不看进程退出码、不看超时、不看文件时间。
func resolveUpdateFailure(handoff datamigration.Handoff, handoffErr error, previousVersion string, previousDataVersion string) updateFailureDecision {
	if handoffErr != nil {
		return decisionUnsafe
	}
	if handoff.ProcessPending {
		return decisionRecoveryStart
	}
	if handoff.StatusPresent && handoff.Outcome.State == datamigration.StateRecoveryFailed && !handoff.Completed {
		return decisionUnsafe
	}
	if handoff.CurrentDataVersion == "" || previousDataVersion == "" {
		return decisionUnsafe
	}
	comparison, err := release.CompareVersions(handoff.CurrentDataVersion, previousDataVersion)
	if err != nil {
		return decisionUnsafe
	}
	if comparison <= 0 {
		return decisionRestorePrevious
	}
	return decisionKeepNewVersion
}

// previousDataVersionFromProgram 读取上一版程序目录 release-product.json 中的数据目标版本；
// 读取或解析失败时返回空串，由决策表按「版本事实不全就不猜」处理。
func previousDataVersionFromProgram(programStoreDir string, version string) string {
	if strings.TrimSpace(version) == "" {
		return ""
	}
	payload, err := os.ReadFile(filepath.Join(programStoreDir, "versions", version, "release-product.json"))
	if err != nil {
		return ""
	}
	product, err := release.DecodeReleaseProductRecord(payload)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(product.DataVersion)
}

// handleUpdateProbeFailure 是启动验收失败后的唯一决策出口：
// 先读迁移交接事实，按决策表决定恢复上一版、保持新版、恢复启动或不启动任何版本；
// 数据安全确认之前绝不恢复上一版程序。
func (m *localBoxManager) handleUpdateProbeFailure(ctx context.Context, state localBoxState, workDir string, previousVersion string, probeErr error) (localBoxState, error) {
	previousDataVersion := previousDataVersionFromProgram(m.paths.programStoreDir, previousVersion)
	for attempt := 0; attempt < 2; attempt++ {
		handoff, handoffErr := datamigration.ReadHandoff(ctx, m.paths.dataDir)
		switch resolveUpdateFailure(handoff, handoffErr, previousVersion, previousDataVersion) {
		case decisionRecoveryStart:
			// 用当前新版做一次恢复启动：业务端进程内的迁移职责会先完成数据恢复再继续启动。
			process, startErr := m.startCurrentForRecovery(ctx, state)
			if startErr == nil {
				return m.finishUpdateSuccess(state, process, process.registration.BoxVersion, true, true, workDir), nil
			}
			// 恢复启动失败：下一次循环重新决策；仍是 recovery-start 时在循环结束后升级为 unsafe。
		case decisionRestorePrevious:
			return m.restorePreviousAfterFailure(ctx, state, workDir, previousVersion, probeErr)
		case decisionKeepNewVersion:
			m.recordOperationFailure(types.ArtifactPhaseProbe, localBoxErrorUpdateFailed, "数据已迁移到新版本，无法退回上一版；可通过启动入口重试新版")
			_ = os.RemoveAll(workDir)
			failed := localBoxFailure(state, localBoxErrorUpdateFailed, fmt.Sprintf("新版启动验收失败：%v；数据已迁移到新版本，无法退回上一版，可通过启动入口重试新版", probeErr), types.ArtifactPhaseProbe)
			m.publishState(failed)
			return failed, nil
		case decisionUnsafe:
			return m.handleUnsafeUpdateFailure(state, workDir, fmt.Sprintf("新版启动验收失败且数据状态无法确认：%v", probeErr))
		}
	}
	return m.handleUnsafeUpdateFailure(state, workDir, "恢复启动失败，禁止无限重试，需要人工处理")
}

// startCurrentForRecovery 用切换后的当前新版启动业务端，用于未完成迁移的恢复启动。
func (m *localBoxManager) startCurrentForRecovery(ctx context.Context, state localBoxState) (*localBoxProcess, error) {
	state.Status = localBoxStatusRestoring
	state.Progress.Phase = types.ArtifactPhaseProbe
	m.publishState(state)
	record, err := m.readInstall()
	if err != nil {
		return nil, err
	}
	program, err := m.currentProgramFacts()
	if err != nil {
		return nil, err
	}
	return startLocalBoxProcess(ctx, m.paths, *record, program)
}

// restorePreviousAfterFailure 数据未改变或已恢复时：恢复上一版程序并用上一版走同一条启动验收链。
func (m *localBoxManager) restorePreviousAfterFailure(ctx context.Context, state localBoxState, workDir string, previousVersion string, probeErr error) (localBoxState, error) {
	state.Status = localBoxStatusRestoring
	state.Progress.Phase = types.ArtifactPhaseRestore
	m.publishState(state)
	store, err := m.programStore()
	if err != nil {
		return m.failRestoreRetained(state, err)
	}
	if err := store.Restore(ctx, previousVersion); err != nil {
		return m.failRestoreRetained(state, fmt.Errorf("恢复上一版程序失败：%w", err))
	}
	program, err := m.currentProgramFacts()
	if err != nil {
		return m.failRestoreRetained(state, err)
	}
	record, err := m.readInstall()
	if err != nil {
		return m.failRestoreRetained(state, err)
	}
	process, err := startLocalBoxProcess(ctx, m.paths, *record, program)
	if err != nil {
		return m.failRestoreRetained(state, fmt.Errorf("上一版启动验收失败：%w", err))
	}
	m.recordOperationFailure(types.ArtifactPhaseRestore, localBoxErrorUpdateFailed, fmt.Sprintf("更新失败，已自动恢复上一版：%v", probeErr))
	_ = os.RemoveAll(workDir)
	return m.finishUpdateSuccess(state, process, program.Version, false, false, workDir), nil
}

// failRestoreRetained 恢复上一版失败或上一版验收失败：保留 operation.json、工作区与全部版本目录，停止重试。
func (m *localBoxManager) failRestoreRetained(state localBoxState, cause error) (localBoxState, error) {
	m.recordOperationFailure(types.ArtifactPhaseRestore, localBoxErrorRestoreFailed, cause.Error())
	failed := localBoxFailure(state, localBoxErrorRestoreFailed, cause.Error(), types.ArtifactPhaseRestore)
	m.publishState(failed)
	return failed, nil
}

// handleUnsafeUpdateFailure 数据状态无法确认或恢复失败：
// 不启动任何版本、不执行任何 Restore，保留 operation.json、本次工作区、数据目录与迁移工作区全部现场。
func (m *localBoxManager) handleUnsafeUpdateFailure(state localBoxState, workDir string, message string) (localBoxState, error) {
	m.recordOperationFailure(types.ArtifactPhaseRestore, localBoxErrorDataUnsafe, message)
	failed := localBoxFailure(state, localBoxErrorDataUnsafe, message+"；需要人工处理，请不要重装或删除数据目录", types.ArtifactPhaseRestore)
	m.publishState(failed)
	return failed, nil
}

// finishUpdateSuccess 更新或恢复启动成功后的统一收口：登记连接、发布 connected、按需清理操作记录与工作区。
// recovered 表示数据迁移未完成但已恢复，error 字段附信息码，下次启动业务端会重试迁移。
func (m *localBoxManager) finishUpdateSuccess(state localBoxState, process *localBoxProcess, version string, recovered bool, clearOperation bool, workDir string) localBoxState {
	m.mu.Lock()
	m.process = process
	m.connection = process.connection
	onConnect := m.onConnect
	m.mu.Unlock()
	if onConnect != nil {
		onConnect(process.connection)
	}
	state.Status = localBoxStatusConnected
	state.Connected = true
	state.CurrentVersion = version
	state.TargetVersion = ""
	state.Progress = localBoxProgress{}
	state.Error = localBoxError{}
	if recovered {
		state.Error = localBoxError{Code: localBoxInfoMigrationRecovered, Message: "数据迁移未完成、已恢复到迁移前；下次启动业务端会重试迁移"}
	}
	m.publishState(state)
	go m.monitor(process, version)
	if clearOperation {
		if err := os.Remove(m.operationRecordPath()); err == nil {
			_ = os.RemoveAll(workDir)
		}
	}
	return state
}

// recoverPendingUpdate 在接手任何业务端管理动作前处理上次中断的更新；
// 必须持有操作互斥锁，按「切换前丢弃、切换后核对数据再决定」的固定规则处理，不凭文件时间猜。
func (m *localBoxManager) recoverPendingUpdate(ctx context.Context) error {
	record, err := release.ReadOperationRecord(m.operationRecordPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		// 无法解析：先核对当前版本记录完整性；通过则把损坏记录改名保留现场后继续
		if _, currentErr := m.currentProgramFacts(); currentErr == nil {
			invalidPath := m.operationRecordPath() + ".invalid-" + time.Now().UTC().Format("20060102T150405.000000000Z")
			if renameErr := os.Rename(m.operationRecordPath(), invalidPath); renameErr != nil {
				return newError(localBoxErrorUpdateFailed, fmt.Sprintf("损坏操作记录改名保留失败：%v", renameErr))
			}
			return nil
		}
		return newError("LOCAL_BOX_INSTALL_FAILED", "操作记录损坏且版本事实不清，需要重新安装或人工处理")
	}
	if record.Result != release.OperationResultRunning {
		// failed 记录保留不动，供 status() 展示上次失败原因；下一次 update 真正开始时由新记录覆盖
		return nil
	}
	if release.OperationPhaseIsPreSwitch(record.Phase) {
		// 中断发生在切换前：只丢弃未核对内容，当前版本不变
		if err := os.RemoveAll(record.WorkDirectory); err != nil {
			return newError(localBoxErrorUpdateFailed, fmt.Sprintf("清理中断更新工作区失败：%v", err))
		}
		if err := os.Remove(m.operationRecordPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return newError(localBoxErrorUpdateFailed, fmt.Sprintf("清理中断更新记录失败：%v", err))
		}
		return nil
	}
	// 中断发生在切换或验收阶段：必须核对当前版本记录并恢复
	current, err := m.currentProgramFacts()
	if err != nil {
		return newError("LOCAL_BOX_INSTALL_FAILED", "切换后版本事实无法读取，需要重新安装或人工处理")
	}
	switch current.Version {
	case record.TargetVersion:
		return m.resolveSwitchedButUnconfirmed(ctx, record, current)
	case record.CurrentVersion:
		if err := os.RemoveAll(record.WorkDirectory); err != nil {
			return newError(localBoxErrorUpdateFailed, fmt.Sprintf("清理中断更新工作区失败：%v", err))
		}
		m.recordOperationFailure(record.Phase, localBoxErrorUpdateFailed, "更新未生效，保持旧版")
		return nil
	default:
		m.recordOperationFailure(record.Phase, localBoxErrorUpdateFailed, "无法解释的版本状态")
		return newError("LOCAL_BOX_INSTALL_FAILED", "无法解释的版本状态，需要人工处理或重新安装")
	}
}

// resolveSwitchedButUnconfirmed 处理「已切换、未确认成功」的中断更新：
// 数据交接无误且数据版本不低于上一版目标时视为上次实际已成功；其余按决策表处理，
// 接手场景（非用户 update 动作）中 recovery-start 不自动启动业务端。
func (m *localBoxManager) resolveSwitchedButUnconfirmed(ctx context.Context, record release.OperationRecord, current release.CurrentProgram) error {
	previousDataVersion := previousDataVersionFromProgram(m.paths.programStoreDir, record.CurrentVersion)
	handoff, handoffErr := datamigration.ReadHandoff(ctx, m.paths.dataDir)
	if handoffErr == nil && !handoff.ProcessPending && handoff.CurrentDataVersion != "" && previousDataVersion != "" {
		comparison, compareErr := release.CompareVersions(handoff.CurrentDataVersion, previousDataVersion)
		if compareErr == nil && comparison >= 0 {
			if err := os.Remove(m.operationRecordPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
				return newError(localBoxErrorUpdateFailed, fmt.Sprintf("清理成功更新记录失败：%v", err))
			}
			_ = os.RemoveAll(record.WorkDirectory)
			return nil
		}
	}
	switch resolveUpdateFailure(handoff, handoffErr, record.CurrentVersion, previousDataVersion) {
	case decisionRestorePrevious:
		store, err := m.programStore()
		if err != nil {
			return err
		}
		if err := store.Restore(ctx, record.CurrentVersion); err != nil {
			return newError(localBoxErrorRestoreFailed, fmt.Sprintf("恢复上一版失败：%v", err))
		}
		m.recordOperationFailure(record.Phase, localBoxErrorUpdateFailed, "更新未生效，已恢复上一版")
		_ = os.RemoveAll(record.WorkDirectory)
		return nil
	case decisionKeepNewVersion:
		m.recordOperationFailure(record.Phase, localBoxErrorUpdateFailed, "数据已迁移到新版本，保持新版")
		_ = os.RemoveAll(record.WorkDirectory)
		return nil
	case decisionRecoveryStart:
		// 接手场景不自动启动业务端：保持新版为当前版本，说明存在未完成迁移，下次启动会先自动恢复数据
		m.recordOperationFailure(record.Phase, localBoxErrorUpdateFailed, "存在未完成迁移，业务端下次启动会先自动恢复数据")
		_ = os.RemoveAll(record.WorkDirectory)
		return nil
	case decisionUnsafe:
		return newError(localBoxErrorDataUnsafe, "数据状态无法确认或恢复失败，需要人工处理，现场完整保留")
	}
	return nil
}
