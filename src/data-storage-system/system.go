package datastorage

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

type System interface {
	Initialize(ctx context.Context) error

	CreateSession(ctx context.Context, roleID string, title string) (types.Session, error)
	SaveSession(ctx context.Context, session types.Session) error
	LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error)
	ListSessions(ctx context.Context, roleID string) ([]types.SessionSummary, error)
	DeleteSession(ctx context.Context, roleID string, sessionID string) error
	UpdateSessionTitle(ctx context.Context, roleID string, sessionID string, title string) (types.Session, error)
	UpdateSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string, content string) (types.Session, error)
	DeleteSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string) (types.Session, error)
	DeleteSessionMessageSubtree(ctx context.Context, roleID string, sessionID string, messageID string) (types.Session, error)
	SaveSessionMessageAttachment(ctx context.Context, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error)
	LoadSessionAttachmentImage(ctx context.Context, relPath string) (string, error)
	LoadSessionFavorites(ctx context.Context) (types.SessionFavorites, error)
	SaveSessionFavorites(ctx context.Context, favorites types.SessionFavorites) (types.SessionFavorites, error)

	SaveRole(ctx context.Context, role types.Role) error
	LoadRole(ctx context.Context, roleID string) (types.Role, error)
	ListRoles(ctx context.Context) ([]types.RoleSummary, error)
	DeleteRole(ctx context.Context, roleID string) error
	SaveRoleAvatar(ctx context.Context, roleID string, dataURL string) error
	LoadRoleAvatar(ctx context.Context, roleID string) (string, error)
	DeleteRoleAvatar(ctx context.Context, roleID string) error

	SaveProvider(ctx context.Context, provider types.Provider) error
	LoadProvider(ctx context.Context, providerID string) (types.Provider, error)
	ListProviders(ctx context.Context) ([]types.ProviderSummary, error)
	DeleteProvider(ctx context.Context, providerID string) error

	SaveTool(ctx context.Context, tool types.ToolDefinition) error
	LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error)
	ListTools(ctx context.Context) ([]types.ToolSummary, error)
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

	SaveCallRecord(ctx context.Context, record types.CallRecord) error

	RebuildIndexes(ctx context.Context) error
}

type Config struct {
	RootDir string
}

type system struct {
	paths paths
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
	now := time.Now().UTC()
	version := storageVersion{Version: "1.0.0", CreatedAt: now, UpdatedAt: now}
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
