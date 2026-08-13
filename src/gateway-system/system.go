package gateway

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"eucli-box/internal/boxrelease"
	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

type System interface {
	Start(ctx context.Context) error
	StartLocal(ctx context.Context) (LocalStartResult, error)
	Shutdown(ctx context.Context) error
	Handler() http.Handler
	// LongTermHandler 返回长期端口使用的统一鉴权与业务处理入口：
	// 请求先经过长期 Key 核对，再进入正常业务路由；访问设置路由仍要求受托凭证。
	LongTermHandler() http.Handler
}

type LocalStartResult struct {
	Endpoint string
}

type LocalRunInfo struct {
	InstallIdentity  string
	DataIdentity     string
	RunIdentity      string
	ProcessID        int
	ProcessStartedAt time.Time
	Version          string
}

type RuntimeSystem interface {
	StartRun(ctx context.Context, request types.RunRequest) (types.RunState, error)
	SubmitToolConfirmation(ctx context.Context, confirmation types.ToolConfirmation) error
	CancelRun(ctx context.Context, runID string) error
	GetRun(ctx context.Context, runID string) (types.RunState, error)
	ListActiveRuns(ctx context.Context) ([]types.RunState, error)
	ListAsyncToolTasks(ctx context.Context, query types.AsyncToolTaskQuery) ([]types.AsyncToolTask, error)
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

type ChatGroupSystem interface {
	SaveChatGroup(ctx context.Context, group types.ChatGroup) error
	LoadChatGroup(ctx context.Context, groupID string) (types.ChatGroup, error)
	ListChatGroups(ctx context.Context) ([]types.ChatGroupSummary, error)
	DeleteChatGroup(ctx context.Context, groupID string) error
	SaveChatGroupAvatar(ctx context.Context, groupID string, dataURL string) error
	LoadChatGroupAvatar(ctx context.Context, groupID string) (string, error)
	DeleteChatGroupAvatar(ctx context.Context, groupID string) error
}

type WorkspaceSystem interface {
	SaveWorkspace(ctx context.Context, workspace types.Workspace) error
	LoadWorkspace(ctx context.Context, workspaceID string) (types.Workspace, error)
	ListWorkspaces(ctx context.Context) ([]types.WorkspaceSummary, error)
	PreviewWorkspacePrompt(ctx context.Context, workspace types.Workspace) (string, error)
	DeleteWorkspace(ctx context.Context, workspaceID string) error
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
	SaveToolUserSettings(ctx context.Context, toolID string, settings types.ToolUserSettings) (types.ToolDefinition, error)
	InstallTool(ctx context.Context, toolID string) (types.ArtifactInstallState, error)
	UpdateTool(ctx context.Context, toolID string) (types.ArtifactInstallState, error)
	ToolInstallState(ctx context.Context, toolID string) (types.ArtifactInstallState, error)
	ToolActivity(ctx context.Context, toolID string) (types.ArtifactActivityState, error)
}

type SessionSystem interface {
	CreateSession(ctx context.Context, roleID string, title string) (types.Session, error)
	CreateGroupSession(ctx context.Context, groupID string, title string) (types.Session, error)
	CreateWorkspaceSession(ctx context.Context, workspaceID string, roleID string, title string) (types.Session, error)
	SaveSession(ctx context.Context, session types.Session) error
	LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error)
	LoadGroupSession(ctx context.Context, groupID string, sessionID string) (types.Session, error)
	LoadWorkspaceSession(ctx context.Context, workspaceID string, roleID string, sessionID string) (types.Session, error)
	ListSessions(ctx context.Context, roleID string) ([]types.SessionSummary, error)
	ListGroupSessions(ctx context.Context, groupID string) ([]types.SessionSummary, error)
	ListWorkspaceSessions(ctx context.Context, workspaceID string, roleID string) ([]types.SessionSummary, error)
	DeleteSession(ctx context.Context, roleID string, sessionID string) error
	DeleteGroupSession(ctx context.Context, groupID string, sessionID string) error
	DeleteWorkspaceSession(ctx context.Context, workspaceID string, roleID string, sessionID string) error
	UpdateSessionTitle(ctx context.Context, roleID string, sessionID string, title string) (types.Session, error)
	UpdateGroupSessionTitle(ctx context.Context, groupID string, sessionID string, title string) (types.Session, error)
	UpdateWorkspaceSessionTitle(ctx context.Context, workspaceID string, roleID string, sessionID string, title string) (types.Session, error)
	UpdateSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string, patch types.SessionMessagePatch) (types.Message, error)
	UpdateGroupSessionMessage(ctx context.Context, groupID string, sessionID string, messageID string, patch types.SessionMessagePatch) (types.Message, error)
	UpdateWorkspaceSessionMessage(ctx context.Context, workspaceID string, roleID string, sessionID string, messageID string, patch types.SessionMessagePatch) (types.Message, error)
	DeleteSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string) (types.Session, error)
	DeleteGroupSessionMessage(ctx context.Context, groupID string, sessionID string, messageID string) (types.Session, error)
	DeleteWorkspaceSessionMessage(ctx context.Context, workspaceID string, roleID string, sessionID string, messageID string) (types.Session, error)
	DeleteSessionMessageSubtree(ctx context.Context, roleID string, sessionID string, messageID string) (types.Session, error)
	DeleteGroupSessionMessageSubtree(ctx context.Context, groupID string, sessionID string, messageID string) (types.Session, error)
	DeleteWorkspaceSessionMessageSubtree(ctx context.Context, workspaceID string, roleID string, sessionID string, messageID string) (types.Session, error)
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

type HookPromptSystem interface {
	LoadHookPromptLibrary(ctx context.Context) (types.HookPromptLibrary, error)
	SaveHookPromptLibrary(ctx context.Context, library types.HookPromptLibrary) (types.HookPromptLibrary, error)
}

type PlaceholderSystem interface {
	LoadPlaceholderLibrary(ctx context.Context) (types.PlaceholderLibrary, error)
	SavePlaceholderLibrary(ctx context.Context, library types.PlaceholderLibrary) (types.PlaceholderLibrary, error)
	ResolveText(ctx context.Context, text string) (types.PlaceholderResolveResult, error)
	Problems(ctx context.Context) ([]types.PlaceholderProblem, error)
	DependencyTree(ctx context.Context, name string) (types.PlaceholderDependencyNode, error)
}

type SystemPluginSystem interface {
	ListPlugins(ctx context.Context) ([]types.SystemPluginSummary, error)
	LoadPlugin(ctx context.Context, pluginID string) (types.SystemPluginView, error)
	SavePluginUserConfig(ctx context.Context, pluginID string, config types.SystemPluginUserConfig) (types.SystemPluginView, error)
	AvailablePlaceholderInterfaces(ctx context.Context, library types.PlaceholderLibrary) ([]types.SystemPluginAvailablePlaceholderInterface, error)
	CreatePlaceholderFromInterface(ctx context.Context, library types.PlaceholderLibrary, pluginID string, interfaceID string) (types.PlaceholderLibrary, error)
	InstallPlugin(ctx context.Context, pluginID string) (types.ArtifactInstallState, error)
	UpdatePlugin(ctx context.Context, pluginID string) (types.ArtifactInstallState, error)
	PluginInstallState(ctx context.Context, pluginID string) (types.ArtifactInstallState, error)
	PluginActivity(ctx context.Context, pluginID string) (types.ArtifactActivityState, error)
}

type AIAssistSystem interface {
	GenerateStickerName(ctx context.Context, request types.StickerNameRequest) (types.StickerNameResult, error)
	GenerateChatTitle(ctx context.Context, request types.ChatTitleRequest) (types.ChatTitleResult, error)
	FixMermaidInMessage(ctx context.Context, request types.MermaidFixRequest) (types.MermaidFixResult, error)
}

type ReleaseCheckSystem interface {
	Snapshot() types.ReleaseCheckSnapshot
	Refresh(ctx context.Context, kind string) types.ReleaseCheckSnapshot
}

// AccessSystem 是业务端长期访问能力的网关视图：
// 长期端口与长期 Key 的 CRUD 以及长期 Key 核对。
type AccessSystem interface {
	ListPorts(ctx context.Context) ([]types.PersistentPort, error)
	AddPort(ctx context.Context, name string, port int) (types.PersistentPort, error)
	EnablePort(ctx context.Context, id string) (types.PersistentPort, error)
	DisablePort(ctx context.Context, id string) (types.PersistentPort, error)
	DeletePort(ctx context.Context, id string) error

	ListKeys(ctx context.Context) ([]types.PersistentKeyView, error)
	CreateKey(ctx context.Context, name string, expiresAt *string) (types.PersistentKeyCreated, error)
	RevealKey(ctx context.Context, id string) (string, error)
	SetKeyEnabled(ctx context.Context, id string, enabled bool) error
	SetKeyExpiration(ctx context.Context, id string, expiresAt *string) error
	DeleteKey(ctx context.Context, id string) error

	VerifyKey(ctx context.Context, providedKey string) types.PersistentKeyVerifyResult
	RegisterConnection(keyID string, closer interface{ Close() error })
	UnregisterConnection(keyID string, closer interface{ Close() error })
}

type Config struct {
	Addr              string
	Key               string
	BoxVersion        string
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	LocalRun          bool
	LocalInstallID    string
	LocalDataID       string
	LocalRunID        string
	LocalCredential   string
	LocalProcessID    int
	LocalProcessStart time.Time
	LocalStop         func()
	Access            AccessSystem
}

type system struct {
	config        Config
	boxRelease    types.EucliBoxRelease
	runtime       RuntimeSystem
	roles         RoleSystem
	groups        ChatGroupSystem
	workspaces    WorkspaceSystem
	providers     ProviderSystem
	tools         ToolSystem
	sessions      SessionSystem
	stickers      StickerSystem
	hooks         HookPromptSystem
	placeholders  PlaceholderSystem
	systemPlugins SystemPluginSystem
	assist        AIAssistSystem
	releaseChecks ReleaseCheckSystem
	access        AccessSystem
	mux           *http.ServeMux
	server        *http.Server
	upgrader      websocket.Upgrader
	localInfo     LocalRunInfo
	endpoint      string
	localStopOnce sync.Once

	wsMu        sync.Mutex
	connections map[*websocket.Conn]struct{}
}

func NewSystem(config Config, runtime RuntimeSystem, roles RoleSystem, groups ChatGroupSystem, workspaces WorkspaceSystem, providers ProviderSystem, tools ToolSystem, sessions SessionSystem, stickers StickerSystem, hooks HookPromptSystem, placeholders PlaceholderSystem, systemPlugins SystemPluginSystem, assist AIAssistSystem, releaseChecks ReleaseCheckSystem) (System, error) {
	if runtime == nil {
		return nil, gatewayInvalid("runtime system dependency is required", nil)
	}
	if roles == nil {
		return nil, gatewayInvalid("role system dependency is required", nil)
	}
	if groups == nil {
		return nil, gatewayInvalid("group system dependency is required", nil)
	}
	if workspaces == nil {
		return nil, gatewayInvalid("workspace system dependency is required", nil)
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
	if hooks == nil {
		return nil, gatewayInvalid("hook prompt system dependency is required", nil)
	}
	if placeholders == nil {
		return nil, gatewayInvalid("placeholder system dependency is required", nil)
	}
	if systemPlugins == nil {
		return nil, gatewayInvalid("system plugin system dependency is required", nil)
	}
	if assist == nil {
		return nil, gatewayInvalid("ai assist system dependency is required", nil)
	}
	if releaseChecks == nil {
		return nil, gatewayInvalid("release check system dependency is required", nil)
	}
	if config.LocalRun {
		if config.Addr == "" {
			config.Addr = "127.0.0.1:0"
		}
		if config.Addr != "127.0.0.1:0" {
			return nil, gatewayInvalid("受托模式地址必须为 127.0.0.1:0", nil)
		}
		if strings.TrimSpace(config.LocalCredential) == "" {
			return nil, gatewayInvalid("受托模式本次凭证不能为空", nil)
		}
		if config.LocalStop == nil {
			return nil, gatewayInvalid("受托模式停止回调不能为空", nil)
		}
	} else if config.Addr == "" {
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
	boxVersion := strings.TrimSpace(config.BoxVersion)
	if boxVersion == "" {
		release, err := boxrelease.Load()
		if err != nil {
			return nil, gatewayDependencyFailed("eucli-box 发布资料无效", err)
		}
		boxVersion = release.Version
	}
	if err := release.ValidateVersion(boxVersion); err != nil {
		return nil, gatewayInvalid("eucli-box 版本无效", err)
	}
	boxRelease, err := boxrelease.Load()
	if err != nil {
		return nil, gatewayDependencyFailed("eucli-box 发布资料无效", err)
	}
	boxRelease.Version = boxVersion
	s := &system{
		config:        config,
		boxRelease:    boxRelease,
		runtime:       runtime,
		roles:         roles,
		groups:        groups,
		workspaces:    workspaces,
		providers:     providers,
		tools:         tools,
		sessions:      sessions,
		stickers:      stickers,
		hooks:         hooks,
		assist:        assist,
		releaseChecks: releaseChecks,
		placeholders:  placeholders,
		systemPlugins: systemPlugins,
		access:        config.Access,
		mux:           http.NewServeMux(),
		upgrader:      websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		connections:   map[*websocket.Conn]struct{}{},
		localInfo: LocalRunInfo{
			InstallIdentity:  strings.TrimSpace(config.LocalInstallID),
			DataIdentity:     strings.TrimSpace(config.LocalDataID),
			RunIdentity:      strings.TrimSpace(config.LocalRunID),
			ProcessID:        config.LocalProcessID,
			ProcessStartedAt: config.LocalProcessStart.UTC(),
			Version:          boxVersion,
		},
	}
	s.registerRoutes()
	s.server = &http.Server{Addr: config.Addr, Handler: s.mux, ReadTimeout: config.ReadTimeout, WriteTimeout: config.WriteTimeout}
	return s, nil
}

func (s *system) Handler() http.Handler {
	return s.mux
}

// LongTermHandler 供长期端口连接转发使用。它使用长期 Key 鉴权包装，并排除受托专用路由。
func (s *system) LongTermHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/local-run" || strings.HasPrefix(r.URL.Path, "/api/local-run/") {
			writeError(w, gatewayNotAuthorized("long-term route is unavailable", nil))
			return
		}
		s.longTermAuthWrap(s.mux.ServeHTTP)(w, r)
	})
}
