package gateway

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"eucli-box/pkg/types"
)

type System interface {
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
	Handler() http.Handler
}

type RuntimeSystem interface {
	StartRun(ctx context.Context, request types.RunRequest) (types.RunState, error)
	SubmitToolConfirmation(ctx context.Context, confirmation types.ToolConfirmation) error
	CancelRun(ctx context.Context, runID string) error
	GetRun(ctx context.Context, runID string) (types.RunState, error)
	Subscribe(ctx context.Context) (<-chan types.RunEvent, func(), error)
}

type RoleSystem interface {
	SaveRole(ctx context.Context, role types.Role) error
	LoadRole(ctx context.Context, roleID string) (types.Role, error)
	ListRoles(ctx context.Context) ([]types.RoleSummary, error)
	DeleteRole(ctx context.Context, roleID string) error
}

type ProviderSystem interface {
	SaveProvider(ctx context.Context, provider types.Provider) error
	LoadProvider(ctx context.Context, providerID string) (types.Provider, error)
	ListProviders(ctx context.Context) ([]types.ProviderSummary, error)
	DeleteProvider(ctx context.Context, providerID string) error
	RefreshModels(ctx context.Context, providerID string) ([]types.ModelInfo, error)
}

type ToolSystem interface {
	SaveTool(ctx context.Context, tool types.ToolDefinition) error
	LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error)
	ListTools(ctx context.Context) ([]types.ToolSummary, error)
}

type Config struct {
	Addr         string
	Key          string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type system struct {
	config    Config
	runtime   RuntimeSystem
	roles     RoleSystem
	providers ProviderSystem
	tools     ToolSystem
	mux       *http.ServeMux
	server    *http.Server
	upgrader  websocket.Upgrader

	wsMu        sync.Mutex
	connections map[*websocket.Conn]struct{}
}

func NewSystem(config Config, runtime RuntimeSystem, roles RoleSystem, providers ProviderSystem, tools ToolSystem) (System, error) {
	if runtime == nil {
		return nil, gatewayInvalid("runtime system dependency is required", nil)
	}
	if roles == nil {
		return nil, gatewayInvalid("role system dependency is required", nil)
	}
	if providers == nil {
		return nil, gatewayInvalid("provider system dependency is required", nil)
	}
	if tools == nil {
		return nil, gatewayInvalid("tool system dependency is required", nil)
	}
	if config.Addr == "" {
		config.Addr = "127.0.0.1:8765"
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 15 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 60 * time.Second
	}
	if config.ReadTimeout < 0 || config.WriteTimeout < 0 {
		return nil, gatewayInvalid("server timeouts cannot be negative", nil)
	}
	s := &system{
		config:      config,
		runtime:     runtime,
		roles:       roles,
		providers:   providers,
		tools:       tools,
		mux:         http.NewServeMux(),
		upgrader:    websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		connections: map[*websocket.Conn]struct{}{},
	}
	s.registerRoutes()
	s.server = &http.Server{Addr: config.Addr, Handler: s.mux, ReadTimeout: config.ReadTimeout, WriteTimeout: config.WriteTimeout}
	return s, nil
}

func (s *system) Handler() http.Handler {
	return s.mux
}
