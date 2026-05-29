package toolcalling

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func (s *system) Prepare(ctx context.Context, roleID string, action types.ToolAction) (types.ToolRunPlan, error) {
	if strings.TrimSpace(roleID) == "" {
		return types.ToolRunPlan{}, toolInvalid("role id is required", nil)
	}
	if err := validateAction(action); err != nil {
		return types.ToolRunPlan{}, err
	}
	tool, err := s.resolveTool(ctx, action.ToolName)
	if err != nil {
		return types.ToolRunPlan{}, err
	}
	if err := validateToolDefinition(tool); err != nil {
		return types.ToolRunPlan{}, err
	}
	if err := ensureToolDirectory(tool); err != nil {
		return types.ToolRunPlan{}, err
	}
	executable, err := selectExecutable(tool)
	if err != nil {
		return types.ToolRunPlan{}, err
	}
	decision, err := s.permission.Decide(ctx, roleID, action)
	if err != nil {
		return types.ToolRunPlan{}, toolPermissionFailed("failed to decide tool permission", err)
	}
	plan := types.ToolRunPlan{ID: utils.NewID("tool-plan"), Action: action, Tool: tool, Decision: decision, Executable: executable, CreatedAt: time.Now().UTC()}
	if decision.Status == types.PermissionStatusDenied {
		plan.Status = types.ToolStatusDenied
	}
	return plan, nil
}

func (s *system) ApplyConfirmation(ctx context.Context, plan types.ToolRunPlan, confirmation types.ToolConfirmation) (types.ToolRunPlan, error) {
	if plan.Decision.Status != types.PermissionStatusNeedsConfirmation {
		return types.ToolRunPlan{}, toolInvalid("tool plan is not waiting for confirmation", nil)
	}
	decision, err := s.permission.ApplyConfirmation(ctx, plan.Decision, confirmation)
	if err != nil {
		return types.ToolRunPlan{}, toolPermissionFailed("failed to apply tool confirmation", err)
	}
	plan.Decision = decision
	if decision.Status == types.PermissionStatusDenied {
		plan.Status = types.ToolStatusDenied
	}
	return plan, nil
}

func validateAction(action types.ToolAction) error {
	if strings.TrimSpace(action.ID) == "" {
		return toolInvalid("tool action id is required", nil)
	}
	if strings.TrimSpace(action.ToolName) == "" {
		return toolInvalid("tool action requires tool name", nil)
	}
	return nil
}
