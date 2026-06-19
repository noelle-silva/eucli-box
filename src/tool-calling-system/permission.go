package toolcalling

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func (s *system) Prepare(ctx context.Context, roleID string, workspaceID string, action types.ToolAction) (types.ToolRunPlan, error) {
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
	fence, err := s.evaluateWorkspaceFence(ctx, workspaceID, tool, action)
	if err != nil {
		return types.ToolRunPlan{}, err
	}
	if fence != nil && fence.RequiresConfirmation {
		decision := workspaceFenceDecision(action, fence)
		return types.ToolRunPlan{ID: utils.NewID("tool-plan"), RoleID: roleID, Action: action, Tool: tool, InvocationMode: resolveInvocationMode(action, tool), Decision: decision, WorkspaceFence: fence, PlanStatus: types.ToolPlanStatusNeedsConfirmation, CreatedAt: time.Now().UTC()}, nil
	}
	decision, err := s.permission.Decide(ctx, roleID, action)
	if err != nil {
		return types.ToolRunPlan{}, toolPermissionFailed("failed to decide tool permission", err)
	}
	return s.planFromDecision(roleID, action, tool, decision, fence)
}

func (s *system) planFromDecision(roleID string, action types.ToolAction, tool types.ToolDefinition, decision types.PermissionDecision, fence *types.ToolWorkspaceFence) (types.ToolRunPlan, error) {
	invocationMode := resolveInvocationMode(action, tool)
	switch decision.Status {
	case types.PermissionStatusDenied:
		return types.ToolRunPlan{ID: utils.NewID("tool-plan"), RoleID: roleID, Action: action, Tool: tool, InvocationMode: invocationMode, Decision: decision, WorkspaceFence: fence, PlanStatus: types.ToolPlanStatusDenied, CreatedAt: time.Now().UTC()}, nil
	case types.PermissionStatusNeedsConfirmation:
		return types.ToolRunPlan{ID: utils.NewID("tool-plan"), RoleID: roleID, Action: action, Tool: tool, InvocationMode: invocationMode, Decision: decision, WorkspaceFence: fence, PlanStatus: types.ToolPlanStatusNeedsConfirmation, CreatedAt: time.Now().UTC()}, nil
	case types.PermissionStatusAllowed:
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
	return types.ToolRunPlan{ID: utils.NewID("tool-plan"), RoleID: roleID, Action: action, Tool: tool, InvocationMode: invocationMode, Decision: decision, WorkspaceFence: fence, PlanStatus: types.ToolPlanStatusReady, Executable: executable, CreatedAt: time.Now().UTC()}, nil
}

func resolveInvocationMode(action types.ToolAction, tool types.ToolDefinition) types.ToolInvocationMode {
	if strings.TrimSpace(string(action.InvocationMode)) != "" {
		return types.NormalizeToolInvocationMode(action.InvocationMode)
	}
	return types.CleanToolInvocationMode(tool.DefaultInvocationMode)
}

func (s *system) ApplyConfirmation(ctx context.Context, plan types.ToolRunPlan, confirmation types.ToolConfirmation) (types.ToolRunPlan, error) {
	if plan.PlanStatus != types.ToolPlanStatusNeedsConfirmation {
		return types.ToolRunPlan{}, toolInvalid("tool plan is not waiting for confirmation", nil)
	}
	if confirmation.DecisionID != plan.Decision.ID {
		return types.ToolRunPlan{}, toolInvalid("confirmation decision id does not match plan", nil)
	}
	if !confirmation.Approved {
		return types.ToolRunPlan{ID: plan.ID, RoleID: plan.RoleID, Action: plan.Action, Tool: plan.Tool, InvocationMode: plan.InvocationMode, Decision: types.PermissionDecision{ID: plan.Decision.ID, ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.PermissionStatusDenied, Reason: confirmationReason(confirmation), Details: plan.Decision.Details, CreatedAt: time.Now().UTC()}, WorkspaceFence: plan.WorkspaceFence, PlanStatus: types.ToolPlanStatusDenied, CreatedAt: time.Now().UTC()}, nil
	}
	if isWorkspaceFenceDecision(plan.Decision) {
		if strings.TrimSpace(plan.RoleID) == "" {
			return types.ToolRunPlan{}, toolInvalid("role id is required", nil)
		}
		if plan.WorkspaceFence != nil {
			plan.WorkspaceFence.RequiresConfirmation = false
		}
		decision, err := s.permission.Decide(ctx, plan.RoleID, plan.Action)
		if err != nil {
			return types.ToolRunPlan{}, toolPermissionFailed("failed to decide tool permission", err)
		}
		return s.planFromDecision(plan.RoleID, plan.Action, plan.Tool, decision, plan.WorkspaceFence)
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
	if decision.Status == types.PermissionStatusNeedsConfirmation {
		plan.PlanStatus = types.ToolPlanStatusNeedsConfirmation
		plan.CreatedAt = time.Now().UTC()
		return plan, nil
	}
	return s.planFromDecision(plan.RoleID, plan.Action, plan.Tool, decision, plan.WorkspaceFence)
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
	if !types.ValidToolInvocationMode(action.InvocationMode) {
		return toolInvalid("tool invocation mode must be sync or async", nil)
	}
	return nil
}

func isWorkspaceFenceDecision(decision types.PermissionDecision) bool {
	if len(decision.Details) == 0 {
		return false
	}
	_, ok := decision.Details["workspaceFence"]
	return ok
}
