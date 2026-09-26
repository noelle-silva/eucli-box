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
	NormalizeIntent(ctx context.Context, intent types.ToolIntent) (types.ToolAction, error)
	Prepare(ctx context.Context, scope types.ToolRunScope, action types.ToolAction) (types.ToolRunPlan, error)
	ApplyConfirmation(ctx context.Context, plan types.ToolRunPlan, confirmation types.ToolConfirmation) (types.ToolRunPlan, error)
	Execute(ctx context.Context, plan types.ToolRunPlan) (types.ToolResult, error)
	ExecuteWithOutputUpdate(ctx context.Context, plan types.ToolRunPlan, onUpdate func(update types.ToolOutputUpdate)) (types.ToolResult, error)
	SaveTool(ctx context.Context, tool types.ToolDefinition) error
	LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error)
	ListTools(ctx context.Context) ([]types.ToolSummary, error)
	SaveToolUserSettings(ctx context.Context, toolID string, settings types.ToolUserSettings) (types.ToolDefinition, error)
	LoadToolWorkDirectoryConfig(ctx context.Context) (types.ToolWorkDirectoryConfig, error)
	SaveToolWorkDirectoryConfig(ctx context.Context, config types.ToolWorkDirectoryConfig) (types.ToolWorkDirectoryConfig, error)

	InstallTool(ctx context.Context, toolID string) (types.ArtifactInstallState, error)
	UpdateTool(ctx context.Context, toolID string) (types.ArtifactInstallState, error)
	ToolInstallState(ctx context.Context, toolID string) (types.ArtifactInstallState, error)
	CancelToolOperation(ctx context.Context, toolID string) (types.ArtifactInstallState, error)
	ListToolOperations(ctx context.Context) ([]types.ArtifactInstallState, error)
	ToolActivity(ctx context.Context, toolID string) (types.ArtifactActivityState, error)
	StopToolExecution(ctx context.Context, toolID string) (types.ToolStopResult, error)

	StartWarmup(ctx context.Context) error
	Shutdown(ctx context.Context) error
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
	LoadToolWorkDirectoryConfig(ctx context.Context) (types.ToolWorkDirectoryConfig, error)
	SaveToolWorkDirectoryConfig(ctx context.Context, config types.ToolWorkDirectoryConfig) (types.ToolWorkDirectoryConfig, error)
	LoadWorkspace(ctx context.Context, workspaceID string) (types.Workspace, error)
	LoadRole(ctx context.Context, roleID string) (types.Role, error)
	LoadChatGroup(ctx context.Context, groupID string) (types.ChatGroup, error)
	LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error)
	LoadGroupSession(ctx context.Context, groupID string, sessionID string) (types.Session, error)
	LoadWorkspaceSession(ctx context.Context, workspaceID string, roleID string, sessionID string) (types.Session, error)
	SaveSessionMessageAttachment(ctx context.Context, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error)
	SaveGroupSessionMessageAttachment(ctx context.Context, groupID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error)
	SaveWorkspaceSessionMessageAttachment(ctx context.Context, workspaceID string, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error)
	LoadSessionAttachmentImage(ctx context.Context, relPath string) (string, error)
}

type Config struct {
	ToolWatchdogTimeout      time.Duration
	ToolWatchdogPingInterval time.Duration
	ToolWarmupInterval       time.Duration
	BoxVersion               string
	ProgramRoot              string
	Candidates               releasecheck.CandidateReader
	HTTPClient               release.HTTPDoer
}

type system struct {
	config           Config
	boxVersion       string
	permission       PermissionSystem
	storage          StorageSystem
	activities       map[string]*toolActivity
	activeExecutions map[string]map[*toolRunContext]struct{}
	mu               sync.Mutex
	warmupMu         sync.Mutex
	warmupCancel     context.CancelFunc
	warmupDone       chan struct{}
}

// toolRunContext cancels one tool execution from a user-facing stop action.
type toolRunContext struct {
	cancel context.CancelFunc
}

func NewSystem(config Config, permission PermissionSystem, storage StorageSystem) (System, error) {
	if permission == nil {
		return nil, toolInvalid("permission system dependency is required", nil)
	}
	if storage == nil {
		return nil, toolInvalid("storage system dependency is required", nil)
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
	if config.ToolWarmupInterval <= 0 {
		config.ToolWarmupInterval = 15 * time.Minute
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
	return &system{config: config, boxVersion: boxVersion, permission: permission, storage: storage, activities: map[string]*toolActivity{}, activeExecutions: map[string]map[*toolRunContext]struct{}{}}, nil
}
