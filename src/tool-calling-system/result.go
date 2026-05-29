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

func deniedResult(plan types.ToolRunPlan, message string) types.ToolResult {
	return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.ToolStatusDenied, Error: message, CreatedAt: time.Now().UTC()}
}

func cancelledResult(plan types.ToolRunPlan, message string) types.ToolResult {
	return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.ToolStatusCancelled, Error: message, CreatedAt: time.Now().UTC()}
}
