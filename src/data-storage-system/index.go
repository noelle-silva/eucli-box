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
	if err := s.rebuildSessionIndexes(ctx); err != nil {
		return err
	}
	if err := s.rebuildRequestRecordIndex(ctx); err != nil {
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
	root := s.paths.toolProgramsRoot()
	if s.paths.managedToolPrograms() {
		root = s.paths.toolDataRoot()
	}
	return writeIndex(ctx, filepath.Join(root, "index.json"), rootIndex[types.ToolSummary]{Items: tools})
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
