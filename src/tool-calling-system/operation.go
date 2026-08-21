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
	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

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

func (s *system) toolOperationFile(toolID string) string {
	return filepath.Join(s.toolProgramRoot(toolID), "operation.json")
}

func (s *system) toolProgramStore(toolID string) (release.ProgramStore, error) {
	return release.NewProgramStore(s.toolProgramRoot(toolID), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: toolID})
}

// InstallTool 只接受工具 ID，通过统一候选读取器取得官方候选后完成安装。
func (s *system) InstallTool(ctx context.Context, toolID string) (types.ArtifactInstallState, error) {
	return s.runToolOperation(ctx, toolID, release.OperationActionInstall)
}

// UpdateTool 只接受工具 ID；不适用或有真实活动时在下载前返回。
func (s *system) UpdateTool(ctx context.Context, toolID string) (types.ArtifactInstallState, error) {
	return s.runToolOperation(ctx, toolID, release.OperationActionUpdate)
}

// ToolInstallState 返回工具当前整体安装/更新状态；发现中断操作时先按阶段恢复。
func (s *system) ToolInstallState(ctx context.Context, toolID string) (types.ArtifactInstallState, error) {
	toolID, err := cleanToolID(toolID)
	if err != nil {
		return types.ArtifactInstallState{}, err
	}
	if s.config.ProgramRoot == "" {
		return types.ArtifactInstallState{}, toolInvalid("managed tool programs are not configured", nil)
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

func (s *system) runToolOperation(ctx context.Context, toolID string, action string) (types.ArtifactInstallState, error) {
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
	if err := s.recoverPendingOperation(ctx, toolID); err != nil {
		return types.ArtifactInstallState{}, err
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: toolID}
	currentVersion, installed, stateErr := s.currentToolVersion(ctx, toolID)
	if stateErr != nil {
		return types.ArtifactInstallState{}, stateErr
	}
	if action == release.OperationActionUpdate && !installed {
		return s.operationState(identity, "", "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorNotInstalled, "工具尚未安装，无法更新")
	}

	candidate, err := s.config.Candidates.LatestCandidate(ctx, identity)
	if err != nil {
		return s.operationState(identity, currentVersion, "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorReleaseUnavailable, "读取官方候选失败："+err.Error())
	}
	if candidate.Artifact != identity {
		return s.operationState(identity, currentVersion, "", types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorCandidateMismatch, "候选身份与目标工具不一致")
	}
	source, err := candidate.PackageSource()
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

	activity := s.activityFor(toolID)
	operationID := utils.NewID("tool-operation")
	if blocked := activity.beginUpdate(operationID, s.updateWaitTimeout); blocked != "" {
		if blocked == types.ArtifactErrorUpdateInProgress {
			return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorUpdateInProgress, "同一工具已有操作正在进行")
		}
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusBlocked, types.ArtifactPhaseActivity, types.ArtifactErrorToolActive, "工具仍有真实执行，无法开始更新")
	}
	defer activity.endUpdate()

	workDir := filepath.Join(s.toolProgramRoot(toolID), "work", operationID)
	record, err := release.NewOperationRecord(operationID, identity, action, targetVersion, workDir)
	if err != nil {
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorPathInvalid, err.Error())
	}
	record.CurrentVersion = currentVersion
	if err := s.writeOperation(toolID, record); err != nil {
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhaseCandidate, types.ArtifactErrorPathInvalid, err.Error())
	}

	validated, err := release.AcquireAndValidatePackage(ctx, release.AcquirePackageOptions{
		Source:       source,
		DownloadDir:  filepath.Join(workDir, "download"),
		ExtractedDir: filepath.Join(workDir, "extracted"),
		Client:       s.config.HTTPClient,
	})
	if err != nil {
		code, phase := acquireErrorMapping(err)
		s.finishOperation(toolID, record, phase, code, err.Error())
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, phase, code, err.Error())
	}

	record.Phase = types.ArtifactPhasePrepare
	_ = s.writeOperation(toolID, record)
	store, err := s.toolProgramStore(toolID)
	if err != nil {
		s.finishOperation(toolID, record, types.ArtifactPhasePrepare, types.ArtifactErrorPathInvalid, err.Error())
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhasePrepare, types.ArtifactErrorPathInvalid, err.Error())
	}
	prepared, err := store.PrepareVersion(ctx, validated.Directory, source.Product, validated.Files)
	if err != nil {
		s.finishOperation(toolID, record, types.ArtifactPhasePrepare, types.ArtifactErrorPrepareFailed, err.Error())
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhasePrepare, types.ArtifactErrorPrepareFailed, err.Error())
	}

	record.Phase = types.ArtifactPhaseProbe
	_ = s.writeOperation(toolID, record)
	probeDataDir := filepath.Join(workDir, "probe-data")
	if err := s.probeTool(ctx, prepared, probeDataDir); err != nil {
		s.finishOperation(toolID, record, types.ArtifactPhaseProbe, types.ArtifactErrorProbeFailed, err.Error())
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhaseProbe, types.ArtifactErrorProbeFailed, err.Error())
	}

	record.Phase = types.ArtifactPhaseSwitch
	_ = s.writeOperation(toolID, record)
	if err := store.Activate(ctx, prepared, currentVersion); err != nil {
		restoreErr := s.restoreVersion(ctx, toolID, store, currentVersion)
		code := types.ArtifactErrorSwitchFailed
		message := "切换失败：" + err.Error()
		if restoreErr != nil {
			code = types.ArtifactErrorRestoreFailed
			message = "切换失败且恢复上一版失败：" + err.Error() + "；" + restoreErr.Error()
		}
		s.finishOperation(toolID, record, types.ArtifactPhaseSwitch, code, message)
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhaseSwitch, code, message)
	}

	record.Phase = types.ArtifactPhaseRefresh
	_ = s.writeOperation(toolID, record)
	state, refreshErr := s.buildInstallState(ctx, toolID)
	if refreshErr != nil {
		restoreErr := s.restoreVersion(ctx, toolID, store, currentVersion)
		if restoreErr != nil {
			s.finishOperation(toolID, record, types.ArtifactPhaseRestore, types.ArtifactErrorRestoreFailed, "刷新失败且恢复上一版失败："+refreshErr.Error()+"；"+restoreErr.Error())
			return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhaseRestore, types.ArtifactErrorRestoreFailed, "刷新失败且恢复上一版失败")
		}
		s.finishOperation(toolID, record, types.ArtifactPhaseRestore, types.ArtifactErrorSwitchFailed, "刷新失败，已恢复上一版："+refreshErr.Error())
		_ = os.Remove(s.toolOperationFile(toolID))
		return s.operationState(identity, currentVersion, targetVersion, types.ArtifactStatusFailed, types.ArtifactPhaseRestore, types.ArtifactErrorSwitchFailed, "刷新失败，已恢复上一版")
	}
	if state.Status == types.ArtifactStatusUnavailable {
		_ = os.Remove(s.toolOperationFile(toolID))
		return state, nil
	}
	s.finishOperation(toolID, record, types.ArtifactPhaseRefresh, "", "")
	_ = os.Remove(s.toolOperationFile(toolID))
	return s.buildInstallState(ctx, toolID)
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
	outcome := s.executeToolProcess(ctx, executable, prepared.Directory, input, definition.ControlCapabilities)
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

// recoverPendingOperation 按阶段处理上次中断的操作；不根据文件时间或目录猜测当前版本。
func (s *system) recoverPendingOperation(ctx context.Context, toolID string) error {
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
		if recordErr == nil && record.Result == release.OperationResultFailed && record.Artifact == identity {
			return s.operationState(identity, record.CurrentVersion, record.TargetVersion, types.ArtifactStatusFailed, record.Phase, record.ErrorCode, record.ErrorMessage)
		}
		return s.operationState(identity, "", "", types.ArtifactStatusFailed, types.ArtifactPhaseRestore, types.ArtifactErrorStateUnknown, "无法确认当前工具版本，不启用任何版本")
	}
	record, recordErr := release.ReadOperationRecord(s.toolOperationFile(toolID))
	failedRecord := recordErr == nil && record.Result == release.OperationResultFailed && record.Artifact == identity
	if failedRecord && !installed {
		return s.operationState(identity, record.CurrentVersion, record.TargetVersion, types.ArtifactStatusFailed, record.Phase, record.ErrorCode, record.ErrorMessage)
	}
	if !installed {
		return s.operationState(identity, "", "", types.ArtifactStatusNotInstalled, "", "", "")
	}
	tool, err := s.storage.LoadTool(ctx, toolID)
	if err != nil {
		state, _ := s.operationState(identity, currentVersion, "", types.ArtifactStatusUnavailable, "", types.ArtifactErrorStateUnknown, "工具程序资料不能进入正常业务："+err.Error())
		if failedRecord {
			state.Error = types.ReleaseOperationError{Code: record.ErrorCode, Phase: record.Phase, Message: record.ErrorMessage}
		}
		return state, toolStorageFailed("failed to load installed tool", err)
	}
	tool = s.annotateTool(tool)
	state, _ := s.operationState(identity, currentVersion, "", types.ArtifactStatusActive, "", "", "")
	if !tool.Compatibility.Compatible {
		state, _ = s.operationState(identity, currentVersion, "", types.ArtifactStatusUnavailable, "", types.ArtifactErrorCompatibility, tool.Compatibility.Reason)
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
