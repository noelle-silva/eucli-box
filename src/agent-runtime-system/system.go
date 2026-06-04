package agentruntime

import (
	"context"
	"sync"
	"time"

	"eucli-box/pkg/types"
)

type System interface {
	StartRun(ctx context.Context, request types.RunRequest) (types.RunState, error)
	SubmitToolConfirmation(ctx context.Context, confirmation types.ToolConfirmation) error
	CancelRun(ctx context.Context, runID string) error
	GetRun(ctx context.Context, runID string) (types.RunState, error)
	Subscribe(ctx context.Context) (<-chan types.RunEvent, func(), error)
}

type StorageSystem interface {
	SaveSession(ctx context.Context, session types.Session) error
	LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error)
	SaveSessionMessageAttachment(ctx context.Context, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error)
	LoadSessionAttachmentImage(ctx context.Context, relPath string) (string, error)
}

type RoleSystem interface {
	BuildContext(ctx context.Context, roleID string, session types.Session, tools []types.ToolDefinition) (types.RoleContext, error)
	GetToolPolicy(ctx context.Context, roleID string) (types.ToolPolicy, error)
}

type ProviderSystem interface {
	Complete(ctx context.Context, request types.ModelRequest) (types.ModelResponse, error)
	CompleteStream(ctx context.Context, request types.ModelRequest, onEvent types.ModelStreamHandler) (types.ModelResponse, error)
}

type ToolSystem interface {
	VisibleTextToolContent(ctx context.Context, content string) (string, error)
	ParseTextToolRequests(ctx context.Context, content string) (string, []types.ToolIntent, error)
	NormalizeIntent(ctx context.Context, intent types.ToolIntent) (types.ToolAction, error)
	Prepare(ctx context.Context, roleID string, action types.ToolAction) (types.ToolRunPlan, error)
	ApplyConfirmation(ctx context.Context, plan types.ToolRunPlan, confirmation types.ToolConfirmation) (types.ToolRunPlan, error)
	Execute(ctx context.Context, plan types.ToolRunPlan) (types.ToolResult, error)
	LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error)
	ListTools(ctx context.Context) ([]types.ToolSummary, error)
}

type Config struct {
	MaxSteps         int
	ToolTimeout      time.Duration
	MaxParallelTools int
}

type system struct {
	config    Config
	storage   StorageSystem
	roles     RoleSystem
	providers ProviderSystem
	tools     ToolSystem

	mu          sync.Mutex
	runs        map[string]*runRecord
	subscribers map[chan types.RunEvent]struct{}
}

type runRecord struct {
	runID             string
	roleID            string
	state             types.RunState
	session           types.Session
	messageParent     types.Message
	inputMessageID    string
	lastMessageID     string
	activeAssistantID string
	stream            bool
	streamContent     string
	cancel            context.CancelFunc

	pendingPlans   map[string]types.ToolRunPlan
	confirmationCh chan types.ToolConfirmation
}

func NewSystem(config Config, storage StorageSystem, roles RoleSystem, providers ProviderSystem, tools ToolSystem) (System, error) {
	if storage == nil {
		return nil, runtimeInvalid("storage system dependency is required", nil)
	}
	if roles == nil {
		return nil, runtimeInvalid("role system dependency is required", nil)
	}
	if providers == nil {
		return nil, runtimeInvalid("provider system dependency is required", nil)
	}
	if tools == nil {
		return nil, runtimeInvalid("tool system dependency is required", nil)
	}
	if config.MaxSteps <= 0 {
		config.MaxSteps = 8
	}
	if config.ToolTimeout < 0 {
		return nil, runtimeInvalid("tool timeout cannot be negative", nil)
	}
	if config.ToolTimeout == 0 {
		config.ToolTimeout = 120 * time.Second
	}
	if config.MaxParallelTools < 0 {
		return nil, runtimeInvalid("max parallel tools cannot be negative", nil)
	}
	if config.MaxParallelTools == 0 {
		config.MaxParallelTools = 4
	}
	return &system{config: config, storage: storage, roles: roles, providers: providers, tools: tools, runs: map[string]*runRecord{}, subscribers: map[chan types.RunEvent]struct{}{}}, nil
}

func (s *system) getRunState(runID string) (types.RunState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.runs[runID]
	if !ok {
		return types.RunState{}, false
	}
	return record.state, true
}

func (s *system) removeSubscriber(ch chan types.RunEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subscribers[ch]; ok {
		delete(s.subscribers, ch)
		close(ch)
	}
}
