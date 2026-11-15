package toolcalling

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) Execute(ctx context.Context, plan types.ToolRunPlan) (types.ToolResult, error) {
	return s.ExecuteWithOutputUpdate(ctx, plan, nil)
}

func (s *system) ExecuteWithOutputUpdate(ctx context.Context, plan types.ToolRunPlan, onUpdate func(update types.ToolOutputUpdate)) (types.ToolResult, error) {
	if plan.PlanStatus == types.ToolPlanStatusDenied {
		return deniedResult(plan, plan.Decision.Reason), nil
	}
	if plan.PlanStatus != types.ToolPlanStatusReady {
		return types.ToolResult{}, toolExecutionInvalid("tool plan is not ready for execution", nil)
	}
	if err := ctx.Err(); err != nil {
		return types.ToolResult{}, toolExecutionInvalid("execution cancelled", err)
	}
	if err := validatePlan(plan); err != nil {
		return types.ToolResult{}, err
	}
	if plan.Tool.Status == types.ToolAvailabilityUnavailable {
		return deniedResult(plan, plan.Tool.StatusMessage), nil
	}
	if err := ensureToolBodyDirectory(plan.Tool); err != nil {
		return types.ToolResult{}, err
	}
	if resolved, err := selectExecutable(plan.Tool); err != nil {
		return types.ToolResult{}, err
	} else if resolved != plan.Executable {
		return types.ToolResult{}, toolExecutionInvalid("tool executable path has changed since prepare", nil)
	}
	_, executableErr := cleanExecutablePath(plan.Tool, plan.Executable)
	if executableErr != nil {
		return types.ToolResult{}, executableErr
	}
	activity := s.activityFor(plan.Tool.ID)
	if blocked := activity.acquire(); blocked != "" {
		return types.ToolResult{}, toolExecutionInvalid(blocked, nil)
	}
	defer activity.release()
	// 工具的工作目录 = 配置的默认工作目录：无工作区会话下工具在此干活，
	// 有工作区的会话由注入的工作区路径优先覆盖，绝不落入部署目录。
	workDirectory, err := s.toolWorkDirectory(ctx)
	if err != nil {
		return types.ToolResult{}, err
	}
	timeoutMs := requestedTimeoutMs(plan.Action.Arguments)
	executionInput := types.ToolExecutionInput{ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Arguments: plan.Action.Arguments, UserConfig: plan.Tool.UserConfig, DefaultConfig: plan.Tool.DefaultConfig, ToolBodyDirectory: plan.Tool.BodyDirectory, ToolDataDirectory: plan.Tool.DataDirectory, HostWorkingDirectory: workDirectory, TimeoutMs: timeoutMs}
	workspace, err := s.workspaceContextForTool(ctx, plan)
	if err != nil {
		return types.ToolResult{}, err
	}
	executionInput.Workspace = workspace
	executionInput.WorkspaceID = s.workspaceIdentityForTool(plan)
	input, err := json.Marshal(executionInput)
	if err != nil {
		return types.ToolResult{}, toolExecutionInvalid("failed to encode tool input", err)
	}
	startedAt := time.Now()
	capabilities := newCapabilitySession(plan, s.storage, s.messageSystem(), s.confirmationBridge())
	live := &toolLiveOutput{}
	outcome := s.executeToolProcess(ctx, plan.Tool.ID, plan.Executable, plan.Tool.BodyDirectory, input, capabilities, func(update types.ToolOutputUpdate) {
		live.observe(update)
		var relayed types.ToolOutputUpdate
		relayed.CallID = plan.Action.ID
		relayed.ToolName = plan.Action.ToolName
		relayed.Bytes = update.Bytes
		relayed.Preview = update.Preview
		if onUpdate != nil {
			onUpdate(relayed)
		}
	})
	result := toolResultFromOutcome(plan, outcome, live.previewSnapshot())
	// 产出附件随结果返回：附件在写入时已由宿主校验与落盘，
	// 即使本次执行随后失败，已完成的附件仍是有效产物。
	result.ProducedAttachments = capabilities.attachments()
	result.DurationMs = time.Since(startedAt).Milliseconds()
	return result, nil
}

// workspaceIdentityForTool 只对声明了工作区身份能力的工具注入工作区标识；
// 不在工作区会话里就没有标识可注入，工具拿不到这个字段。
func (s *system) workspaceIdentityForTool(plan types.ToolRunPlan) string {
	if !types.ToolDeclaresCapability(plan.Tool, types.ToolCapabilityWorkspaceIdentity, types.ToolCapabilityAccessRead) {
		return ""
	}
	return strings.TrimSpace(plan.Scope.WorkspaceID)
}

// workspaceContextForTool 只对声明了工作区能力的工具注入工作区路径值；
// 未声明的工具拿不到这个值，也永远不会收到该字段。
func (s *system) workspaceContextForTool(ctx context.Context, plan types.ToolRunPlan) (*types.ToolWorkspaceContext, error) {
	if !types.ToolDeclaresCapability(plan.Tool, types.ToolCapabilityWorkspace, types.ToolCapabilityAccessRead) {
		return nil, nil
	}
	workspaceID := strings.TrimSpace(plan.Scope.WorkspaceID)
	if workspaceID == "" {
		return nil, nil
	}
	workspace, err := s.storage.LoadWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, toolStorageFailed("failed to load workspace for tool runtime", err)
	}
	return &types.ToolWorkspaceContext{ID: workspace.ID, Name: workspace.Name, Directories: workspace.Directories}, nil
}

// toolResultFromOutcome 把一次工具进程执行的真实终局翻译成统一的工具结果事实。
// 它只翻译终局，不决定执行耗时——耗时由执行边界统一测量后写入。
//
// 所有终局共用同一份「已产出内容」：取消、失联、协议失败、进程异常退出，
// 工具跑出来的东西都必须留在正文里，收尾说明只追加在后。
func toolResultFromOutcome(plan types.ToolRunPlan, outcome toolProcessOutcome, producedPreview string) types.ToolResult {
	produced := producedToolContent(outcome.Stdout, producedPreview)
	switch outcome.FailureKind {
	case "user_cancelled":
		return toolCancelledResult(plan, "tool execution cancelled", produced, outcome.FailureError)
	case "tool_unresponsive":
		return toolFailureResult(plan, protocolFailureMessage("tool execution became unresponsive", outcome.FailureError), produced, outcome.FailureKind, outcome.FailureError)
	case "tool_protocol_failed":
		return toolFailureResult(plan, protocolFailureMessage("tool control protocol failed", outcome.FailureError), produced, outcome.FailureKind, outcome.FailureError)
	}
	if outcome.ExitError != nil {
		message := outcome.ExitError.Error()
		if stderr := strings.TrimSpace(string(outcome.Stderr)); stderr != "" {
			message += ": " + stderr
		}
		if outcome.FailureError != nil {
			message += ": " + outcome.FailureError.Error()
		}
		return failedResult(plan, message, produced)
	}
	if outcome.FailureError != nil {
		return failedResult(plan, "tool process failed: "+outcome.FailureError.Error(), produced)
	}
	return parseToolOutput(plan, outcome.Stdout, producedPreview)
}

// producedToolContent 还原工具在落定之前已经产出的内容。
// 工具自己写出的完整成功输出最完整，优先用它；
// 其次用实时尾窗；两者都没有时退回 stdout 原文，绝不让已有的输出凭空消失。
func producedToolContent(stdout []byte, producedPreview string) string {
	if content, settled := parsedOutputContent(stdout); settled {
		return content
	}
	return rawOutputText(stdout, producedPreview)
}

// rawOutputText 是「工具写出的原始输出」的兜底文本：
// 实时尾窗通常是人类可读的进度，优先；没有尾窗就用 stdout 原文。
func rawOutputText(stdout []byte, producedPreview string) string {
	if preview := strings.TrimSpace(producedPreview); preview != "" {
		return preview
	}
	return strings.TrimSpace(string(stdout))
}

// parsedOutputContent 判断 stdout 是不是一份完整的工具输出。
// 完整输出说明工具已经自己给出了终局：产出只可能是它携带的成功内容，
// 其余状态没有「已产出内容」可言；解析不出来时 settled 为 false，
// 交给调用方退回原始文本。
func parsedOutputContent(stdout []byte) (string, bool) {
	payload := bytes.TrimSpace(stdout)
	if len(payload) == 0 {
		return "", false
	}
	var output types.ToolExecutionOutput
	if err := json.Unmarshal(payload, &output); err != nil {
		return "", false
	}
	if output.Status != types.ToolStatusSuccess {
		return "", true
	}
	return strings.TrimSpace(output.Content), true
}

// protocolFailureMessage 在协议层失败消息后附加可用的真实原因，
// 让"工具未握手退出"等场景不再吞掉工具自身的错误输出。
func protocolFailureMessage(message string, cause error) string {
	if cause == nil {
		return message
	}
	if text := strings.TrimSpace(cause.Error()); text != "" {
		return message + ": " + text
	}
	return message
}

// requestedTimeoutMs extracts the caller-specified tool budget from the action
// arguments when present; zero means the caller did not specify one.
func requestedTimeoutMs(arguments map[string]any) int64 {
	if arguments == nil {
		return 0
	}
	switch value := arguments["timeoutMs"].(type) {
	case float64:
		if value <= 0 {
			return 0
		}
		return int64(value)
	case int:
		if value <= 0 {
			return 0
		}
		return int64(value)
	case int64:
		if value <= 0 {
			return 0
		}
		return value
	default:
		return 0
	}
}

func validatePlan(plan types.ToolRunPlan) error {
	if plan.Tool.ID == "" {
		return toolExecutionInvalid("tool plan is missing tool definition", nil)
	}
	if plan.Action.ID == "" || plan.Action.ToolName == "" {
		return toolExecutionInvalid("tool plan is missing action", nil)
	}
	if plan.Decision.Status != types.PermissionStatusAllowed {
		return toolExecutionInvalid("tool plan decision is not allowed", nil)
	}
	return nil
}

// parseToolOutput 把工具的最终输出解释成结果事实。
// 输出不成形时同样要保住已经写出来的东西：正文用实时尾窗或 stdout 原文兜底，
// 不让一句「不是合法 JSON」把工具的输出抹掉。
func parseToolOutput(plan types.ToolRunPlan, raw []byte, producedPreview string) types.ToolResult {
	var output types.ToolExecutionOutput
	if err := json.Unmarshal(bytes.TrimSpace(raw), &output); err != nil {
		return failedResult(plan, "tool output is not valid json", rawOutputText(raw, producedPreview))
	}
	switch output.Status {
	case types.ToolStatusSuccess:
		return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.ToolStatusSuccess, Content: output.Content, Metadata: output.Metadata, CreatedAt: time.Now().UTC()}
	case types.ToolStatusFailed, types.ToolStatusDenied, types.ToolStatusCancelled:
		errMsg := output.Error
		if errMsg == "" {
			errMsg = output.Content
		}
		return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: output.Status, Content: output.Content, Metadata: output.Metadata, Error: errMsg, CreatedAt: time.Now().UTC()}
	default:
		return failedResult(plan, "tool output status is invalid", rawOutputText(raw, producedPreview))
	}
}
