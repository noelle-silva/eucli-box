package toolcalling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"time"

	"eucli-box/pkg/types"
)

type toolInput struct {
	ActionID      string         `json:"actionId"`
	ToolName      string         `json:"toolName"`
	Arguments     map[string]any `json:"arguments"`
	UserConfig    map[string]any `json:"userConfig"`
	ToolDirectory string         `json:"toolDirectory"`
}

type toolOutput struct {
	Status   types.ToolStatus `json:"status"`
	Content  string           `json:"content"`
	Metadata map[string]any   `json:"metadata"`
}

func (s *system) Execute(ctx context.Context, plan types.ToolRunPlan) (types.ToolResult, error) {
	if plan.Decision.Status == types.PermissionStatusDenied {
		return deniedResult(plan, plan.Decision.Reason), nil
	}
	if plan.Decision.Status == types.PermissionStatusNeedsConfirmation {
		return types.ToolResult{}, toolExecutionInvalid("tool plan still needs confirmation", nil)
	}
	if plan.Decision.Status != types.PermissionStatusAllowed {
		return types.ToolResult{}, toolExecutionInvalid("tool plan is not allowed", nil)
	}
	if plan.Executable == "" {
		return types.ToolResult{}, toolExecutionInvalid("tool executable is required", nil)
	}
	input, err := json.Marshal(toolInput{ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Arguments: plan.Action.Arguments, UserConfig: plan.Tool.UserConfig, ToolDirectory: plan.Tool.Directory})
	if err != nil {
		return types.ToolResult{}, toolExecutionInvalid("failed to encode tool input", err)
	}
	toolCtx, cancel := context.WithTimeout(ctx, s.config.ToolTimeout)
	defer cancel()
	cmd := exec.CommandContext(toolCtx, plan.Executable)
	cmd.Dir = plan.Tool.Directory
	cmd.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if errors.Is(toolCtx.Err(), context.DeadlineExceeded) {
		return failedResult(plan, "tool execution timed out"), nil
	}
	if err != nil {
		message := err.Error()
		if stderr.Len() > 0 {
			message = message + ": " + stderr.String()
		}
		return failedResult(plan, message), nil
	}
	return parseToolOutput(plan, stdout.Bytes()), nil
}

func parseToolOutput(plan types.ToolRunPlan, raw []byte) types.ToolResult {
	var output toolOutput
	if err := json.Unmarshal(bytes.TrimSpace(raw), &output); err != nil {
		return failedResult(plan, "tool output is not valid json")
	}
	switch output.Status {
	case types.ToolStatusSuccess, types.ToolStatusFailed, types.ToolStatusDenied, types.ToolStatusCancelled:
		return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: output.Status, Content: output.Content, Metadata: output.Metadata, CreatedAt: time.Now().UTC()}
	default:
		return failedResult(plan, "tool output status is invalid")
	}
}
