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
	ListActiveRuns(ctx context.Context) ([]types.RunState, error)
	Subscribe(ctx context.Context) (<-chan types.RunEvent, func(), error)
}

type RoleSystem interface {
	SaveRole(ctx context.Context, role types.Role) error
	LoadRole(ctx context.Context, roleID string) (types.Role, error)
	ListRoles(ctx context.Context) ([]types.RoleSummary, error)
	DeleteRole(ctx context.Context, roleID string) error
	SaveRoleAvatar(ctx context.Context, roleID string, dataURL string) error
	LoadRoleAvatar(ctx context.Context, roleID string) (string, error)
	DeleteRoleAvatar(ctx context.Context, roleID string) error
}

type ProviderSystem interface {
	SaveProvider(ctx context.Context, provider types.Provider) error
	LoadProvider(ctx context.Context, providerID string) (types.Provider, error)
	ListProviders(ctx context.Context) ([]types.ProviderSummary, error)
	DeleteProvider(ctx context.Context, providerID string) error
	LoadModelRequestConfig(ctx context.Context) (types.ModelRequestConfig, error)
	SaveModelRequestConfig(ctx context.Context, config types.ModelRequestConfig) (types.ModelRequestConfig, error)
	LoadModelGroups(ctx context.Context) ([]types.ModelGroup, error)
	SaveModelGroups(ctx context.Context, groups []types.ModelGroup) ([]types.ModelGroup, error)
	RefreshModels(ctx context.Context, providerID string) ([]types.ModelInfo, error)
}

type ToolSystem interface {
	SaveTool(ctx context.Context, tool types.ToolDefinition) error
	LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error)
	ListTools(ctx context.Context) ([]types.ToolSummary, error)
	SaveToolUserConfig(ctx context.Context, toolID string, userConfig map[string]any) (types.ToolDefinition, error)
}

type SessionSystem interface {
	CreateSession(ctx context.Context, roleID string, title string) (types.Session, error)
	SaveSession(ctx context.Context, session types.Session) error
	LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error)
	ListSessions(ctx context.Context, roleID string) ([]types.SessionSummary, error)
	DeleteSession(ctx context.Context, roleID string, sessionID string) error
	UpdateSessionTitle(ctx context.Context, roleID string, sessionID string, title string) (types.Session, error)
	UpdateSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string, patch types.SessionMessagePatch) (types.Message, error)
	DeleteSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string) (types.Session, error)
	DeleteSessionMessageSubtree(ctx context.Context, roleID string, sessionID string, messageID string) (types.Session, error)
	LoadSessionAttachmentImage(ctx context.Context, relPath string) (string, error)
	LoadSessionFavorites(ctx context.Context) (types.SessionFavorites, error)
	SaveSessionFavorites(ctx context.Context, favorites types.SessionFavorites) (types.SessionFavorites, error)
}

type StickerSystem interface {
	CreateStickerCategory(ctx context.Context, categoryName string) (types.StickerCategory, error)
	ListStickerCategories(ctx context.Context) ([]types.StickerCategorySummary, error)
	LoadStickerCategory(ctx context.Context, categoryName string) (types.StickerCategory, error)
	LoadStickerLibrary(ctx context.Context) (types.StickerLibrary, error)
	AddSticker(ctx context.Context, categoryName string, stickerName string, dataURL string) (types.StickerItem, error)
	RenameSticker(ctx context.Context, categoryName string, oldStickerName string, newStickerName string) (types.StickerItem, error)
	DeleteSticker(ctx context.Context, categoryName string, stickerName string) error
	DeleteStickerCategory(ctx context.Context, categoryName string) error
	LoadStickerImage(ctx context.Context, relPath string) (string, error)
	LoadStickerNamingConfig(ctx context.Context) (types.StickerNamingConfig, error)
	SaveStickerNamingConfig(ctx context.Context, config types.StickerNamingConfig) (types.StickerNamingConfig, error)
	LoadMermaidFixConfig(ctx context.Context) (types.MermaidFixConfig, error)
	SaveMermaidFixConfig(ctx context.Context, config types.MermaidFixConfig) (types.MermaidFixConfig, error)
	LoadChatTitleNamingConfig(ctx context.Context) (types.ChatTitleNamingConfig, error)
	SaveChatTitleNamingConfig(ctx context.Context, config types.ChatTitleNamingConfig) (types.ChatTitleNamingConfig, error)
	LoadContextCompressionConfig(ctx context.Context) (types.ContextCompressionConfig, error)
	SaveContextCompressionConfig(ctx context.Context, config types.ContextCompressionConfig) (types.ContextCompressionConfig, error)
}

type AIAssistSystem interface {
	GenerateStickerName(ctx context.Context, request types.StickerNameRequest) (types.StickerNameResult, error)
	GenerateChatTitle(ctx context.Context, request types.ChatTitleRequest) (types.ChatTitleResult, error)
	FixMermaidInMessage(ctx context.Context, request types.MermaidFixRequest) (types.MermaidFixResult, error)
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
	sessions  SessionSystem
	stickers  StickerSystem
	assist    AIAssistSystem
	mux       *http.ServeMux
	server    *http.Server
	upgrader  websocket.Upgrader

	wsMu        sync.Mutex
	connections map[*websocket.Conn]struct{}
}

func NewSystem(config Config, runtime RuntimeSystem, roles RoleSystem, providers ProviderSystem, tools ToolSystem, sessions SessionSystem, stickers StickerSystem, assist AIAssistSystem) (System, error) {
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
	if sessions == nil {
		return nil, gatewayInvalid("session system dependency is required", nil)
	}
	if stickers == nil {
		return nil, gatewayInvalid("sticker system dependency is required", nil)
	}
	if assist == nil {
		return nil, gatewayInvalid("ai assist system dependency is required", nil)
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
		sessions:    sessions,
		stickers:    stickers,
		assist:      assist,
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
