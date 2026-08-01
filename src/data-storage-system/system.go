package datastorage

import (
	"context"
	"strings"
	"sync"
	"time"

	"eucli-box/internal/boxrelease"
	"eucli-box/pkg/types"
)

type System interface {
	Initialize(ctx context.Context) error

	CreateSession(ctx context.Context, roleID string, title string) (types.Session, error)
	CreateGroupSession(ctx context.Context, groupID string, title string) (types.Session, error)
	CreateWorkspaceSession(ctx context.Context, workspaceID string, roleID string, title string) (types.Session, error)
	SaveSession(ctx context.Context, session types.Session) error
	SaveSessionMessages(ctx context.Context, save types.SessionMessageSave) error
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
	SaveSessionMessageAttachment(ctx context.Context, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error)
	SaveGroupSessionMessageAttachment(ctx context.Context, groupID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error)
	SaveWorkspaceSessionMessageAttachment(ctx context.Context, workspaceID string, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error)
	LoadSessionAttachmentImage(ctx context.Context, relPath string) (string, error)
	LoadSessionFavorites(ctx context.Context) (types.SessionFavorites, error)
	SaveSessionFavorites(ctx context.Context, favorites types.SessionFavorites) (types.SessionFavorites, error)
	LoadHookPromptLibrary(ctx context.Context) (types.HookPromptLibrary, error)
	SaveHookPromptLibrary(ctx context.Context, library types.HookPromptLibrary) (types.HookPromptLibrary, error)

	SaveRole(ctx context.Context, role types.Role) error
	LoadRole(ctx context.Context, roleID string) (types.Role, error)
	ListRoles(ctx context.Context) ([]types.RoleSummary, error)
	DeleteRole(ctx context.Context, roleID string) error
	SaveRoleAvatar(ctx context.Context, roleID string, dataURL string) error
	LoadRoleAvatar(ctx context.Context, roleID string) (string, error)
	DeleteRoleAvatar(ctx context.Context, roleID string) error

	SaveChatGroup(ctx context.Context, group types.ChatGroup) error
	LoadChatGroup(ctx context.Context, groupID string) (types.ChatGroup, error)
	ListChatGroups(ctx context.Context) ([]types.ChatGroupSummary, error)
	DeleteChatGroup(ctx context.Context, groupID string) error
	SaveChatGroupAvatar(ctx context.Context, groupID string, dataURL string) error
	LoadChatGroupAvatar(ctx context.Context, groupID string) (string, error)
	DeleteChatGroupAvatar(ctx context.Context, groupID string) error

	SaveWorkspace(ctx context.Context, workspace types.Workspace) error
	LoadWorkspace(ctx context.Context, workspaceID string) (types.Workspace, error)
	ListWorkspaces(ctx context.Context) ([]types.WorkspaceSummary, error)
	PreviewWorkspacePrompt(ctx context.Context, workspace types.Workspace) (string, error)
	DeleteWorkspace(ctx context.Context, workspaceID string) error

	SaveProvider(ctx context.Context, provider types.Provider) error
	LoadProvider(ctx context.Context, providerID string) (types.Provider, error)
	ListProviders(ctx context.Context) ([]types.ProviderSummary, error)
	DeleteProvider(ctx context.Context, providerID string) error

	SaveTool(ctx context.Context, tool types.ToolDefinition) error
	LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error)
	ListTools(ctx context.Context) ([]types.ToolSummary, error)
	SaveToolUserSettings(ctx context.Context, toolID string, settings types.ToolUserSettings) (types.ToolDefinition, error)
	DeleteTool(ctx context.Context, toolID string) error

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
	LoadModelRequestConfig(ctx context.Context) (types.ModelRequestConfig, error)
	SaveModelRequestConfig(ctx context.Context, config types.ModelRequestConfig) (types.ModelRequestConfig, error)
	LoadModelGroups(ctx context.Context) ([]types.ModelGroup, error)
	SaveModelGroups(ctx context.Context, groups []types.ModelGroup) ([]types.ModelGroup, error)

	RebuildIndexes(ctx context.Context) error
}

type Config struct {
	RootDir string
}

type system struct {
	paths     paths
	sessionMu sync.Mutex
}

func NewSystem(config Config) (System, error) {
	root := strings.TrimSpace(config.RootDir)
	if root == "" {
		root = "data"
	}
	paths, err := newPaths(root)
	if err != nil {
		return nil, err
	}
	return &system{paths: paths}, nil
}

func (s *system) Initialize(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return storageInitFailed("initialization cancelled", err)
	}
	if err := ensureDirs(s.paths.baseDirs()...); err != nil {
		return storageInitFailed("failed to create data directories", err)
	}
	releaseInfo, err := boxrelease.Load()
	if err != nil {
		return storageInitFailed("failed to read target data version", err)
	}
	now := time.Now().UTC()
	version := storageVersion{Version: releaseInfo.DataVersion, CreatedAt: now, UpdatedAt: now}
	if dataFileExists(s.paths.metaVersionFile()) {
		current, err := readJSON[storageVersion](ctx, s.paths.metaVersionFile())
		if err != nil {
			return storageInitFailed("failed to read storage version", err)
		}
		current.UpdatedAt = now
		version = current
	}
	if err := writeJSON(ctx, s.paths.metaVersionFile(), version); err != nil {
		return storageInitFailed("failed to write storage version", err)
	}
	if err := s.ensureSessionFavoritesFile(ctx); err != nil {
		return err
	}
	return s.RebuildIndexes(ctx)
}

type storageVersion struct {
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
