package systemplugin

import (
	"context"
	"os"
	"path/filepath"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

// ImportPluginPackage 从本地上传的压缩包直接导入系统插件。
// 请求线程完成解压、最小核验、兼容判定与占用检查，失败同步返回原因；
// 通过后由独立后台任务停机并推进准备、探测与切换，与商店安装共用同一条落地主路。
func (s *system) ImportPluginPackage(ctx context.Context, archivePath string) (types.ArtifactInstallState, error) {
	if !s.managedPrograms() {
		return types.ArtifactInstallState{}, pluginInvalid("managed plugin programs are not configured", nil)
	}
	if err := ctx.Err(); err != nil {
		return types.ArtifactInstallState{}, pluginExecutionInvalid("import cancelled", err)
	}
	importDir, err := os.MkdirTemp("", "eucli-plugin-import-*")
	if err != nil {
		return types.ArtifactInstallState{}, pluginWriteFailed("failed to create import workspace", err)
	}
	if err := release.ExtractArchive(release.ExtractArchiveOptions{ArchivePath: archivePath, TargetDir: importDir}); err != nil {
		_ = os.RemoveAll(importDir)
		return types.ArtifactInstallState{}, pluginInvalid("导入包解压失败："+err.Error(), err)
	}
	validated, err := release.ValidatePackageDirectory(release.ValidatePackageDirectoryOptions{Directory: importDir, Kind: types.ReleaseArtifactKindPlugin})
	if err != nil {
		_ = os.RemoveAll(importDir)
		return types.ArtifactInstallState{}, pluginInvalid("导入包核验失败："+err.Error(), err)
	}
	pluginID, err := cleanPluginID(validated.Artifact.ID)
	if err != nil {
		_ = os.RemoveAll(importDir)
		return types.ArtifactInstallState{}, err
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: pluginID}
	manifest, err := release.ReadPluginPackageDefinition(importDir)
	if err != nil {
		_ = os.RemoveAll(importDir)
		return types.ArtifactInstallState{}, pluginInvalid("导入包核验失败："+err.Error(), err)
	}
	compatibility := release.AssessEucliBoxCompatibility(validated.Version, s.boxVersion, manifest.EucliBoxCompatibility)
	if !compatibility.Compatible {
		_ = os.RemoveAll(importDir)
		return s.operationState(identity, "", validated.Version, types.ArtifactStatusBlocked, types.ArtifactPhaseCompatibility, types.ArtifactErrorCompatibility, compatibility.Reason)
	}
	activity := s.activityFor(pluginID)
	if snapshot := activity.runningSnapshot(); snapshot != nil {
		_ = os.RemoveAll(importDir)
		return s.operationState(identity, snapshot.CurrentVersion, validated.Version, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorUpdateInProgress, "同一插件已有操作正在进行")
	}
	if err := s.recoverPendingOperation(ctx, pluginID); err != nil {
		_ = os.RemoveAll(importDir)
		return types.ArtifactInstallState{}, err
	}
	currentVersion, installed, err := s.currentPluginVersion(ctx, pluginID)
	if err != nil {
		_ = os.RemoveAll(importDir)
		return types.ArtifactInstallState{}, err
	}
	operationID := utils.NewID(pluginOperationIDPrefix)
	taskCtx, cancel := context.WithCancel(context.Background())
	if blocked := activity.beginUpdate(operationID, cancel, s.updateWaitTimeout); blocked != "" {
		cancel()
		_ = os.RemoveAll(importDir)
		if blocked == types.ArtifactErrorUpdateInProgress {
			return s.operationState(identity, currentVersion, validated.Version, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorUpdateInProgress, "同一插件已有操作正在进行")
		}
		return s.operationState(identity, currentVersion, validated.Version, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorPluginActive, "插件仍有真实请求或刷新，无法开始安装")
	}
	activity.setRunningBase(currentVersion, installed)
	go s.runPluginImportAsync(taskCtx, pluginID, operationID, currentVersion, validated, importDir)
	snapshot := activity.runningSnapshot()
	if snapshot == nil {
		snapshot = &release.RunningOperationSnapshot{OperationID: operationID, Phase: types.ArtifactPhaseActivity, CurrentVersion: currentVersion, Installed: installed}
	}
	return s.runningState(identity, snapshot), nil
}

// runPluginImportAsync 推进一次本地导入的落地阶段（停机 → 准备 → 探测 → 切换）。
// 解压、核验、兼容与占用已在受理线程完成；本函数负责在终态释放租约、恢复生命周期并清理导入工作区。
func (s *system) runPluginImportAsync(ctx context.Context, pluginID string, operationID string, currentVersion string, validated release.ValidatedPackage, importDir string) {
	activity := s.activityFor(pluginID)
	stableCtx := context.WithoutCancel(ctx)
	defer func() {
		activity.endUpdate()
		s.restorePluginLifecycle(stableCtx, pluginID)
		activity.finishUpdate()
	}()
	defer func() { _ = os.RemoveAll(importDir) }()
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: pluginID}

	s.sweepStalePluginWorkDirs(pluginID)
	workDir := filepath.Join(s.pluginProgramRoot(pluginID), "work", operationID)
	defer func() { _ = os.RemoveAll(workDir) }()
	record, err := release.NewOperationRecord(operationID, identity, release.OperationActionInstall, validated.Version, workDir)
	if err != nil {
		return
	}
	record.CurrentVersion = currentVersion
	setPhase := func(phase string) {
		record.Phase = phase
		activity.updatePhase(phase)
		_ = s.writeOperation(pluginID, record)
	}

	setPhase(types.ArtifactPhaseActivity)
	if err := s.stopPluginLifecycles(ctx, pluginID); err != nil {
		if ctx.Err() != nil {
			s.finishOperationCancelled(pluginID, record, types.ArtifactPhaseActivity)
			return
		}
		s.finishOperation(pluginID, record, types.ArtifactPhaseActivity, types.ArtifactErrorPluginActive, "无法确认真实活动结束："+err.Error())
		return
	}
	if ctx.Err() != nil {
		s.finishOperationCancelled(pluginID, record, types.ArtifactPhaseActivity)
		return
	}
	s.advancePluginOperation(ctx, pluginID, identity, record, setPhase, currentVersion, validated)
}
