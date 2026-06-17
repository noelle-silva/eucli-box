package roleprompt

import (
	"context"

	"eucli-box/pkg/types"
)

type System interface {
	SaveRole(ctx context.Context, role types.Role) error
	LoadRole(ctx context.Context, roleID string) (types.Role, error)
	ListRoles(ctx context.Context) ([]types.RoleSummary, error)
	DeleteRole(ctx context.Context, roleID string) error
	SaveRoleAvatar(ctx context.Context, roleID string, dataURL string) error
	LoadRoleAvatar(ctx context.Context, roleID string) (string, error)
	DeleteRoleAvatar(ctx context.Context, roleID string) error

	BuildContext(ctx context.Context, roleID string, session types.Session, tools []types.ToolDefinition) (types.RoleContext, error)
	GetToolPolicy(ctx context.Context, roleID string) (types.ToolPolicy, error)
	GetToolRunMode(ctx context.Context, roleID string, toolName string) (types.ToolRunMode, error)
}

type StorageSystem interface {
	SaveRole(ctx context.Context, role types.Role) error
	LoadRole(ctx context.Context, roleID string) (types.Role, error)
	ListRoles(ctx context.Context) ([]types.RoleSummary, error)
	DeleteRole(ctx context.Context, roleID string) error
	SaveRoleAvatar(ctx context.Context, roleID string, dataURL string) error
	LoadRoleAvatar(ctx context.Context, roleID string) (string, error)
	DeleteRoleAvatar(ctx context.Context, roleID string) error
	LoadChatGroup(ctx context.Context, groupID string) (types.ChatGroup, error)
	LoadWorkspace(ctx context.Context, workspaceID string) (types.Workspace, error)
}

type ProviderSystem interface {
	ResolveModel(ctx context.Context, coordinate types.ModelCoordinate) (types.Provider, types.ModelInfo, error)
}

type Config struct{}

type system struct {
	storage   StorageSystem
	providers ProviderSystem
}

func NewSystem(config Config, storage StorageSystem, providers ProviderSystem) (System, error) {
	if storage == nil {
		return nil, roleInvalid("storage system dependency is required", nil)
	}
	if providers == nil {
		return nil, roleInvalid("provider system dependency is required", nil)
	}
	return &system{storage: storage, providers: providers}, nil
}
