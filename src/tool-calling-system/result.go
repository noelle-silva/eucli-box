package toolcalling

import (
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func newToolResultID() string {
	return utils.NewID("tool-result")
}

func failedResult(plan types.ToolRunPlan, message string) types.ToolResult {
	return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.ToolStatusFailed, Error: message, CreatedAt: time.Now().UTC()}
}

func toolFailureResult(plan types.ToolRunPlan, message string, kind string, terminationError error) types.ToolResult {
	metadata := map[string]any{"failureKind": kind}
	if terminationError != nil {
		metadata["terminationError"] = terminationError.Error()
	}
	return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.ToolStatusFailed, Content: message, Error: message, Metadata: metadata, CreatedAt: time.Now().UTC()}
}

func toolCancelledResult(plan types.ToolRunPlan, message string, terminationError error) types.ToolResult {
	metadata := map[string]any{"failureKind": "user_cancelled"}
	if terminationError != nil {
		metadata["terminationError"] = terminationError.Error()
	}
	return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.ToolStatusCancelled, Content: message, Error: message, Metadata: metadata, CreatedAt: time.Now().UTC()}
}

func deniedResult(plan types.ToolRunPlan, message string) types.ToolResult {
	return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.ToolStatusDenied, Error: message, CreatedAt: time.Now().UTC()}
}

func cancelledResult(plan types.ToolRunPlan, message string) types.ToolResult {
	return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.ToolStatusCancelled, Error: message, CreatedAt: time.Now().UTC()}
}
