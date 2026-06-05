package toolcalling

import (
	"context"
	"time"

	"eucli-box/pkg/types"
)

type System interface {
	TextToolInstructions(ctx context.Context, tools []types.ToolDefinition) (types.PromptMessage, error)
	ParseTextToolRequests(ctx context.Context, content string) ([]types.ToolIntent, error)
	NormalizeIntent(ctx context.Context, intent types.ToolIntent) (types.ToolAction, error)
	Prepare(ctx context.Context, roleID string, action types.ToolAction) (types.ToolRunPlan, error)
	ApplyConfirmation(ctx context.Context, plan types.ToolRunPlan, confirmation types.ToolConfirmation) (types.ToolRunPlan, error)
	Execute(ctx context.Context, plan types.ToolRunPlan) (types.ToolResult, error)
	SaveTool(ctx context.Context, tool types.ToolDefinition) error
	LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error)
	ListTools(ctx context.Context) ([]types.ToolSummary, error)
	SaveToolUserConfig(ctx context.Context, toolID string, userConfig map[string]any) (types.ToolDefinition, error)
}

type PermissionSystem interface {
	Decide(ctx context.Context, roleID string, action types.ToolAction) (types.PermissionDecision, error)
	ApplyConfirmation(ctx context.Context, decision types.PermissionDecision, confirmation types.ToolConfirmation) (types.PermissionDecision, error)
}

type StorageSystem interface {
	SaveTool(ctx context.Context, tool types.ToolDefinition) error
	LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error)
	ListTools(ctx context.Context) ([]types.ToolSummary, error)
	SaveToolUserConfig(ctx context.Context, toolID string, userConfig map[string]any) (types.ToolDefinition, error)
}

type Config struct {
	ToolTimeout time.Duration
}

type system struct {
	config     Config
	permission PermissionSystem
	storage    StorageSystem
}

func NewSystem(config Config, permission PermissionSystem, storage StorageSystem) (System, error) {
	if permission == nil {
		return nil, toolInvalid("permission system dependency is required", nil)
	}
	if storage == nil {
		return nil, toolInvalid("storage system dependency is required", nil)
	}
	if config.ToolTimeout < 0 {
		return nil, toolInvalid("tool timeout cannot be negative", nil)
	}
	if config.ToolTimeout == 0 {
		config.ToolTimeout = 120 * time.Second
	}
	return &system{config: config, permission: permission, storage: storage}, nil
}
