package toolcalling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

// toolCancelWait 是取消接口等待任务收尾的上限；超时后返回当前状态。
const toolCancelWait = 10 * time.Second

// cleanToolID 校验工具 ID；拒绝路径分隔符、空值和越界写法。
func cleanToolID(toolID string) (string, error) {
	toolID = strings.TrimSpace(toolID)
	if toolID == "" || toolID == "." || toolID == ".." || strings.Contains(toolID, "..") || strings.ContainsAny(toolID, `/\\`) {
		return "", toolInvalid("tool id contains unsafe path characters", nil)
	}
	return toolID, nil
}

func (s *system) toolProgramRoot(toolID string) string {
	return filepath.Join(s.config.ProgramRoot, toolID)
}

// toolPackageSource 从候选构造取包来源；本地候选走本地货架打包事实。
func (s *system) toolPackageSource(candidate *releasecheck.ReleaseCandidate) (release.ArtifactPackageSource, error) {
	if candidate != nil && candidate.Local {
		return releasecheck.LocalSource(candidate)
	}
	return candidate.PackageSource()
}

// toolOperationIDPrefix 是工具操作 ID 的固定前缀；work/ 下只有该前缀的目录属于操作工作目录。
const toolOperationIDPrefix = "tool-operation"

func (s *system) toolOperationFile(toolID string) string {
	return filepath.Join(s.toolProgramRoot(toolID), "operation.json")
}

func (s *system) toolProgramStore(toolID string) (release.ProgramStore, error) {
	return release.NewProgramStore(s.toolProgramRoot(toolID), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: toolID})
}

// InstallTool 只接受工具 ID：请求线程完成占用与恢复后立即返回运行态，
// 安装由独立后台任务推进，与请求生命周期无关。
func (s *system) InstallTool(ctx context.Context, toolID string) (types.ArtifactInstallState, error) {
	return s.startToolOperation(ctx, toolID, release.OperationActionInstall)
}

// UpdateTool 只接受工具 ID；不适用或有真实活动时在启动前返回。
func (s *system) UpdateTool(ctx context.Context, toolID string) (types.ArtifactInstallState, error) {
	return s.startToolOperation(ctx, toolID, release.OperationActionUpdate)
}

// ToolInstallState 返回工具当前整体安装/更新状态：
// 运行中的任务直接来自租约事实（含阶段与下载进度）；
// 否则先按阶段恢复中断操作，再组合当前版本与落盘记录。
func (s *system) ToolInstallState(ctx context.Context, toolID string) (types.ArtifactInstallState, error) {
	toolID, err := cleanToolID(toolID)
	if err != nil {
		return types.ArtifactInstallState{}, err
	}
	if s.config.ProgramRoot == "" {
		return types.ArtifactInstallState{}, toolInvalid("managed tool programs are not configured", nil)
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: toolID}
	activity := s.activityFor(toolID)
	if snapshot := activity.runningSnapshot(); snapshot != nil {
		return s.runningState(identity, snapshot), nil
	}
	if err := s.recoverPendingOperation(ctx, toolID); err != nil {
		return types.ArtifactInstallState{}, err
	}
	state, err := s.buildInstallState(ctx, toolID)
	if err != nil && state.Artifact.ID == "" {
		return types.ArtifactInstallState{}, err
	}
	return state, nil
}

// CancelToolOperation 取消正在进行的工具操作。
// 只允许在切换前阶段与探测阶段取消；进入切换后拒绝，保证要么切成功、要么完整回滚。
func (s *system) CancelToolOperation(ctx context.Context, toolID string) (types.ArtifactInstallState, error) {
	toolID, err := cleanToolID(toolID)
	if err != nil {
		return types.ArtifactInstallState{}, err
	}
	if s.config.ProgramRoot == "" {
		return types.ArtifactInstallState{}, toolInvalid("managed tool programs are not configured", nil)
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: toolID}
	activity := s.activityFor(toolID)
	snapshot := activity.runningSnapshot()
	if snapshot == nil {
		return s.ToolInstallState(ctx, toolID)
	}
	if !release.OperationPhaseAllowsCancel(snapshot.Phase) {
		return s.operationState(identity, snapshot.CurrentVersion, "", types.ArtifactStatusBlocked, snapshot.Phase, types.ArtifactErrorCancelRejected, "已进入切换阶段，无法取消")
	}
	cancel, ok := activity.cancelUpdate()
	if !ok {
		return s.ToolInstallState(ctx, toolID)
	}
	cancel()
	waitCtx, waitCancel := context.WithTimeout(ctx, toolCancelWait)
	defer waitCancel()
	if err := activity.waitDone(waitCtx); err != nil {
		return s.ToolInstallState(ctx, toolID)
	}
	return s.ToolInstallState(ctx, toolID)
}

// ListToolOperations 返回所有有操作事实的工具状态：
// 运行中的任务（含阶段与进度）与落盘的失败/取消终态记录。
func (s *system) ListToolOperations(ctx context.Context) ([]types.ArtifactInstallState, error) {
	states := []types.ArtifactInstallState{}
	if s.config.ProgramRoot == "" {
		return states, nil
	}
	for _, toolID := range s.runningToolIDs() {
		state, err := s.ToolInstallState(ctx, toolID)
		if err == nil && state.Artifact.ID != "" {
			states = append(states, state)
		}
	}
	entries, err := os.ReadDir(s.config.ProgramRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return states, nil
		}
		return nil, toolStorageFailed("failed to read tool program root", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		toolID := entry.Name()
		if s.activityFor(toolID).runningSnapshot() != nil {
			continue
		}
		record, recordErr := release.ReadOperationRecord(s.toolOperationFile(toolID))
		if recordErr != nil {
			continue
		}
		identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: toolID}
		if record.Artifact != identity || !record.IsTerminal() {
			continue
		}
		state, stateErr := s.buildInstallState(ctx, toolID)
		if stateErr == nil && state.Artifact.ID != "" {
			states = append(states, state)
		}
	}
	return states, nil
}

// runningToolIDs 返回当前有更新任务在进行的工具 ID 快照。
func (s *system) runningToolIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.activities))
	for toolID, activity := range s.activities {
		if activity.runningSnapshot() != nil {
			ids = append(ids, toolID)
		}
	}
	return ids
}

// ToolActivity 返回工具当前真实活动事实。
func (s *system) ToolActivity(ctx context.Context, toolID string) (types.ArtifactActivityState, error) {
	toolID, err := cleanToolID(toolID)
	if err != nil {
		return types.ArtifactActivityState{}, err
	}
	state := s.activityFor(toolID).state()
	state.Artifact = types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: toolID}
	return state, nil
}

// StopToolExecution stops the currently executing instances of one tool.
// Each affected run is cancelled by the user-facing stop action and reports
// the cancelled outcome through the normal execution flow.
func (s *system) StopToolExecution(ctx context.Context, toolID string) (types.ToolStopResult, error) {
	toolID, err := cleanToolID(toolID)
	if err != nil {
		return types.ToolStopResult{}, err
	}
	s.mu.Lock()
	set := s.activeExecutions[toolID]
	runs := make([]*toolRunContext, 0, len(set))
	for runCtx := range set {
		runs = append(runs, runCtx)
	}
	s.mu.Unlock()
	if len(runs) == 0 {
		return types.ToolStopResult{Terminated: 0}, nil
	}
	for _, runCtx := range runs {
		runCtx.cancel()
	}
	return types.ToolStopResult{Terminated: len(runs)}, nil
}

// startToolOperation 在请求线程完成校验、中断恢复、候选确定与租约占用，
// 随后启动独立后台任务并立即返回运行态。
// 任务上下文独立于请求：桌面端断开、退出都不再中断下载与切换。
func (s *system) startToolOperation(ctx context.Context, toolID string, action string) (types.ArtifactInstallState, error) {
	if s.config.ProgramRoot == "" {
		return types.ArtifactInstallState{}, toolInvalid("managed tool programs are not configured", nil)
	}
	toolID, err := cleanToolID(toolID)
	if err != nil {
		return types.ArtifactInstallState{}, err
	}
	if err := ctx.Err(); err != nil {
		return types.ArtifactInstallState{}, toolExecutionInvalid("operation cancelled", err)
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: toolID}
	activity := s.activityFor(toolID)
	// 运行中的任务必须最先识别：中断恢复只服务崩溃遗留，绝不能触碰进行中的工作目录。
	if snapshot := activity.runningSnapshot(); snapshot != nil {
		return s.operationState(identity, snapshot.CurrentVersion, "", types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorUpdateInProgress, "同一工具已有操作正在进行")
	}
	if err := s.recoverPendingOperation(ctx, toolID); err != nil {
		return types.ArtifactInstallState{}, err
	}
	currentVersion, installed, stateErr := s.currentToolVersion(ctx, toolID)
	if stateErr != nil {
		return types.ArtifactInstallState{}, stateErr
	}
	if action == release.OperationActionUpdate && !installed {
		return s.operationState(identity, "", "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorNotInstalled, "工具尚未安装，无法更新")
	}

	// 候选与适用性判定在受理线程完成：失败与阻止在此同步返回，后台任务只负责执行。
	candidate, err := s.config.Candidates.LatestCandidate(ctx, identity)
	if err != nil {
		return s.operationState(identity, currentVersion, "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorReleaseUnavailable, "读取官方候选失败："+err.Error())
	}
	if candidate.Artifact != identity {
		return s.operationState(identity, currentVersion, "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorCandidateMismatch, "候选身份与目标工具不一致")
	}
	source, err := s.toolPackageSource(candidate)
	if err != nil {
		return s.operationState(identity, currentVersion, "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorCandidateMismatch, "候选取包来源无效："+err.Error())
	}
	targetVersion := candidate.Version
	if action == release.OperationActionUpdate {
		order, compareErr := release.CompareVersions(targetVersion, currentVersion)
		if compareErr == nil && order <= 0 {
			return s.buildInstallState(ctx, toolID)
		}
	}
	compatibility := release.AssessEucliBoxCompatibility(targetVersion, s.boxVersion, *candidate.Compatibility)
	if !compatibility.Compatible {
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusBlocked, types.ArtifactPhaseCompatibility, types.ArtifactErrorCompatibility, compatibility.Reason)
	}

	operationID := utils.NewID(toolOperationIDPrefix)
	taskCtx, cancel := context.WithCancel(context.Background())
	if blocked := activity.beginUpdate(operationID, cancel); blocked != "" {
		cancel()
		if blocked == types.ArtifactErrorUpdateInProgress {
			return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorUpdateInProgress, "同一工具已有操作正在进行")
		}
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorToolActive, "工具仍有真实执行，无法开始更新")
	}
	activity.setRunningBase(currentVersion, installed)
	go s.runToolOperationAsync(taskCtx, toolID, action, operationID, currentVersion, targetVersion, source)
	snapshot := activity.runningSnapshot()
	if snapshot == nil {
		snapshot = &release.RunningOperationSnapshot{OperationID: operationID, Phase: types.ArtifactPhaseCandidate, CurrentVersion: currentVersion, Installed: installed}
	}
	return s.runningState(identity, snapshot), nil
}

// runToolOperationAsync 推进一次工具安装/更新的执行阶段（下载 → 准备 → 探测 → 切换）。
// 候选、兼容与租约占用已在受理线程完成；本函数负责在终态释放租约。
// 每次返回都表示本轮操作已进入终态；崩溃不返回时由 recoverPendingOperation 回收。
func (s *system) runToolOperationAsync(ctx context.Context, toolID string, action string, operationID string, currentVersion string, targetVersion string, source release.ArtifactPackageSource) {
	activity := s.activityFor(toolID)
	defer activity.finishUpdate()
	defer activity.endUpdate()
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: toolID}

	s.sweepStaleToolWorkDirs(toolID)
	workDir := filepath.Join(s.toolProgramRoot(toolID), "work", operationID)
	defer func() { _ = os.RemoveAll(workDir) }()
	record, err := release.NewOperationRecord(operationID, identity, action, targetVersion, workDir)
	if err != nil {
		return
	}
	record.CurrentVersion = currentVersion
	setPhase := func(phase string) {
		record.Phase = phase
		activity.updatePhase(phase)
		_ = s.writeOperation(toolID, record)
	}

	setPhase(types.ArtifactPhaseDownload)
	validated, err := release.AcquireAndValidatePackage(ctx, release.AcquirePackageOptions{
		Source:       source,
		DownloadDir:  filepath.Join(workDir, "download"),
		ExtractedDir: filepath.Join(workDir, "extracted"),
		Client:       s.config.HTTPClient,
		OnProgress: func(progress release.DownloadProgress) {
			activity.updateProgress(types.ReleaseOperationProgress{ReceivedBytes: progress.ReceivedBytes, TotalBytes: progress.TotalBytes})
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			s.finishOperationCancelled(toolID, record, types.ArtifactPhaseDownload)
			return
		}
		code, phase := acquireErrorMapping(err)
		s.finishOperation(toolID, record, phase, code, err.Error())
		return
	}
	if ctx.Err() != nil {
		s.finishOperationCancelled(toolID, record, types.ArtifactPhaseDownload)
		return
	}

	setPhase(types.ArtifactPhasePrepare)
	store, err := s.toolProgramStore(toolID)
	if err != nil {
		s.finishOperation(toolID, record, types.ArtifactPhasePrepare, types.ArtifactErrorPathInvalid, err.Error())
		return
	}
	prepared, err := store.PrepareVersion(ctx, validated.Directory, source.Product, validated.Files)
	if err != nil {
		if ctx.Err() != nil {
			s.finishOperationCancelled(toolID, record, types.ArtifactPhasePrepare)
			return
		}
		s.finishOperation(toolID, record, types.ArtifactPhasePrepare, types.ArtifactErrorPrepareFailed, err.Error())
		return
	}
	if ctx.Err() != nil {
		s.finishOperationCancelled(toolID, record, types.ArtifactPhasePrepare)
		return
	}

	setPhase(types.ArtifactPhaseProbe)
	probeDataDir := filepath.Join(workDir, "probe-data")
	if err := s.probeTool(ctx, prepared, probeDataDir); err != nil {
		if ctx.Err() != nil {
			s.finishOperationCancelled(toolID, record, types.ArtifactPhaseProbe)
			return
		}
		s.finishOperation(toolID, record, types.ArtifactPhaseProbe, types.ArtifactErrorProbeFailed, err.Error())
		return
	}

	// 进入切换瞬间起取消窗口关闭：当前版本开始变更，必须保证要么切成功、要么完整回滚。
	setPhase(types.ArtifactPhaseSwitch)
	stableCtx := context.WithoutCancel(ctx)
	if err := store.Activate(stableCtx, prepared, currentVersion); err != nil {
		restoreErr := s.restoreVersion(stableCtx, toolID, store, currentVersion)
		code := types.ArtifactErrorSwitchFailed
		message := "切换失败：" + err.Error()
		if restoreErr != nil {
			code = types.ArtifactErrorRestoreFailed
			message = "切换失败且恢复上一版失败：" + err.Error() + "；" + restoreErr.Error()
		}
		s.finishOperation(toolID, record, types.ArtifactPhaseSwitch, code, message)
		return
	}

	setPhase(types.ArtifactPhaseRefresh)
	state, refreshErr := s.buildInstallState(stableCtx, toolID)
	if refreshErr != nil {
		restoreErr := s.restoreVersion(stableCtx, toolID, store, currentVersion)
		if restoreErr != nil {
			s.finishOperation(toolID, record, types.ArtifactPhaseRestore, types.ArtifactErrorRestoreFailed, "刷新失败且恢复上一版失败："+refreshErr.Error()+"；"+restoreErr.Error())
			return
		}
		s.finishOperation(toolID, record, types.ArtifactPhaseRestore, types.ArtifactErrorSwitchFailed, "刷新失败，已恢复上一版："+refreshErr.Error())
		_ = os.Remove(s.toolOperationFile(toolID))
		return
	}
	if state.Status == types.ArtifactStatusUnavailable {
		_ = os.Remove(s.toolOperationFile(toolID))
		return
	}
	_ = os.Remove(s.toolOperationFile(toolID))
}

func (s *system) currentToolVersion(ctx context.Context, toolID string) (string, bool, error) {
	store, err := s.toolProgramStore(toolID)
	if err != nil {
		return "", false, err
	}
	current, err := store.Current()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, toolStorageFailed("failed to read current tool version", err)
	}
	return current.Version, true, nil
}

func (s *system) restoreVersion(ctx context.Context, toolID string, store release.ProgramStore, version string) error {
	if strings.TrimSpace(version) == "" {
		return nil
	}
	if err := store.Restore(ctx, version); err != nil {
		return toolStorageFailed("failed to restore previous tool version", err)
	}
	return nil
}

// probeTool 对准备完成的版本执行基础交接：空业务参数、空用户配置和当前程序目录。
// 它只验证程序能够启动、读取标准输入并返回符合统一交接协议的结构化结果；
// 不执行真实外部任务，不要求模型密钥，不检查工具特有的业务内容
// （强参数工具对空参数返回结构化失败同样证明交接链路可用）。
func (s *system) probeTool(ctx context.Context, prepared release.PreparedProgram, probeDataDir string) error {
	definitionPath := filepath.Join(prepared.Directory, "definition.json")
	payload, err := os.ReadFile(definitionPath)
	if err != nil {
		return toolExecutionInvalid("failed to read tool definition for probe", err)
	}
	var definition types.ToolDefinition
	if err := json.Unmarshal(payload, &definition); err != nil {
		return toolExecutionInvalid("tool definition is invalid for probe", err)
	}
	definition.BodyDirectory = prepared.Directory
	executable, err := selectExecutable(definition)
	if err != nil {
		return toolExecutionInvalid("failed to select probe executable", err)
	}
	executable, err = cleanExecutablePath(types.ToolDefinition{BodyDirectory: prepared.Directory}, executable)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(probeDataDir, 0o755); err != nil {
		return toolExecutionInvalid("failed to prepare probe data directory", err)
	}
	input, err := json.Marshal(types.ToolExecutionInput{
		ActionID:             "probe",
		ToolName:             definition.ID,
		Arguments:            map[string]any{},
		UserConfig:           map[string]any{},
		DefaultConfig:        definition.DefaultConfig,
		ToolBodyDirectory:    prepared.Directory,
		ToolDataDirectory:    probeDataDir,
		HostWorkingDirectory: probeDataDir,
	})
	if err != nil {
		return toolExecutionInvalid("failed to encode probe input", err)
	}
	outcome := s.executeToolProcess(ctx, definition.ID, executable, prepared.Directory, input, nil)
	if outcome.FailureKind != "" {
		message := "tool probe failed: " + outcome.FailureKind
		if outcome.FailureError != nil {
			message += ": " + outcome.FailureError.Error()
		}
		return toolExecutionInvalid(message, outcome.FailureError)
	}
	if outcome.ExitError != nil {
		message := outcome.ExitError.Error()
		if stderr := strings.TrimSpace(string(outcome.Stderr)); stderr != "" {
			message += ": " + stderr
		}
		return toolExecutionInvalid("tool probe failed: "+message, outcome.ExitError)
	}
	if outcome.FailureError != nil {
		return toolExecutionInvalid("tool probe failed: "+outcome.FailureError.Error(), outcome.FailureError)
	}
	var output types.ToolExecutionOutput
	if err := json.Unmarshal(bytes.TrimSpace(outcome.Stdout), &output); err != nil {
		return toolExecutionInvalid("tool probe output is not valid json", err)
	}
	return nil
}

func (s *system) writeOperation(toolID string, record release.OperationRecord) error {
	return release.WriteOperationRecord(s.toolOperationFile(toolID), record)
}

func (s *system) finishOperation(toolID string, record release.OperationRecord, phase string, code string, message string) {
	record.Phase = phase
	record.Result = release.OperationResultFailed
	record.ErrorCode = code
	record.ErrorMessage = message
	record.UpdatedAt = time.Now().UTC()
	_ = s.writeOperation(toolID, record)
}

// finishOperationCancelled 把本轮操作标记为取消终态；记录保留供状态查询展示。
func (s *system) finishOperationCancelled(toolID string, record release.OperationRecord, phase string) {
	record.Phase = phase
	record.Result = release.OperationResultCancelled
	record.UpdatedAt = time.Now().UTC()
	_ = s.writeOperation(toolID, record)
}

// sweepStaleToolWorkDirs 清扫同一工具 work/ 下遗留的旧操作工作目录。
// work/ 只服务进行中的操作；崩溃窗口和历史版本遗留的目录在此自愈。
func (s *system) sweepStaleToolWorkDirs(toolID string) {
	workRoot := filepath.Join(s.toolProgramRoot(toolID), "work")
	entries, err := os.ReadDir(workRoot)
	if err != nil {
		return
	}
	prefix := toolOperationIDPrefix + "-"
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			_ = os.RemoveAll(filepath.Join(workRoot, entry.Name()))
		}
	}
}

// recoverPendingOperation 按阶段处理上次中断的操作；不根据文件时间或目录猜测当前版本。
// 内存中已有运行任务时不做任何恢复：正在进行的操作不受盘上记录影响。
func (s *system) recoverPendingOperation(ctx context.Context, toolID string) error {
	if s.activityFor(toolID).runningSnapshot() != nil {
		return nil
	}
	operationFile := s.toolOperationFile(toolID)
	record, err := release.ReadOperationRecord(operationFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return toolStorageFailed("failed to read pending operation", err)
	}
	if record.Result != release.OperationResultRunning {
		return nil
	}
	store, err := s.toolProgramStore(toolID)
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
			record.ErrorMessage = "无法确认当前工具版本，不启用任何版本"
			record.UpdatedAt = time.Now().UTC()
			_ = s.writeOperation(toolID, record)
			return toolStorageFailed("无法确认当前工具版本："+currentErr.Error(), currentErr)
		}
		if strings.TrimSpace(record.CurrentVersion) != "" && record.CurrentVersion != current.Version {
			if restoreErr := store.Restore(ctx, record.CurrentVersion); restoreErr != nil {
				record.Result = release.OperationResultFailed
				record.Phase = types.ArtifactPhaseRestore
				record.ErrorCode = types.ArtifactErrorRestoreFailed
				record.ErrorMessage = "恢复上一版失败：" + restoreErr.Error()
				record.UpdatedAt = time.Now().UTC()
				_ = s.writeOperation(toolID, record)
				return toolStorageFailed("恢复上一版失败", restoreErr)
			}
		}
		_ = os.RemoveAll(record.WorkDirectory)
		_ = os.Remove(operationFile)
		return nil
	}
	return nil
}

// buildInstallState 组合当前版本事实、操作记录和适用状态。
func (s *system) buildInstallState(ctx context.Context, toolID string) (types.ArtifactInstallState, error) {
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: toolID}
	currentVersion, installed, err := s.currentToolVersion(ctx, toolID)
	if err != nil {
		record, recordErr := release.ReadOperationRecord(s.toolOperationFile(toolID))
		if recordErr == nil && record.IsTerminal() && record.Artifact == identity {
			return s.recordState(identity, record)
		}
		return s.operationState(identity, "", "", types.ArtifactStatusFailed, types.ArtifactPhaseRestore, types.ArtifactErrorStateUnknown, "无法确认当前工具版本，不启用任何版本")
	}
	record, recordErr := release.ReadOperationRecord(s.toolOperationFile(toolID))
	terminalRecord := recordErr == nil && record.IsTerminal() && record.Artifact == identity
	if terminalRecord && !installed {
		return s.recordState(identity, record)
	}
	if !installed {
		return s.operationState(identity, "", "", types.ArtifactStatusNotInstalled, "", "", "")
	}
	tool, err := s.storage.LoadTool(ctx, toolID)
	if err != nil {
		state, _ := s.operationState(identity, currentVersion, "", types.ArtifactStatusUnavailable, "", types.ArtifactErrorStateUnknown, "工具程序资料不能进入正常业务："+err.Error())
		if terminalRecord {
			state.Error = types.ReleaseOperationError{Code: record.ErrorCode, Phase: record.Phase, Message: record.ErrorMessage}
		}
		return state, toolStorageFailed("failed to load installed tool", err)
	}
	tool = s.annotateTool(tool)
	state, _ := s.operationState(identity, currentVersion, "", types.ArtifactStatusActive, "", "", "")
	if !tool.Compatibility.Compatible {
		state, _ = s.operationState(identity, currentVersion, "", types.ArtifactStatusUnavailable, "", types.ArtifactErrorCompatibility, tool.Compatibility.Reason)
	}
	if terminalRecord {
		state.Error = types.ReleaseOperationError{Code: record.ErrorCode, Phase: record.Phase, Message: record.ErrorMessage}
	}
	return state, nil
}

// recordState 把终态操作记录映射为对外状态；失败与取消同等处理。
func (s *system) recordState(identity types.ReleaseArtifactIdentity, record release.OperationRecord) (types.ArtifactInstallState, error) {
	status := types.ArtifactStatusFailed
	if record.Result == release.OperationResultCancelled {
		status = types.ArtifactStatusCancelled
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
