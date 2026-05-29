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

func (s *system) RebuildIndexes(ctx context.Context) error {
	if err := s.rebuildRoleIndex(ctx); err != nil {
		return err
	}
	if err := s.rebuildProviderIndex(ctx); err != nil {
		return err
	}
	if err := s.rebuildToolIndex(ctx); err != nil {
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
	entries, err := os.ReadDir(s.paths.sessionsRoot())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return storageIndexFailed("failed to scan sessions root", err)
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
	return writeIndex(ctx, filepath.Join(s.paths.sessionsRoot(), "index.json"), sessionRoleIndex{Folders: folders})
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
