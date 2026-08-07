package toolcalling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) Execute(ctx context.Context, plan types.ToolRunPlan) (types.ToolResult, error) {
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
	hostWorkingDirectory, err := os.Getwd()
	if err != nil {
		return types.ToolResult{}, toolExecutionInvalid("failed to resolve host working directory", err)
	}
	input, err := json.Marshal(types.ToolExecutionInput{ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Arguments: plan.Action.Arguments, UserConfig: plan.Tool.UserConfig, DefaultConfig: plan.Tool.DefaultConfig, ToolBodyDirectory: plan.Tool.BodyDirectory, ToolDataDirectory: plan.Tool.DataDirectory, HostWorkingDirectory: hostWorkingDirectory})
	if err != nil {
		return types.ToolResult{}, toolExecutionInvalid("failed to encode tool input", err)
	}
	toolCtx, cancel := context.WithTimeout(ctx, s.config.ToolTimeout)
	defer cancel()
	cmd := exec.CommandContext(toolCtx, plan.Executable)
	cmd.Dir = plan.Tool.BodyDirectory
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

func parseToolOutput(plan types.ToolRunPlan, raw []byte) types.ToolResult {
	var output types.ToolExecutionOutput
	if err := json.Unmarshal(bytes.TrimSpace(raw), &output); err != nil {
		return failedResult(plan, "tool output is not valid json")
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
		return failedResult(plan, "tool output status is invalid")
	}
}
