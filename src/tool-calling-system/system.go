package toolcalling

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"eucli-box/internal/boxrelease"
	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

type System interface {
	TextToolInstructions(ctx context.Context, tools []types.ToolDefinition) (types.PromptMessage, error)
	ParseTextToolRequests(ctx context.Context, content string) ([]types.ToolIntent, error)
	NormalizeIntent(ctx context.Context, intent types.ToolIntent) (types.ToolAction, error)
	Prepare(ctx context.Context, roleID string, workspaceID string, action types.ToolAction) (types.ToolRunPlan, error)
	ApplyConfirmation(ctx context.Context, plan types.ToolRunPlan, confirmation types.ToolConfirmation) (types.ToolRunPlan, error)
	Execute(ctx context.Context, plan types.ToolRunPlan) (types.ToolResult, error)
	ExecuteWithOutputUpdate(ctx context.Context, plan types.ToolRunPlan, onUpdate func(update types.ToolOutputUpdate)) (types.ToolResult, error)
	SaveTool(ctx context.Context, tool types.ToolDefinition) error
	LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error)
	ListTools(ctx context.Context) ([]types.ToolSummary, error)
	SaveToolUserSettings(ctx context.Context, toolID string, settings types.ToolUserSettings) (types.ToolDefinition, error)

	InstallTool(ctx context.Context, toolID string) (types.ArtifactInstallState, error)
	UpdateTool(ctx context.Context, toolID string) (types.ArtifactInstallState, error)
	ToolInstallState(ctx context.Context, toolID string) (types.ArtifactInstallState, error)
	ToolActivity(ctx context.Context, toolID string) (types.ArtifactActivityState, error)
}

type PermissionSystem interface {
	Decide(ctx context.Context, roleID string, action types.ToolAction) (types.PermissionDecision, error)
	ApplyConfirmation(ctx context.Context, decision types.PermissionDecision, confirmation types.ToolConfirmation) (types.PermissionDecision, error)
}

type StorageSystem interface {
	SaveTool(ctx context.Context, tool types.ToolDefinition) error
	LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error)
	ListTools(ctx context.Context) ([]types.ToolSummary, error)
	SaveToolUserSettings(ctx context.Context, toolID string, settings types.ToolUserSettings) (types.ToolDefinition, error)
	LoadWorkspace(ctx context.Context, workspaceID string) (types.Workspace, error)
}

type Config struct {
	LegacyToolTimeout        time.Duration
	ToolWatchdogTimeout      time.Duration
	ToolWatchdogPingInterval time.Duration
	BoxVersion               string
	ProgramRoot              string
	Candidates               releasecheck.CandidateReader
	HTTPClient               release.HTTPDoer
}

type system struct {
	config            Config
	boxVersion        string
	permission        PermissionSystem
	storage           StorageSystem
	activities        map[string]*toolActivity
	updateWaitTimeout time.Duration
	mu                sync.Mutex
}

func NewSystem(config Config, permission PermissionSystem, storage StorageSystem) (System, error) {
	if permission == nil {
		return nil, toolInvalid("permission system dependency is required", nil)
	}
	if storage == nil {
		return nil, toolInvalid("storage system dependency is required", nil)
	}
	if config.LegacyToolTimeout <= 0 {
		config.LegacyToolTimeout = 120 * time.Second
	}
	if config.ToolWatchdogTimeout <= 0 {
		config.ToolWatchdogTimeout = 60 * time.Second
	}
	if config.ToolWatchdogPingInterval <= 0 {
		config.ToolWatchdogPingInterval = 10 * time.Second
	}
	if config.ToolWatchdogPingInterval >= config.ToolWatchdogTimeout {
		return nil, toolInvalid("tool watchdog ping interval must be less than watchdog timeout", nil)
	}
	boxVersion := strings.TrimSpace(config.BoxVersion)
	if boxVersion == "" {
		info, err := boxrelease.Load()
		if err != nil {
			return nil, toolInvalid("eucli-box 发布资料无效", err)
		}
		boxVersion = info.Version
	}
	if err := release.ValidateVersion(boxVersion); err != nil {
		return nil, toolInvalid(fmt.Sprintf("eucli-box 版本无效：%v", err), err)
	}
	programRoot := strings.TrimSpace(config.ProgramRoot)
	if programRoot != "" {
		if config.Candidates == nil {
			return nil, toolInvalid("official candidate reader is required for managed tool programs", nil)
		}
		if config.HTTPClient == nil {
			return nil, toolInvalid("download client is required for managed tool programs", nil)
		}
		absolute, absErr := filepath.Abs(programRoot)
		if absErr != nil {
			return nil, toolInvalid("program root is invalid", absErr)
		}
		config.ProgramRoot = absolute
	}
	return &system{config: config, boxVersion: boxVersion, permission: permission, storage: storage, activities: map[string]*toolActivity{}, updateWaitTimeout: defaultUpdateWaitTimeout}, nil
}
