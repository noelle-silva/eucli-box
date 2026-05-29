package permission

import (
	"context"

	"eucli-box/pkg/types"
)

type System interface {
	Decide(ctx context.Context, roleID string, action types.ToolAction) (types.PermissionDecision, error)
	ApplyConfirmation(ctx context.Context, decision types.PermissionDecision, confirmation types.ToolConfirmation) (types.PermissionDecision, error)
}

type RoleSystem interface {
	GetToolPolicy(ctx context.Context, roleID string) (types.ToolPolicy, error)
	GetToolRunMode(ctx context.Context, roleID string, toolName string) (types.ToolRunMode, error)
}

type Config struct{}

type system struct {
	roles RoleSystem
}

func NewSystem(config Config, roles RoleSystem) (System, error) {
	if roles == nil {
		return nil, permissionInvalid("role system dependency is required", nil)
	}
	return &system{roles: roles}, nil
}
