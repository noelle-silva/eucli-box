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
	decision, err := s.permission.Decide(ctx, roleID, action)
	if err != nil {
		return types.ToolRunPlan{}, toolPermissionFailed("failed to decide tool permission", err)
	}
	switch decision.Status {
	case types.PermissionStatusDenied:
		return types.ToolRunPlan{ID: utils.NewID("tool-plan"), Action: action, Tool: tool, Decision: decision, PlanStatus: types.ToolPlanStatusDenied, CreatedAt: time.Now().UTC()}, nil
	case types.PermissionStatusAllowed:
	case types.PermissionStatusNeedsConfirmation:
		return types.ToolRunPlan{ID: utils.NewID("tool-plan"), Action: action, Tool: tool, Decision: decision, PlanStatus: types.ToolPlanStatusNeedsConfirmation, CreatedAt: time.Now().UTC()}, nil
	default:
		return types.ToolRunPlan{}, toolPermissionFailed("unexpected permission status", nil)
	}
	if err := ensureToolDirectory(tool); err != nil {
		return types.ToolRunPlan{}, err
	}
	executable, err := selectExecutable(tool)
	if err != nil {
		return types.ToolRunPlan{}, err
	}
	return types.ToolRunPlan{ID: utils.NewID("tool-plan"), Action: action, Tool: tool, Decision: decision, PlanStatus: types.ToolPlanStatusReady, Executable: executable, CreatedAt: time.Now().UTC()}, nil
}

func (s *system) ApplyConfirmation(ctx context.Context, plan types.ToolRunPlan, confirmation types.ToolConfirmation) (types.ToolRunPlan, error) {
	if plan.PlanStatus != types.ToolPlanStatusNeedsConfirmation {
		return types.ToolRunPlan{}, toolInvalid("tool plan is not waiting for confirmation", nil)
	}
	if confirmation.DecisionID != plan.Decision.ID {
		return types.ToolRunPlan{}, toolInvalid("confirmation decision id does not match plan", nil)
	}
	if !confirmation.Approved {
		return types.ToolRunPlan{ID: plan.ID, Action: plan.Action, Tool: plan.Tool, Decision: types.PermissionDecision{ID: plan.Decision.ID, ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.PermissionStatusDenied, Reason: confirmationReason(confirmation), CreatedAt: time.Now().UTC()}, PlanStatus: types.ToolPlanStatusDenied, CreatedAt: time.Now().UTC()}, nil
	}
	decision, err := s.permission.ApplyConfirmation(ctx, plan.Decision, confirmation)
	if err != nil {
		return types.ToolRunPlan{}, toolPermissionFailed("failed to apply tool confirmation", err)
	}
	if decision.Status == types.PermissionStatusDenied {
		plan.Decision = decision
		plan.PlanStatus = types.ToolPlanStatusDenied
		return plan, nil
	}
	plan.Decision = decision
	if err := ensureToolDirectory(plan.Tool); err != nil {
		return types.ToolRunPlan{}, err
	}
	executable, err := selectExecutable(plan.Tool)
	if err != nil {
		return types.ToolRunPlan{}, err
	}
	plan.PlanStatus = types.ToolPlanStatusReady
	plan.Executable = executable
	plan.CreatedAt = time.Now().UTC()
	return plan, nil
}

func confirmationReason(confirmation types.ToolConfirmation) string {
	if strings.TrimSpace(confirmation.Reason) != "" {
		return confirmation.Reason
	}
	return "user rejected tool action"
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
