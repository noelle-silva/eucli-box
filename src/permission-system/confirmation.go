package permission

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) ApplyConfirmation(ctx context.Context, decision types.PermissionDecision, confirmation types.ToolConfirmation) (types.PermissionDecision, error) {
	if strings.TrimSpace(decision.ID) == "" {
		return types.PermissionDecision{}, permissionConfirmationInvalid("decision id is required", nil)
	}
	if decision.Status != types.PermissionStatusNeedsConfirmation {
		return types.PermissionDecision{}, permissionConfirmationInvalid("only pending confirmation decisions can apply confirmation", nil)
	}
	if confirmation.DecisionID != decision.ID {
		return types.PermissionDecision{}, permissionConfirmationInvalid("confirmation decision id does not match", nil)
	}
	if confirmation.Approved {
		return allow(decision, "user approved tool action"), nil
	}
	return deny(decision, confirmationReason(confirmation)), nil
}

func confirmationReason(confirmation types.ToolConfirmation) string {
	if strings.TrimSpace(confirmation.Reason) != "" {
		return confirmation.Reason
	}
	return "user rejected tool action"
}
