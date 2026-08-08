package systemplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

// cleanPluginID 校验插件 ID；拒绝路径分隔符、空值和越界写法。
func cleanPluginID(pluginID string) (string, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" || pluginID == "." || pluginID == ".." || strings.Contains(pluginID, "..") || strings.ContainsAny(pluginID, `/\\`) {
		return "", pluginInvalid("system plugin id contains unsafe path characters", nil)
	}
	return pluginID, nil
}

func (s *system) pluginProgramRoot(pluginID string) string {
	return filepath.Join(s.sourceDir, pluginID)
}

func (s *system) pluginOperationFile(pluginID string) string {
	return filepath.Join(s.pluginProgramRoot(pluginID), "operation.json")
}

func (s *system) pluginProgramStore(pluginID string) (release.ProgramStore, error) {
	return release.NewProgramStore(s.pluginProgramRoot(pluginID), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: pluginID})
}

// InstallPlugin 只接受插件 ID，通过统一候选读取器取得官方候选后完成安装。
func (s *system) InstallPlugin(ctx context.Context, pluginID string) (types.ArtifactInstallState, error) {
	return s.runPluginOperation(ctx, pluginID, release.OperationActionInstall)
}

// UpdatePlugin 只接受插件 ID；不适用或有真实活动时在下载前返回。
func (s *system) UpdatePlugin(ctx context.Context, pluginID string) (types.ArtifactInstallState, error) {
	return s.runPluginOperation(ctx, pluginID, release.OperationActionUpdate)
}

// PluginInstallState 返回插件当前整体安装/更新状态；发现中断操作时先按阶段恢复。
func (s *system) PluginInstallState(ctx context.Context, pluginID string) (types.ArtifactInstallState, error) {
	pluginID, err := cleanPluginID(pluginID)
	if err != nil {
		return types.ArtifactInstallState{}, err
	}
	if !s.managedPrograms() {
		return types.ArtifactInstallState{}, pluginInvalid("managed plugin programs are not configured", nil)
	}
	if err := s.recoverPendingOperation(ctx, pluginID); err != nil {
		return types.ArtifactInstallState{}, err
	}
	state, err := s.buildInstallState(ctx, pluginID)
	if err != nil && state.Artifact.ID == "" {
		return types.ArtifactInstallState{}, err
	}
	return state, nil
}

// PluginActivity 返回插件当前真实活动事实。
func (s *system) PluginActivity(ctx context.Context, pluginID string) (types.ArtifactActivityState, error) {
	pluginID, err := cleanPluginID(pluginID)
	if err != nil {
		return types.ArtifactActivityState{}, err
	}
	state := s.activityFor(pluginID).state()
	state.Artifact = types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: pluginID}
	state.Reason = s.pluginActiveReason(pluginID)
	return state, nil
}

func (s *system) pluginActiveReason(pluginID string) string {
	s.mu.Lock()
	process := s.persistent[pluginID]
	heartbeat := s.heartbeats[pluginID]
	s.mu.Unlock()
	if process != nil {
		return "插件 persistent 进程仍在运行"
	}
	if heartbeat != nil {
		return "插件心跳刷新仍在运行"
	}
	return ""
}

func (s *system) runPluginOperation(ctx context.Context, pluginID string, action string) (types.ArtifactInstallState, error) {
	if !s.managedPrograms() {
		return types.ArtifactInstallState{}, pluginInvalid("managed plugin programs are not configured", nil)
	}
	pluginID, err := cleanPluginID(pluginID)
	if err != nil {
		return types.ArtifactInstallState{}, err
	}
	if err := ctx.Err(); err != nil {
		return types.ArtifactInstallState{}, pluginExecutionInvalid("operation cancelled", err)
	}
	if err := s.recoverPendingOperation(ctx, pluginID); err != nil {
		return types.ArtifactInstallState{}, err
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: pluginID}
	currentVersion, installed, stateErr := s.currentPluginVersion(ctx, pluginID)
	if stateErr != nil {
		return types.ArtifactInstallState{}, stateErr
	}
	if action == release.OperationActionUpdate && !installed {
		return s.operationState(identity, "", "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorNotInstalled, "插件尚未安装，无法更新")
	}

	candidate, err := s.candidates.LatestCandidate(ctx, identity)
	if err != nil {
		return s.operationState(identity, currentVersion, "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorReleaseUnavailable, "读取官方候选失败："+err.Error())
	}
	if candidate.Artifact != identity {
		return s.operationState(identity, currentVersion, "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorCandidateMismatch, "候选身份与目标插件不一致")
	}
	source, err := candidate.PackageSource()
	if err != nil {
		return s.operationState(identity, currentVersion, "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorCandidateMismatch, "候选取包来源无效："+err.Error())
	}
	targetVersion := candidate.Version
	if action == release.OperationActionUpdate {
		order, compareErr := release.CompareVersions(targetVersion, currentVersion)
		if compareErr == nil && order <= 0 {
			return s.buildInstallState(ctx, pluginID)
		}
	}
	compatibility := release.AssessEucliBoxCompatibility(targetVersion, s.boxVersion, *candidate.Compatibility)
	if !compatibility.Compatible {
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusBlocked, types.ArtifactPhaseCompatibility, types.ArtifactErrorCompatibility, compatibility.Reason)
	}

	activity := s.activityFor(pluginID)
	operationID := utils.NewID("plugin-operation")
	if blocked := activity.beginUpdate(operationID, s.updateWaitTimeout); blocked != "" {
		if blocked == types.ArtifactErrorUpdateInProgress {
			return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorUpdateInProgress, "同一插件已有操作正在进行")
		}
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorPluginActive, "插件仍有真实请求或刷新，无法开始更新")
	}
	defer activity.endUpdate()

	if err := s.stopPluginLifecycles(ctx, pluginID); err != nil {
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorPluginActive, "无法确认真实活动结束："+err.Error())
	}

	workDir := filepath.Join(s.pluginProgramRoot(pluginID), "work", operationID)
	record, err := release.NewOperationRecord(operationID, identity, action, targetVersion, workDir)
	if err != nil {
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorPathInvalid, err.Error())
	}
	record.CurrentVersion = currentVersion
	if err := s.writeOperation(pluginID, record); err != nil {
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorPathInvalid, err.Error())
	}

	validated, err := release.AcquireAndValidatePackage(ctx, release.AcquirePackageOptions{
		Source:       source,
		DownloadDir:  filepath.Join(workDir, "download"),
		ExtractedDir: filepath.Join(workDir, "extracted"),
		Client:       s.httpClient,
	})
	if err != nil {
		code, phase := acquireErrorMapping(err)
		s.finishOperation(pluginID, record, phase, code, err.Error())
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, phase, code, err.Error())
	}

	record.Phase = types.ArtifactPhasePrepare
	_ = s.writeOperation(pluginID, record)
	store, err := s.pluginProgramStore(pluginID)
	if err != nil {
		s.finishOperation(pluginID, record, types.ArtifactPhasePrepare, types.ArtifactErrorPathInvalid, err.Error())
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhasePrepare, types.ArtifactErrorPathInvalid, err.Error())
	}
	prepared, err := store.PrepareVersion(ctx, validated.Directory, source.Product, validated.Files)
	if err != nil {
		s.finishOperation(pluginID, record, types.ArtifactPhasePrepare, types.ArtifactErrorPrepareFailed, err.Error())
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhasePrepare, types.ArtifactErrorPrepareFailed, err.Error())
	}

	record.Phase = types.ArtifactPhaseProbe
	_ = s.writeOperation(pluginID, record)
	probeDataDir := filepath.Join(workDir, "probe-data")
	if err := s.probePlugin(ctx, prepared, probeDataDir); err != nil {
		s.finishOperation(pluginID, record, types.ArtifactPhaseProbe, types.ArtifactErrorProbeFailed, err.Error())
		s.endUpdateAndRestoreLifecycle(activity, ctx, pluginID)
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhaseProbe, types.ArtifactErrorProbeFailed, err.Error())
	}

	record.Phase = types.ArtifactPhaseSwitch
	_ = s.writeOperation(pluginID, record)
	if err := store.Activate(ctx, prepared, currentVersion); err != nil {
		restoreErr := s.restoreVersion(ctx, pluginID, store, currentVersion)
		code := types.ArtifactErrorSwitchFailed
		message := "切换失败：" + err.Error()
		if restoreErr != nil {
			code = types.ArtifactErrorRestoreFailed
			message = "切换失败且恢复上一版失败：" + err.Error() + "；" + restoreErr.Error()
		}
		s.finishOperation(pluginID, record, types.ArtifactPhaseSwitch, code, message)
		s.endUpdateAndRestoreLifecycle(activity, ctx, pluginID)
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhaseSwitch, code, message)
	}
	// 新版本已经启用，旧版本的失败记录不再适用于当前插件状态；
	// 不清除会让插件一直无法使用，且无法通过占位符解析自愈。
	s.setFailure(pluginID, "")

	record.Phase = types.ArtifactPhaseRefresh
	_ = s.writeOperation(pluginID, record)
	state, refreshErr := s.buildInstallState(ctx, pluginID)
	if refreshErr != nil {
		restoreErr := s.restoreVersion(ctx, pluginID, store, currentVersion)
		if restoreErr != nil {
			s.finishOperation(pluginID, record, types.ArtifactPhaseRestore, types.ArtifactErrorRestoreFailed, "刷新失败且恢复上一版失败："+refreshErr.Error()+"；"+restoreErr.Error())
			s.endUpdateAndRestoreLifecycle(activity, ctx, pluginID)
			return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhaseRestore, types.ArtifactErrorRestoreFailed, "刷新失败且恢复上一版失败")
		}
		s.finishOperation(pluginID, record, types.ArtifactPhaseRestore, types.ArtifactErrorSwitchFailed, "刷新失败，已恢复上一版："+refreshErr.Error())
		_ = os.Remove(s.pluginOperationFile(pluginID))
		s.endUpdateAndRestoreLifecycle(activity, ctx, pluginID)
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhaseRestore, types.ArtifactErrorSwitchFailed, "刷新失败，已恢复上一版")
	}
	if state.Status == types.ArtifactStatusUnavailable {
		_ = os.Remove(s.pluginOperationFile(pluginID))
		s.endUpdateAndRestoreLifecycle(activity, ctx, pluginID)
		return state, nil
	}
	s.finishOperation(pluginID, record, types.ArtifactPhaseRefresh, "", "")
	_ = os.Remove(s.pluginOperationFile(pluginID))
	s.endUpdateAndRestoreLifecycle(activity, ctx, pluginID)
	return s.buildInstallState(ctx, pluginID)
}

func (s *system) currentPluginVersion(ctx context.Context, pluginID string) (string, bool, error) {
	store, err := s.pluginProgramStore(pluginID)
	if err != nil {
		return "", false, err
	}
	current, err := store.Current()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, pluginReadFailed("failed to read current plugin version", err)
	}
	return current.Version, true, nil
}

func (s *system) restoreVersion(ctx context.Context, pluginID string, store release.ProgramStore, version string) error {
	if strings.TrimSpace(version) == "" {
		return nil
	}
	if err := store.Restore(ctx, version); err != nil {
		return pluginReadFailed("failed to restore previous plugin version", err)
	}
	return nil
}

// stopPluginLifecycles 停止 persistent 进程和 cached-heartbeat ticker，等待真实结束。
func (s *system) stopPluginLifecycles(ctx context.Context, pluginID string) error {
	s.mu.Lock()
	process := s.persistent[pluginID]
	delete(s.persistent, pluginID)
	s.mu.Unlock()
	if process != nil {
		stopCtx, cancel := context.WithTimeout(ctx, s.updateWaitTimeout)
		defer cancel()
		if err := process.stopGracefully(stopCtx); err != nil {
			return pluginExecutionFailed("persistent process did not stop in time", err)
		}
	}
	if err := s.stopCachedHeartbeat(ctx, pluginID); err != nil {
		return err
	}
	return nil
}

// endUpdateAndRestoreLifecycle 先关闭更新闸门，再按当前版本恢复插件生命周期；
// 恢复动作本身是更新的一部分，不能被自己的闸门挡住。
func (s *system) endUpdateAndRestoreLifecycle(activity *pluginActivity, ctx context.Context, pluginID string) {
	activity.endUpdate()
	s.restorePluginLifecycle(ctx, pluginID)
}

// restorePluginLifecycle 切换或失败恢复后，按当前版本重新发现并恢复对应生命周期。
func (s *system) restorePluginLifecycle(ctx context.Context, pluginID string) {
	record, err := s.findRecord(ctx, pluginID)
	if err != nil || record.status != types.SystemPluginStatusActive {
		return
	}
	switch record.manifest.LifecycleType {
	case types.SystemPluginLifecyclePersistent:
		if _, err := s.ensurePersistentProcess(ctx, record); err != nil {
			s.setFailure(pluginID, err.Error())
		}
	case types.SystemPluginLifecycleCachedHeartbeat:
		if err := s.refreshCachedPlugin(ctx, pluginID); err != nil {
			s.setFailure(pluginID, err.Error())
		}
		s.startCachedHeartbeat(pluginID, time.Duration(record.manifest.HeartbeatIntervalMs)*time.Millisecond)
	}
}

// probePlugin 对新版本执行基础交接：resolve_placeholders、空接口和空配置。
// 它只验证程序能够启动、读取标准输入并返回符合统一交接协议的结构化结果；
// 不执行真实外部任务，不要求模型密钥，不检查插件特有的业务内容
// （插件对空配置返回结构化失败同样证明交接链路可用）。
// 不得把用户占位符数据写入验证工作区。
func (s *system) probePlugin(ctx context.Context, prepared release.PreparedProgram, probeDataDir string) error {
	manifestPath := filepath.Join(prepared.Directory, "manifest.json")
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		return pluginExecutionInvalid("failed to read plugin manifest for probe", err)
	}
	var manifest types.SystemPluginManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return pluginExecutionInvalid("plugin manifest is invalid for probe", err)
	}
	if manifest.LifecycleType == types.SystemPluginLifecycleCachedHeartbeat {
		if err := os.MkdirAll(probeDataDir, 0o755); err != nil {
			return pluginExecutionInvalid("failed to prepare probe data directory", err)
		}
	}
	executable, err := selectExecutable(prepared.Directory, manifest.Binaries)
	if err != nil {
		return pluginExecutionInvalid("failed to select probe executable", err)
	}
	if err := os.MkdirAll(probeDataDir, 0o755); err != nil {
		return pluginExecutionInvalid("failed to prepare probe data directory", err)
	}
	request := types.SystemPluginPlaceholderRequest{
		Action:                pluginPlaceholderAction,
		PluginID:              manifest.ID,
		PlaceholderInterfaces: []types.SystemPluginPlaceholderInterfaceView{},
		UserConfig:            map[string]any{},
		DefaultConfig:         map[string]any{},
		PluginDirectory:       prepared.Directory,
		PluginDataDirectory:   probeDataDir,
		HostWorkingDirectory:  probeDataDir,
	}
	probeCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if manifest.LifecycleType == types.SystemPluginLifecyclePersistent {
		// persistent 插件按长驻方式交接：启动进程、请求一次并验证响应，然后强制终止探针进程。
		process, err := s.startPersistentProcess(probeCtx, pluginRecord{
			manifest:  manifest,
			directory: prepared.Directory,
			executable: executable,
		})
		if err != nil {
			return err
		}
		response, requestErr := process.request(probeCtx, request)
		stopErr := process.forceStopAndWait(probeCtx)
		if requestErr != nil {
			return pluginExecutionInvalid("plugin probe request failed: "+requestErr.Error(), requestErr)
		}
		if stopErr != nil {
			return pluginExecutionInvalid("plugin probe process did not stop: "+stopErr.Error(), stopErr)
		}
		_ = response
		return nil
	}
	input, err := json.Marshal(request)
	if err != nil {
		return pluginExecutionInvalid("failed to encode probe input", err)
	}
	cmd := exec.CommandContext(probeCtx, executable)
	cmd.Dir = prepared.Directory
	cmd.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := err.Error()
		if stderr.Len() > 0 {
			message = message + ": " + stderr.String()
		}
		return pluginExecutionInvalid("plugin probe failed: "+message, err)
	}
	var response types.SystemPluginPlaceholderResponse
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &response); err != nil {
		return pluginExecutionInvalid("plugin probe output is not valid json", err)
	}
	return nil
}

func (s *system) writeOperation(pluginID string, record release.OperationRecord) error {
	return release.WriteOperationRecord(s.pluginOperationFile(pluginID), record)
}

func (s *system) finishOperation(pluginID string, record release.OperationRecord, phase string, code string, message string) {
	record.Phase = phase
	record.Result = release.OperationResultFailed
	record.ErrorCode = code
	record.ErrorMessage = message
	record.UpdatedAt = time.Now().UTC()
	_ = s.writeOperation(pluginID, record)
}

// recoverPendingOperation 按阶段处理上次中断的操作；不根据文件时间或目录猜测当前版本。
func (s *system) recoverPendingOperation(ctx context.Context, pluginID string) error {
	operationFile := s.pluginOperationFile(pluginID)
	record, err := release.ReadOperationRecord(operationFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return pluginReadFailed("failed to read pending operation", err)
	}
	if record.Result != release.OperationResultRunning {
		return nil
	}
	store, err := s.pluginProgramStore(pluginID)
	if err != nil {
		return err
	}
	if release.OperationPhaseIsPreSwitch(record.Phase) {
		_ = os.RemoveAll(record.WorkDirectory)
		_ = os.Remove(operationFile)
		return nil
	}
	if release.OperationPhaseIsPostSwitch(record.Phase) {
		current, currentErr := store.Current()
		if currentErr != nil {
			record.Result = release.OperationResultFailed
			record.Phase = types.ArtifactPhaseRestore
			record.ErrorCode = types.ArtifactErrorStateUnknown
			record.ErrorMessage = "无法确认当前插件版本，不启用任何版本"
			record.UpdatedAt = time.Now().UTC()
			_ = s.writeOperation(pluginID, record)
			return pluginReadFailed("无法确认当前插件版本："+currentErr.Error(), currentErr)
		}
		if strings.TrimSpace(record.CurrentVersion) != "" && record.CurrentVersion != current.Version {
			if restoreErr := store.Restore(ctx, record.CurrentVersion); restoreErr != nil {
				record.Result = release.OperationResultFailed
				record.Phase = types.ArtifactPhaseRestore
				record.ErrorCode = types.ArtifactErrorRestoreFailed
				record.ErrorMessage = "恢复上一版失败：" + restoreErr.Error()
				record.UpdatedAt = time.Now().UTC()
				_ = s.writeOperation(pluginID, record)
				return pluginReadFailed("恢复上一版失败", restoreErr)
			}
		}
		_ = os.RemoveAll(record.WorkDirectory)
		_ = os.Remove(operationFile)
		return nil
	}
	return nil
}

// buildInstallState 组合当前版本事实、操作记录和适用状态。
func (s *system) buildInstallState(ctx context.Context, pluginID string) (types.ArtifactInstallState, error) {
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: pluginID}
	currentVersion, installed, err := s.currentPluginVersion(ctx, pluginID)
	if err != nil {
		record, recordErr := release.ReadOperationRecord(s.pluginOperationFile(pluginID))
		if recordErr == nil && record.Result == release.OperationResultFailed && record.Artifact == identity {
			return s.operationState(identity, record.CurrentVersion, record.TargetVersion, types.ArtifactStatusFailed, record.Phase, record.ErrorCode, record.ErrorMessage)
		}
		return s.operationState(identity, "", "", types.ArtifactStatusFailed, types.ArtifactPhaseRestore, types.ArtifactErrorStateUnknown, "无法确认当前插件版本，不启用任何版本")
	}
	record, recordErr := release.ReadOperationRecord(s.pluginOperationFile(pluginID))
	failedRecord := recordErr == nil && record.Result == release.OperationResultFailed && record.Artifact == identity
	if failedRecord && !installed {
		return s.operationState(identity, record.CurrentVersion, record.TargetVersion, types.ArtifactStatusFailed, record.Phase, record.ErrorCode, record.ErrorMessage)
	}
	if !installed {
		return s.operationState(identity, "", "", types.ArtifactStatusNotInstalled, "", "", "")
	}
	view, err := s.LoadPlugin(ctx, pluginID)
	if err != nil {
		state, _ := s.operationState(identity, currentVersion, "", types.ArtifactStatusUnavailable, "", types.ArtifactErrorStateUnknown, "插件程序资料不能进入正常业务："+err.Error())
		if failedRecord {
			state.Error = types.ReleaseOperationError{Code: record.ErrorCode, Phase: record.Phase, Message: record.ErrorMessage}
		}
		return state, pluginReadFailed("failed to load installed plugin", err)
	}
	state, _ := s.operationState(identity, currentVersion, "", types.ArtifactStatusActive, "", "", "")
	if view.Status != types.SystemPluginStatusActive {
		state, _ = s.operationState(identity, currentVersion, "", types.ArtifactStatusUnavailable, "", types.ArtifactErrorCompatibility, view.StatusMessage)
	}
	if failedRecord {
		state.Error = types.ReleaseOperationError{Code: record.ErrorCode, Phase: record.Phase, Message: record.ErrorMessage}
	}
	return state, nil
}

func (s *system) operationState(identity types.ReleaseArtifactIdentity, currentVersion string, targetVersion string, status string, phase string, code string, message string) (types.ArtifactInstallState, error) {
	state := types.ArtifactInstallState{
		Artifact:       identity,
		Installed:      status == types.ArtifactStatusActive || status == types.ArtifactStatusUnavailable,
		CurrentVersion: currentVersion,
		TargetVersion:  targetVersion,
		Status:         status,
		Phase:          phase,
	}
	if code != "" {
		state.Error = types.ReleaseOperationError{Code: code, Phase: phase, Message: message}
	} else {
		state.Error.Message = message
	}
	return state, nil
}

func acquireErrorMapping(err error) (string, string) {
	message := err.Error()
	switch {
	case strings.Contains(message, "发行清单无效"), strings.Contains(message, "发行清单与冻结候选不一致"), strings.Contains(message, "清单大小与冻结候选不一致"):
		return types.ArtifactErrorManifestInvalid, types.ArtifactPhaseManifest
	case strings.Contains(message, "解开压缩包失败"):
		return types.ArtifactErrorPackageInvalid, types.ArtifactPhaseArchive
	case strings.Contains(message, "包内核对失败"):
		return types.ArtifactErrorPackageInvalid, types.ArtifactPhasePackage
	case strings.Contains(message, "下载压缩包失败"), strings.Contains(message, "下载发行清单失败"):
		return types.ArtifactErrorDownloadFailed, types.ArtifactPhaseDownload
	default:
		return types.ArtifactErrorDownloadFailed, types.ArtifactPhaseDownload
	}
}
