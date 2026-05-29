package permission

import (
	"context"
	"fmt"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func (s *system) Decide(ctx context.Context, roleID string, action types.ToolAction) (types.PermissionDecision, error) {
	if err := validateAction(action); err != nil {
		return types.PermissionDecision{}, err
	}
	decision := newDecision(action)
	policy, err := s.roles.GetToolPolicy(ctx, roleID)
	if err != nil {
		return deny(decision, "failed to read role tool policy"), permissionRoleFailed("failed to read role tool policy", err)
	}
	allowed, reason, err := isAllowedByPolicy(policy, action.ToolName)
	if err != nil {
		return deny(decision, reason), err
	}
	if !allowed {
		return deny(decision, reason), nil
	}
	mode, err := s.roles.GetToolRunMode(ctx, roleID, action.ToolName)
	if err != nil {
		return deny(decision, "failed to read tool run mode"), permissionRoleFailed("failed to read tool run mode", err)
	}
	switch mode {
	case types.ToolRunDirect:
		return allow(decision, reason), nil
	case types.ToolRunAsk:
		return needsConfirmation(decision, "tool requires user confirmation"), nil
	default:
		return deny(decision, fmt.Sprintf("unsupported tool run mode %q", mode)), permissionInvalid("unsupported tool run mode", nil)
	}
}

func newDecision(action types.ToolAction) types.PermissionDecision {
	return types.PermissionDecision{
		ID:        utils.NewID("permission"),
		ActionID:  action.ID,
		ToolName:  action.ToolName,
		Status:    types.PermissionStatusDenied,
		Reason:    "not decided",
		CreatedAt: time.Now().UTC(),
	}
}

func allow(decision types.PermissionDecision, reason string) types.PermissionDecision {
	decision.Status = types.PermissionStatusAllowed
	decision.Reason = reason
	return decision
}

func deny(decision types.PermissionDecision, reason string) types.PermissionDecision {
	decision.Status = types.PermissionStatusDenied
	decision.Reason = reason
	return decision
}

func needsConfirmation(decision types.PermissionDecision, reason string) types.PermissionDecision {
	decision.Status = types.PermissionStatusNeedsConfirmation
	decision.Reason = reason
	return decision
}
