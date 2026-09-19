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
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

// pluginCancelWait 是取消接口等待任务收尾的上限；超时后返回当前状态。
const pluginCancelWait = 10 * time.Second

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

// pluginPackageSource 从候选构造取包来源；本地候选走本地货架打包事实。
func (s *system) pluginPackageSource(candidate *releasecheck.ReleaseCandidate) (release.ArtifactPackageSource, error) {
	if candidate != nil && candidate.Local {
		return releasecheck.LocalSource(candidate)
	}
	return candidate.PackageSource()
}

// pluginOperationIDPrefix 是插件操作 ID 的固定前缀；work/ 下只有该前缀的目录属于操作工作目录。
const pluginOperationIDPrefix = "plugin-operation"

func (s *system) pluginOperationFile(pluginID string) string {
	return filepath.Join(s.pluginProgramRoot(pluginID), "operation.json")
}

func (s *system) pluginProgramStore(pluginID string) (release.ProgramStore, error) {
	return release.NewProgramStore(s.pluginProgramRoot(pluginID), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: pluginID})
}

// InstallPlugin 只接受插件 ID：请求线程完成占用与恢复后立即返回运行态，
// 安装由独立后台任务推进，与请求生命周期无关。
func (s *system) InstallPlugin(ctx context.Context, pluginID string) (types.ArtifactInstallState, error) {
	return s.startPluginOperation(ctx, pluginID, release.OperationActionInstall)
}

// UpdatePlugin 只接受插件 ID；不适用或有真实活动时在启动前返回。
func (s *system) UpdatePlugin(ctx context.Context, pluginID string) (types.ArtifactInstallState, error) {
	return s.startPluginOperation(ctx, pluginID, release.OperationActionUpdate)
}

// PluginInstallState 返回插件当前整体安装/更新状态：
// 运行中的任务直接来自租约事实（含阶段与下载进度）；
// 否则先按阶段恢复中断操作，再组合当前版本与落盘记录。
func (s *system) PluginInstallState(ctx context.Context, pluginID string) (types.ArtifactInstallState, error) {
	pluginID, err := cleanPluginID(pluginID)
	if err != nil {
		return types.ArtifactInstallState{}, err
	}
	if !s.managedPrograms() {
		return types.ArtifactInstallState{}, pluginInvalid("managed plugin programs are not configured", nil)
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: pluginID}
	activity := s.activityFor(pluginID)
	if snapshot := activity.runningSnapshot(); snapshot != nil {
		return s.runningState(identity, snapshot), nil
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

// CancelPluginOperation 取消正在进行的插件操作。
// 只允许在切换前阶段与探测阶段取消；进入切换后拒绝，保证要么切成功、要么完整回滚。
func (s *system) CancelPluginOperation(ctx context.Context, pluginID string) (types.ArtifactInstallState, error) {
	pluginID, err := cleanPluginID(pluginID)
	if err != nil {
		return types.ArtifactInstallState{}, err
	}
	if !s.managedPrograms() {
		return types.ArtifactInstallState{}, pluginInvalid("managed plugin programs are not configured", nil)
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: pluginID}
	activity := s.activityFor(pluginID)
	snapshot := activity.runningSnapshot()
	if snapshot == nil {
		return s.PluginInstallState(ctx, pluginID)
	}
	if !release.OperationPhaseAllowsCancel(snapshot.Phase) {
		return s.operationState(identity, snapshot.CurrentVersion, "", types.ArtifactStatusBlocked, snapshot.Phase, types.ArtifactErrorCancelRejected, "已进入切换阶段，无法取消")
	}
	cancel, ok := activity.cancelUpdate()
	if !ok {
		return s.PluginInstallState(ctx, pluginID)
	}
	cancel()
	waitCtx, waitCancel := context.WithTimeout(ctx, pluginCancelWait)
	defer waitCancel()
	if err := activity.waitDone(waitCtx); err != nil {
		return s.PluginInstallState(ctx, pluginID)
	}
	return s.PluginInstallState(ctx, pluginID)
}

// ListPluginOperations 返回所有有操作事实的插件状态：
// 运行中的任务（含阶段与进度）与落盘的失败/取消终态记录。
func (s *system) ListPluginOperations(ctx context.Context) ([]types.ArtifactInstallState, error) {
	states := []types.ArtifactInstallState{}
	if !s.managedPrograms() {
		return states, nil
	}
	for _, pluginID := range s.runningPluginIDs() {
		state, err := s.PluginInstallState(ctx, pluginID)
		if err == nil && state.Artifact.ID != "" {
			states = append(states, state)
		}
	}
	entries, err := os.ReadDir(s.sourceDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return states, nil
		}
		return nil, pluginReadFailed("failed to read plugin program root", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pluginID := entry.Name()
		if s.activityFor(pluginID).runningSnapshot() != nil {
			continue
		}
		record, recordErr := release.ReadOperationRecord(s.pluginOperationFile(pluginID))
		if recordErr != nil {
			continue
		}
		identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: pluginID}
		if record.Artifact != identity || !record.IsTerminal() {
			continue
		}
		state, stateErr := s.buildInstallState(ctx, pluginID)
		if stateErr == nil && state.Artifact.ID != "" {
			states = append(states, state)
		}
	}
	return states, nil
}

// runningPluginIDs 返回当前有更新任务在进行的插件 ID 快照。
func (s *system) runningPluginIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.activities))
	for pluginID, activity := range s.activities {
		if activity.runningSnapshot() != nil {
			ids = append(ids, pluginID)
		}
	}
	return ids
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

// startPluginOperation 在请求线程完成校验、中断恢复、候选确定与租约占用，
// 随后启动独立后台任务并立即返回运行态。
// 任务上下文独立于请求：桌面端断开、退出都不再中断下载与切换。
func (s *system) startPluginOperation(ctx context.Context, pluginID string, action string) (types.ArtifactInstallState, error) {
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
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: pluginID}
	activity := s.activityFor(pluginID)
	// 运行中的任务必须最先识别：中断恢复只服务崩溃遗留，绝不能触碰进行中的工作目录。
	if snapshot := activity.runningSnapshot(); snapshot != nil {
		return s.operationState(identity, snapshot.CurrentVersion, "", types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorUpdateInProgress, "同一插件已有操作正在进行")
	}
	if err := s.recoverPendingOperation(ctx, pluginID); err != nil {
		return types.ArtifactInstallState{}, err
	}
	currentVersion, installed, stateErr := s.currentPluginVersion(ctx, pluginID)
	if stateErr != nil {
		return types.ArtifactInstallState{}, stateErr
	}
	if action == release.OperationActionUpdate && !installed {
		return s.operationState(identity, "", "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorNotInstalled, "插件尚未安装，无法更新")
	}

	// 候选与适用性判定在受理线程完成：失败与阻止在此同步返回，后台任务只负责执行。
	candidate, err := s.candidates.LatestCandidate(ctx, identity)
	if err != nil {
		return s.operationState(identity, currentVersion, "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorReleaseUnavailable, "读取官方候选失败："+err.Error())
	}
	if candidate.Artifact != identity {
		return s.operationState(identity, currentVersion, "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorCandidateMismatch, "候选身份与目标插件不一致")
	}
	source, err := s.pluginPackageSource(candidate)
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

	operationID := utils.NewID(pluginOperationIDPrefix)
	taskCtx, cancel := context.WithCancel(context.Background())
	if blocked := activity.beginUpdate(operationID, cancel, s.updateWaitTimeout); blocked != "" {
		cancel()
		if blocked == types.ArtifactErrorUpdateInProgress {
			return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorUpdateInProgress, "同一插件已有操作正在进行")
		}
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorPluginActive, "插件仍有真实请求或刷新，无法开始更新")
	}
	activity.setRunningBase(currentVersion, installed)
	go s.runPluginOperationAsync(taskCtx, pluginID, action, operationID, currentVersion, targetVersion, source)
	snapshot := activity.runningSnapshot()
	if snapshot == nil {
		snapshot = &release.RunningOperationSnapshot{OperationID: operationID, Phase: types.ArtifactPhaseCandidate, CurrentVersion: currentVersion, Installed: installed}
	}
	return s.runningState(identity, snapshot), nil
}

// runPluginOperationAsync 推进一次插件安装/更新的执行阶段（停止生命周期 → 下载 → 准备 → 探测 → 切换）。
// 候选、兼容与租约占用已在受理线程完成；本函数负责在终态释放租约并恢复插件生命周期。
// 每次返回都表示本轮操作已进入终态；崩溃不返回时由 recoverPendingOperation 回收。
func (s *system) runPluginOperationAsync(ctx context.Context, pluginID string, action string, operationID string, currentVersion string, targetVersion string, source release.ArtifactPackageSource) {
	activity := s.activityFor(pluginID)
	stableCtx := context.WithoutCancel(ctx)
	defer func() {
		activity.endUpdate()
		s.restorePluginLifecycle(stableCtx, pluginID)
		activity.finishUpdate()
	}()
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: pluginID}

	s.sweepStalePluginWorkDirs(pluginID)
	workDir := filepath.Join(s.pluginProgramRoot(pluginID), "work", operationID)
	defer func() { _ = os.RemoveAll(workDir) }()
	record, err := release.NewOperationRecord(operationID, identity, action, targetVersion, workDir)
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

	setPhase(types.ArtifactPhaseDownload)
	validated, err := release.AcquireAndValidatePackage(ctx, release.AcquirePackageOptions{
		Source:       source,
		DownloadDir:  filepath.Join(workDir, "download"),
		ExtractedDir: filepath.Join(workDir, "extracted"),
		Client:       s.httpClient,
		OnProgress: func(progress release.DownloadProgress) {
			activity.updateProgress(types.ReleaseOperationProgress{ReceivedBytes: progress.ReceivedBytes, TotalBytes: progress.TotalBytes})
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			s.finishOperationCancelled(pluginID, record, types.ArtifactPhaseDownload)
			return
		}
		code, phase := acquireErrorMapping(err)
		s.finishOperation(pluginID, record, phase, code, err.Error())
		return
	}
	if ctx.Err() != nil {
		s.finishOperationCancelled(pluginID, record, types.ArtifactPhaseDownload)
		return
	}

	setPhase(types.ArtifactPhasePrepare)
	store, err := s.pluginProgramStore(pluginID)
	if err != nil {
		s.finishOperation(pluginID, record, types.ArtifactPhasePrepare, types.ArtifactErrorPathInvalid, err.Error())
		return
	}
	prepared, err := store.PrepareVersion(ctx, validated.Directory, source.Product, validated.Files)
	if err != nil {
		if ctx.Err() != nil {
			s.finishOperationCancelled(pluginID, record, types.ArtifactPhasePrepare)
			return
		}
		s.finishOperation(pluginID, record, types.ArtifactPhasePrepare, types.ArtifactErrorPrepareFailed, err.Error())
		return
	}
	if ctx.Err() != nil {
		s.finishOperationCancelled(pluginID, record, types.ArtifactPhasePrepare)
		return
	}

	setPhase(types.ArtifactPhaseProbe)
	probeDataDir := filepath.Join(workDir, "probe-data")
	if err := s.probePlugin(ctx, prepared, probeDataDir); err != nil {
		if ctx.Err() != nil {
			s.finishOperationCancelled(pluginID, record, types.ArtifactPhaseProbe)
			return
		}
		s.finishOperation(pluginID, record, types.ArtifactPhaseProbe, types.ArtifactErrorProbeFailed, err.Error())
		return
	}

	// 进入切换瞬间起取消窗口关闭：当前版本开始变更，必须保证要么切成功、要么完整回滚。
	setPhase(types.ArtifactPhaseSwitch)
	if err := store.Activate(stableCtx, prepared, currentVersion); err != nil {
		restoreErr := s.restoreVersion(stableCtx, pluginID, store, currentVersion)
		code := types.ArtifactErrorSwitchFailed
		message := "切换失败：" + err.Error()
		if restoreErr != nil {
			code = types.ArtifactErrorRestoreFailed
			message = "切换失败且恢复上一版失败：" + err.Error() + "；" + restoreErr.Error()
		}
		s.finishOperation(pluginID, record, types.ArtifactPhaseSwitch, code, message)
		return
	}
	// 新版本已经启用，旧版本的失败记录不再适用于当前插件状态；
	// 不清除会让插件一直无法使用，且无法通过占位符解析自愈。
	s.setFailure(pluginID, "")

	setPhase(types.ArtifactPhaseRefresh)
	state, refreshErr := s.buildInstallState(stableCtx, pluginID)
	if refreshErr != nil {
		restoreErr := s.restoreVersion(stableCtx, pluginID, store, currentVersion)
		if restoreErr != nil {
			s.finishOperation(pluginID, record, types.ArtifactPhaseRestore, types.ArtifactErrorRestoreFailed, "刷新失败且恢复上一版失败："+refreshErr.Error()+"；"+restoreErr.Error())
			return
		}
		s.finishOperation(pluginID, record, types.ArtifactPhaseRestore, types.ArtifactErrorSwitchFailed, "刷新失败，已恢复上一版："+refreshErr.Error())
		_ = os.Remove(s.pluginOperationFile(pluginID))
		return
	}
	if state.Status == types.ArtifactStatusUnavailable {
		_ = os.Remove(s.pluginOperationFile(pluginID))
		return
	}
	_ = os.Remove(s.pluginOperationFile(pluginID))
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

// restorePluginLifecycle 切换、失败恢复或取消后，按当前版本重新发现并恢复对应生命周期。
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

// finishOperationCancelled 把本轮操作标记为取消终态；记录保留供状态查询展示。
func (s *system) finishOperationCancelled(pluginID string, record release.OperationRecord, phase string) {
	record.Phase = phase
	record.Result = release.OperationResultCancelled
	record.UpdatedAt = time.Now().UTC()
	_ = s.writeOperation(pluginID, record)
}

// sweepStalePluginWorkDirs 清扫同一插件 work/ 下遗留的旧操作工作目录。
// work/ 只服务进行中的操作；崩溃窗口和历史版本遗留的目录在此自愈。
func (s *system) sweepStalePluginWorkDirs(pluginID string) {
	workRoot := filepath.Join(s.pluginProgramRoot(pluginID), "work")
	entries, err := os.ReadDir(workRoot)
	if err != nil {
		return
	}
	prefix := pluginOperationIDPrefix + "-"
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			_ = os.RemoveAll(filepath.Join(workRoot, entry.Name()))
		}
	}
}

// recoverPendingOperation 按阶段处理上次中断的操作；不根据文件时间或目录猜测当前版本。
// 内存中已有运行任务时不做任何恢复：正在进行的操作不受盘上记录影响。
func (s *system) recoverPendingOperation(ctx context.Context, pluginID string) error {
	if s.activityFor(pluginID).runningSnapshot() != nil {
		return nil
	}
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
		if recordErr == nil && record.IsTerminal() && record.Artifact == identity {
			return s.recordState(identity, record)
		}
		return s.operationState(identity, "", "", types.ArtifactStatusFailed, types.ArtifactPhaseRestore, types.ArtifactErrorStateUnknown, "无法确认当前插件版本，不启用任何版本")
	}
	record, recordErr := release.ReadOperationRecord(s.pluginOperationFile(pluginID))
	terminalRecord := recordErr == nil && record.IsTerminal() && record.Artifact == identity
	if terminalRecord && !installed {
		return s.recordState(identity, record)
	}
	if !installed {
		return s.operationState(identity, "", "", types.ArtifactStatusNotInstalled, "", "", "")
	}
	view, err := s.LoadPlugin(ctx, pluginID)
	if err != nil {
		state, _ := s.operationState(identity, currentVersion, "", types.ArtifactStatusUnavailable, "", types.ArtifactErrorStateUnknown, "插件程序资料不能进入正常业务："+err.Error())
		if terminalRecord {
			state.Error = types.ReleaseOperationError{Code: record.ErrorCode, Phase: record.Phase, Message: record.ErrorMessage}
		}
		return state, pluginReadFailed("failed to load installed plugin", err)
	}
	state, _ := s.operationState(identity, currentVersion, "", types.ArtifactStatusActive, "", "", "")
	if view.Status != types.SystemPluginStatusActive {
		state, _ = s.operationState(identity, currentVersion, "", types.ArtifactStatusUnavailable, "", types.ArtifactErrorCompatibility, view.StatusMessage)
	}
	if terminalRecord {
		state.Error = types.ReleaseOperationError{Code: record.ErrorCode, Phase: record.Phase, Message: record.ErrorMessage}
	}
	return state, nil
}

// recordState 把终态操作记录映射为对外状态；失败与取消同等处理；
// 活动类错误码保留为「被阻止」语义，供界面区分「失败」与「被阻止」。
func (s *system) recordState(identity types.ReleaseArtifactIdentity, record release.OperationRecord) (types.ArtifactInstallState, error) {
	status := types.ArtifactStatusFailed
	switch {
	case record.Result == release.OperationResultCancelled:
		status = types.ArtifactStatusCancelled
	case record.ErrorCode == types.ArtifactErrorPluginActive || record.ErrorCode == types.ArtifactErrorUpdateInProgress:
		status = types.ArtifactStatusBlocked
	}
	return s.operationState(identity, record.CurrentVersion, record.TargetVersion, status, record.Phase, record.ErrorCode, record.ErrorMessage)
}

// runningState 组合运行中任务的事实：状态由阶段映射，进度为最近一次下载快照。
func (s *system) runningState(identity types.ReleaseArtifactIdentity, snapshot *release.RunningOperationSnapshot) types.ArtifactInstallState {
	return types.ArtifactInstallState{
		OperationID:    snapshot.OperationID,
		Artifact:       identity,
		Installed:      snapshot.Installed,
		CurrentVersion: snapshot.CurrentVersion,
		Status:         statusForPhase(snapshot.Phase),
		Phase:          snapshot.Phase,
		Progress:       snapshot.Progress,
	}
}

// statusForPhase 把运行阶段映射为对外的整体状态；只服务运行中的展示。
func statusForPhase(phase string) string {
	switch phase {
	case types.ArtifactPhaseCandidate, types.ArtifactPhaseCompatibility:
		return types.ArtifactStatusCheckingRelease
	case types.ArtifactPhaseActivity:
		return types.ArtifactStatusCheckingActivity
	case types.ArtifactPhaseDownload:
		return types.ArtifactStatusDownloading
	case types.ArtifactPhaseManifest, types.ArtifactPhaseArchive, types.ArtifactPhasePackage:
		return types.ArtifactStatusVerifying
	case types.ArtifactPhasePrepare:
		return types.ArtifactStatusPreparing
	case types.ArtifactPhaseProbe:
		return types.ArtifactStatusStarting
	case types.ArtifactPhaseSwitch, types.ArtifactPhaseRefresh:
		return types.ArtifactStatusSwitching
	case types.ArtifactPhaseRestore:
		return types.ArtifactStatusRestoring
	default:
		return types.ArtifactStatusCheckingRelease
	}
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
