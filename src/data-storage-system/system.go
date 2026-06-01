package datastorage

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

type System interface {
	Initialize(ctx context.Context) error

	SaveSession(ctx context.Context, session types.Session) error
	LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error)
	ListSessions(ctx context.Context, roleID string) ([]types.SessionSummary, error)
	DeleteSession(ctx context.Context, roleID string, sessionID string) error

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
	return s.RebuildIndexes(ctx)
}

type storageVersion struct {
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
