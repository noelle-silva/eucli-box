package agentruntime

import (
	"context"

	"eucli-box/pkg/types"
)

func (s *system) handleToolIntent(ctx context.Context, record *runRecord, intent types.ToolIntent) (types.ToolResult, bool, error) {
	action, err := s.tools.NormalizeIntent(ctx, intent)
	if err != nil {
		return types.ToolResult{}, false, runtimeToolFailed("failed to normalize tool intent", err)
	}
	plan, err := s.tools.Prepare(ctx, record.state.RoleID, action)
	if err != nil {
		return types.ToolResult{}, false, runtimeToolFailed("failed to prepare tool run plan", err)
	}
	s.publish(record.state.ID, "tool_requested", plan)
	if plan.Decision.Status == types.PermissionStatusDenied {
		result := types.ToolResult{ID: newRuntimeID("tool-result"), ActionID: action.ID, ToolName: action.ToolName, Status: types.ToolStatusDenied, Error: plan.Decision.Reason, CreatedAt: nowUTC()}
		return result, false, nil
	}
	if plan.Decision.Status == types.PermissionStatusNeedsConfirmation {
		confirmed, err := s.waitForConfirmation(ctx, record, plan)
		if err != nil {
			return types.ToolResult{}, false, err
		}
		plan = confirmed
		if plan.Decision.Status == types.PermissionStatusDenied {
			result := types.ToolResult{ID: newRuntimeID("tool-result"), ActionID: action.ID, ToolName: action.ToolName, Status: types.ToolStatusDenied, Error: plan.Decision.Reason, CreatedAt: nowUTC()}
			return result, false, nil
		}
	}
	toolCtx, cancel := context.WithTimeout(ctx, s.config.ToolTimeout)
	defer cancel()
	result, err := s.tools.Execute(toolCtx, plan)
	if err != nil {
		return types.ToolResult{}, false, runtimeToolFailed("failed to execute tool", err)
	}
	return result, false, nil
}

func newRuntimeID(prefix string) string {
	return prefix + "-" + nowUTC().Format("20060102150405.000000000")
}
