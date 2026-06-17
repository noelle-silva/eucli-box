package datastorage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"eucli-box/pkg/types"
)

type rootIndex[T any] struct {
	Items []T `json:"items"`
}

type sessionRoleIndex struct {
	Folders []roleFolder `json:"folders"`
}

type roleFolder struct {
	ID string `json:"id"`
}

type sessionIndex struct {
	Sessions []types.SessionSummary `json:"sessions"`
	Sort     string                 `json:"sort"`
}

type workspaceRoleSessionIndex struct {
	Roles []roleFolder `json:"roles"`
}

func (s *system) RebuildIndexes(ctx context.Context) error {
	if err := s.rebuildRoleIndex(ctx); err != nil {
		return err
	}
	if err := s.rebuildChatGroupIndex(ctx); err != nil {
		return err
	}
	if err := s.rebuildWorkspaceIndex(ctx); err != nil {
		return err
	}
	if err := s.rebuildProviderIndex(ctx); err != nil {
		return err
	}
	if err := s.rebuildToolIndex(ctx); err != nil {
		return err
	}
	if err := s.rebuildStickerIndexes(ctx); err != nil {
		return err
	}
	if err := s.rebuildAllSessionIndexes(ctx); err != nil {
		return err
	}
	return rebuildRecycleIndex(ctx, s.paths)
}

func (s *system) rebuildRoleIndex(ctx context.Context) error {
	roles, err := s.ListRoles(ctx)
	if err != nil {
		return err
	}
	return writeIndex(ctx, filepath.Join(s.paths.rolesRoot(), "index.json"), rootIndex[types.RoleSummary]{Items: roles})
}

func (s *system) rebuildProviderIndex(ctx context.Context) error {
	providers, err := s.ListProviders(ctx)
	if err != nil {
		return err
	}
	return writeIndex(ctx, filepath.Join(s.paths.providersRoot(), "index.json"), rootIndex[types.ProviderSummary]{Items: providers})
}

func (s *system) rebuildToolIndex(ctx context.Context) error {
	tools, err := s.ListTools(ctx)
	if err != nil {
		return err
	}
	return writeIndex(ctx, filepath.Join(s.paths.toolsRoot(), "index.json"), rootIndex[types.ToolSummary]{Items: tools})
}

func (s *system) rebuildAllSessionIndexes(ctx context.Context) error {
	entries, err := os.ReadDir(s.paths.sessionRolesRoot())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return storageIndexFailed("failed to scan role sessions root", err)
	}
	folders := make([]roleFolder, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		roleID := entry.Name()
		folders = append(folders, roleFolder{ID: roleID})
		if err := s.rebuildSessionIndexes(ctx, roleID); err != nil {
			return err
		}
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].ID < folders[j].ID })
	if err := writeIndex(ctx, filepath.Join(s.paths.sessionRolesRoot(), "index.json"), sessionRoleIndex{Folders: folders}); err != nil {
		return err
	}
	if err := s.rebuildAllGroupSessionIndexes(ctx); err != nil {
		return err
	}
	return s.rebuildAllWorkspaceSessionIndexes(ctx)
}

func (s *system) rebuildSessionIndexes(ctx context.Context, roleID string) error {
	sessions, err := s.ListSessions(ctx, roleID)
	if err != nil {
		return err
	}
	roleDir, err := s.paths.sessionRoleDir(roleID)
	if err != nil {
		return err
	}
	return writeIndex(ctx, filepath.Join(roleDir, "index.json"), sessionIndex{Sessions: sessions, Sort: "lastActive"})
}

func (s *system) rebuildSessionIndexesForScope(ctx context.Context, scope sessionScope) error {
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return err
	}
	if scope.Kind == sessionScopeGroup {
		return s.rebuildGroupSessionIndexes(ctx, scope.ID)
	}
	if scope.Kind == sessionScopeWorkspace {
		return s.rebuildWorkspaceSessionIndexes(ctx, scope.ID)
	}
	return s.rebuildSessionIndexes(ctx, scope.ID)
}

func (s *system) rebuildAllGroupSessionIndexes(ctx context.Context) error {
	entries, err := os.ReadDir(s.paths.sessionGroupsRoot())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return storageIndexFailed("failed to scan group sessions root", err)
	}
	folders := make([]roleFolder, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		groupID := entry.Name()
		folders = append(folders, roleFolder{ID: groupID})
		if err := s.rebuildGroupSessionIndexes(ctx, groupID); err != nil {
			return err
		}
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].ID < folders[j].ID })
	return writeIndex(ctx, filepath.Join(s.paths.sessionGroupsRoot(), "index.json"), sessionRoleIndex{Folders: folders})
}

func (s *system) rebuildGroupSessionIndexes(ctx context.Context, groupID string) error {
	sessions, err := s.ListGroupSessions(ctx, groupID)
	if err != nil {
		return err
	}
	groupDir, err := s.paths.sessionGroupDir(groupID)
	if err != nil {
		return err
	}
	return writeIndex(ctx, filepath.Join(groupDir, "index.json"), sessionIndex{Sessions: sessions, Sort: "lastActive"})
}

func (s *system) rebuildAllWorkspaceSessionIndexes(ctx context.Context) error {
	entries, err := os.ReadDir(s.paths.sessionWorkspacesRoot())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return storageIndexFailed("failed to scan workspace sessions root", err)
	}
	folders := make([]roleFolder, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		workspaceID := entry.Name()
		folders = append(folders, roleFolder{ID: workspaceID})
		if err := s.rebuildWorkspaceSessionIndexes(ctx, workspaceID); err != nil {
			return err
		}
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].ID < folders[j].ID })
	return writeIndex(ctx, filepath.Join(s.paths.sessionWorkspacesRoot(), "index.json"), sessionRoleIndex{Folders: folders})
}

func (s *system) rebuildWorkspaceSessionIndexes(ctx context.Context, workspaceID string) error {
	workspaceDir, err := s.paths.safeJoin(s.paths.sessionWorkspacesRoot(), workspaceID)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return storageIndexFailed("failed to scan workspace role sessions root", err)
	}
	roleFolders := make([]roleFolder, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		roleID := entry.Name()
		roleFolders = append(roleFolders, roleFolder{ID: roleID})
		if err := s.rebuildWorkspaceRoleSessionIndex(ctx, workspaceID, roleID); err != nil {
			return err
		}
	}
	sort.Slice(roleFolders, func(i, j int) bool { return roleFolders[i].ID < roleFolders[j].ID })
	return writeIndex(ctx, filepath.Join(workspaceDir, "index.json"), workspaceRoleSessionIndex{Roles: roleFolders})
}

func (s *system) rebuildWorkspaceRoleSessionIndex(ctx context.Context, workspaceID string, roleID string) error {
	sessions, err := s.ListWorkspaceSessions(ctx, workspaceID, roleID)
	if err != nil {
		return err
	}
	roleDir, err := s.paths.workspaceRoleSessionsDir(workspaceID, roleID)
	if err != nil {
		return err
	}
	return writeIndex(ctx, filepath.Join(roleDir, "index.json"), sessionIndex{Sessions: sessions, Sort: "lastActive"})
}

func rebuildRecycleIndex(ctx context.Context, paths paths) error {
	records, err := readRecycleRecords(ctx, paths.recycleRoot())
	if err != nil {
		return err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].DeletedAt.After(records[j].DeletedAt) })
	return writeIndex(ctx, filepath.Join(paths.recycleRoot(), "index.json"), rootIndex[types.RecycleRecord]{Items: records})
}

func writeIndex(ctx context.Context, target string, value any) error {
	if err := writeJSON(ctx, target, value); err != nil {
		return storageIndexFailed("failed to write index", err)
	}
	return nil
}

func readObjects[T any](ctx context.Context, root string) ([]T, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, storageReadFailed("failed to scan object directory", err)
	}
	objects := make([]T, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, storageReadFailed("scan cancelled", err)
		}
		if !entry.IsDir() {
			continue
		}
		dataFile := filepath.Join(root, entry.Name(), "data.json")
		if !dataFileExists(dataFile) {
			continue
		}
		object, err := readJSON[T](ctx, dataFile)
		if err != nil {
			return nil, err
		}
		objects = append(objects, object)
	}
	return objects, nil
}

func readRecycleRecords(ctx context.Context, root string) ([]types.RecycleRecord, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, storageReadFailed("failed to scan recycle directory", err)
	}
	records := make([]types.RecycleRecord, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		target := filepath.Join(root, entry.Name(), "deleted-at.json")
		if !dataFileExists(target) {
			continue
		}
		record, err := readJSON[types.RecycleRecord](ctx, target)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}
