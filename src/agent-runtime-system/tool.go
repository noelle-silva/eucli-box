package agentruntime

import (
	"context"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) handleToolIntent(ctx context.Context, record *runRecord, intent types.ToolIntent) (types.ToolResult, error) {
	action, err := s.tools.NormalizeIntent(ctx, intent)
	if err != nil {
		return types.ToolResult{}, runtimeToolFailed("failed to normalize tool intent", err)
	}
	appendRunMessage(record, toolRequestMessage(action))
	if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
		return types.ToolResult{}, err
	}
	plan, err := s.tools.Prepare(ctx, record.roleID, action)
	if err != nil {
		return types.ToolResult{}, runtimeToolFailed("failed to prepare tool run plan", err)
	}
	s.publish(record.runID, "tool_requested", plan)
	if plan.Decision.Status == types.PermissionStatusDenied {
		result := types.ToolResult{ID: newRuntimeID("tool-result"), ActionID: action.ID, ToolName: action.ToolName, Status: types.ToolStatusDenied, Error: plan.Decision.Reason, CreatedAt: time.Now().UTC()}
		appendRunMessage(record, toolMessage(result))
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
			return types.ToolResult{}, err
		}
		return result, nil
	}
	if plan.Decision.Status == types.PermissionStatusNeedsConfirmation {
		confirmed, err := s.waitForConfirmation(ctx, record, plan)
		if err != nil {
			result := types.ToolResult{ID: newRuntimeID("tool-result"), ActionID: action.ID, ToolName: action.ToolName, Status: types.ToolStatusCancelled, Error: err.Error(), CreatedAt: time.Now().UTC()}
			appendRunMessage(record, toolMessage(result))
			if stateErr := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); stateErr != nil {
				return types.ToolResult{}, stateErr
			}
			return result, err
		}
		plan = confirmed
		appendRunMessage(record, toolConfirmationMessage(plan.Decision))
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
			return types.ToolResult{}, err
		}
		if plan.Decision.Status == types.PermissionStatusDenied {
			result := types.ToolResult{ID: newRuntimeID("tool-result"), ActionID: action.ID, ToolName: action.ToolName, Status: types.ToolStatusDenied, Error: plan.Decision.Reason, CreatedAt: time.Now().UTC()}
			appendRunMessage(record, toolMessage(result))
			if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
				return types.ToolResult{}, err
			}
			return result, nil
		}
	}
	toolCtx, cancel := context.WithTimeout(ctx, s.config.ToolTimeout)
	defer cancel()
	result, err := s.tools.Execute(toolCtx, plan)
	if err != nil {
		failedResult := types.ToolResult{ID: newRuntimeID("tool-result"), ActionID: action.ID, ToolName: action.ToolName, Status: types.ToolStatusFailed, Error: err.Error(), CreatedAt: time.Now().UTC()}
		appendRunMessage(record, toolMessage(failedResult))
		if stateErr := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); stateErr != nil {
			return types.ToolResult{}, stateErr
		}
		return failedResult, runtimeToolFailed("failed to execute tool", err)
	}
	appendRunMessage(record, toolMessage(result))
	if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
		return types.ToolResult{}, err
	}
	return result, nil
}

func newRuntimeID(prefix string) string {
	return prefix + "-" + time.Now().UTC().Format("20060102150405.000000000")
}
