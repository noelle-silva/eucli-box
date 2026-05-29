package permission

import (
	"context"
	"errors"
	"testing"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

func TestDecideAllowsWhitelistedDirectTool(t *testing.T) {
	system := newTestPermissionSystem(t, &fakeRoleSystem{
		policy:   types.ToolPolicy{Mode: types.ToolPolicyWhitelist, Tools: []string{"file-reader"}},
		runModes: map[string]types.ToolRunMode{"file-reader": types.ToolRunDirect},
	})
	decision, err := system.Decide(context.Background(), "developer", action("file-reader"))
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Status != types.PermissionStatusAllowed {
		t.Fatalf("status = %s", decision.Status)
	}
}

func TestDecideDeniesToolOutsideWhitelist(t *testing.T) {
	system := newTestPermissionSystem(t, &fakeRoleSystem{policy: types.ToolPolicy{Mode: types.ToolPolicyWhitelist, Tools: []string{"file-reader"}}})
	decision, err := system.Decide(context.Background(), "developer", action("web-search"))
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Status != types.PermissionStatusDenied {
		t.Fatalf("status = %s", decision.Status)
	}
}

func TestDecideDeniesBlacklistedTool(t *testing.T) {
	system := newTestPermissionSystem(t, &fakeRoleSystem{policy: types.ToolPolicy{Mode: types.ToolPolicyBlacklist, Tools: []string{"shell"}}})
	decision, err := system.Decide(context.Background(), "developer", action("shell"))
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Status != types.PermissionStatusDenied {
		t.Fatalf("status = %s", decision.Status)
	}
}

func TestDecideAsksForConfirmation(t *testing.T) {
	system := newTestPermissionSystem(t, &fakeRoleSystem{
		policy:   types.ToolPolicy{Mode: types.ToolPolicyBlacklist},
		runModes: map[string]types.ToolRunMode{"web-search": types.ToolRunAsk},
	})
	decision, err := system.Decide(context.Background(), "developer", action("web-search"))
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Status != types.PermissionStatusNeedsConfirmation {
		t.Fatalf("status = %s", decision.Status)
	}
}

func TestApplyConfirmationAllowsApprovedDecision(t *testing.T) {
	system := newTestPermissionSystem(t, &fakeRoleSystem{})
	decision := types.PermissionDecision{ID: "decision-1", Status: types.PermissionStatusNeedsConfirmation, ToolName: "web-search", ActionID: "action-1"}
	confirmed, err := system.ApplyConfirmation(context.Background(), decision, types.ToolConfirmation{DecisionID: "decision-1", Approved: true})
	if err != nil {
		t.Fatalf("ApplyConfirmation() error = %v", err)
	}
	if confirmed.Status != types.PermissionStatusAllowed {
		t.Fatalf("status = %s", confirmed.Status)
	}
}

func TestApplyConfirmationRejectsMismatchedDecision(t *testing.T) {
	system := newTestPermissionSystem(t, &fakeRoleSystem{})
	decision := types.PermissionDecision{ID: "decision-1", Status: types.PermissionStatusNeedsConfirmation}
	_, err := system.ApplyConfirmation(context.Background(), decision, types.ToolConfirmation{DecisionID: "other", Approved: true})
	assertAppErrorCode(t, err, "permission.confirmation_invalid")
}

func TestDecideReturnsDeniedDecisionAndErrorWhenRoleReadFails(t *testing.T) {
	system := newTestPermissionSystem(t, &fakeRoleSystem{policyErr: errors.New("role unavailable")})
	decision, err := system.Decide(context.Background(), "developer", action("file-reader"))
	assertAppErrorCode(t, err, "permission.role_failed")
	if decision.Status != types.PermissionStatusDenied {
		t.Fatalf("status = %s", decision.Status)
	}
}

func TestDecideRejectsInvalidAction(t *testing.T) {
	system := newTestPermissionSystem(t, &fakeRoleSystem{})
	_, err := system.Decide(context.Background(), "developer", types.ToolAction{ToolName: "file-reader"})
	assertAppErrorCode(t, err, "permission.invalid_request")
}

func action(toolName string) types.ToolAction {
	return types.ToolAction{ID: "action-1", ToolName: toolName}
}

func newTestPermissionSystem(t *testing.T, roles RoleSystem) System {
	t.Helper()
	system, err := NewSystem(Config{}, roles)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	return system
}

type fakeRoleSystem struct {
	policy     types.ToolPolicy
	runModes   map[string]types.ToolRunMode
	policyErr  error
	runModeErr error
}

func (f *fakeRoleSystem) GetToolPolicy(ctx context.Context, roleID string) (types.ToolPolicy, error) {
	if f.policyErr != nil {
		return types.ToolPolicy{}, f.policyErr
	}
	return f.policy, nil
}

func (f *fakeRoleSystem) GetToolRunMode(ctx context.Context, roleID string, toolName string) (types.ToolRunMode, error) {
	if f.runModeErr != nil {
		return "", f.runModeErr
	}
	mode, ok := f.runModes[toolName]
	if !ok {
		return "", errors.New("run mode missing")
	}
	return mode, nil
}

func assertAppErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not AppError", err)
	}
	if appErr.Code != code {
		t.Fatalf("code = %s, want %s", appErr.Code, code)
	}
}
