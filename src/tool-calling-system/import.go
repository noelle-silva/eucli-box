package toolcalling

import (
	"context"
	"os"
	"path/filepath"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

// ImportToolPackage 从本地上传的压缩包直接导入工具。
// 请求线程完成解压、最小核验、兼容判定与占用检查，失败同步返回原因；
// 通过后由独立后台任务推进准备、探测与切换，与商店安装共用同一条落地主路。
func (s *system) ImportToolPackage(ctx context.Context, archivePath string) (types.ArtifactInstallState, error) {
	if s.config.ProgramRoot == "" {
		return types.ArtifactInstallState{}, toolInvalid("managed tool programs are not configured", nil)
	}
	if err := ctx.Err(); err != nil {
		return types.ArtifactInstallState{}, toolExecutionInvalid("import cancelled", err)
	}
	importDir, err := os.MkdirTemp("", "eucli-tool-import-*")
	if err != nil {
		return types.ArtifactInstallState{}, toolStorageFailed("failed to create import workspace", err)
	}
	if err := release.ExtractArchive(release.ExtractArchiveOptions{ArchivePath: archivePath, TargetDir: importDir}); err != nil {
		_ = os.RemoveAll(importDir)
		return types.ArtifactInstallState{}, toolInvalid("导入包解压失败："+err.Error(), err)
	}
	validated, err := release.ValidatePackageDirectory(release.ValidatePackageDirectoryOptions{Directory: importDir, Kind: types.ReleaseArtifactKindTool})
	if err != nil {
		_ = os.RemoveAll(importDir)
		return types.ArtifactInstallState{}, toolInvalid("导入包核验失败："+err.Error(), err)
	}
	toolID, err := cleanToolID(validated.Artifact.ID)
	if err != nil {
		_ = os.RemoveAll(importDir)
		return types.ArtifactInstallState{}, err
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: toolID}
	definition, err := release.ReadToolPackageDefinition(importDir)
	if err != nil {
		_ = os.RemoveAll(importDir)
		return types.ArtifactInstallState{}, toolInvalid("导入包核验失败："+err.Error(), err)
	}
	compatibility := release.AssessEucliBoxCompatibility(validated.Version, s.boxVersion, definition.EucliBoxCompatibility)
	if !compatibility.Compatible {
		_ = os.RemoveAll(importDir)
		return s.operationState(identity, "", validated.Version, types.ArtifactStatusBlocked, types.ArtifactPhaseCompatibility, types.ArtifactErrorCompatibility, compatibility.Reason)
	}
	activity := s.activityFor(toolID)
	if snapshot := activity.runningSnapshot(); snapshot != nil {
		_ = os.RemoveAll(importDir)
		return s.operationState(identity, snapshot.CurrentVersion, validated.Version, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorUpdateInProgress, "同一工具已有操作正在进行")
	}
	if err := s.recoverPendingOperation(ctx, toolID); err != nil {
		_ = os.RemoveAll(importDir)
		return types.ArtifactInstallState{}, err
	}
	currentVersion, installed, err := s.currentToolVersion(ctx, toolID)
	if err != nil {
		_ = os.RemoveAll(importDir)
		return types.ArtifactInstallState{}, err
	}
	operationID := utils.NewID(toolOperationIDPrefix)
	taskCtx, cancel := context.WithCancel(context.Background())
	if blocked := activity.beginUpdate(operationID, cancel); blocked != "" {
		cancel()
		_ = os.RemoveAll(importDir)
		if blocked == types.ArtifactErrorUpdateInProgress {
			return s.operationState(identity, currentVersion, validated.Version, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorUpdateInProgress, "同一工具已有操作正在进行")
		}
		return s.operationState(identity, currentVersion, validated.Version, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorToolActive, "工具仍有真实执行，无法开始安装")
	}
	activity.setRunningBase(currentVersion, installed)
	go s.runToolImportAsync(taskCtx, toolID, operationID, currentVersion, validated, importDir)
	snapshot := activity.runningSnapshot()
	if snapshot == nil {
		snapshot = &release.RunningOperationSnapshot{OperationID: operationID, Phase: types.ArtifactPhasePrepare, CurrentVersion: currentVersion, Installed: installed}
	}
	return s.runningState(identity, snapshot), nil
}

// runToolImportAsync 推进一次本地导入的落地阶段（准备 → 探测 → 切换）。
// 解压、核验、兼容与占用已在受理线程完成；本函数负责在终态释放租约并清理导入工作区。
func (s *system) runToolImportAsync(ctx context.Context, toolID string, operationID string, currentVersion string, validated release.ValidatedPackage, importDir string) {
	activity := s.activityFor(toolID)
	defer activity.finishUpdate()
	defer activity.endUpdate()
	defer func() { _ = os.RemoveAll(importDir) }()
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: toolID}

	s.sweepStaleToolWorkDirs(toolID)
	workDir := filepath.Join(s.toolProgramRoot(toolID), "work", operationID)
	defer func() { _ = os.RemoveAll(workDir) }()
	record, err := release.NewOperationRecord(operationID, identity, release.OperationActionInstall, validated.Version, workDir)
	if err != nil {
		return
	}
	record.CurrentVersion = currentVersion
	setPhase := func(phase string) {
		record.Phase = phase
		activity.updatePhase(phase)
		_ = s.writeOperation(toolID, record)
	}
	s.advanceToolOperation(ctx, toolID, identity, record, setPhase, currentVersion, validated)
}
