package datastorage

import (
	"context"
	"path/filepath"

	"eucli-box/pkg/types"
)

func (s *system) SaveRole(ctx context.Context, role types.Role) error {
	if _, err := cleanID(role.ID); err != nil {
		return err
	}
	target, err := s.paths.roleDataFile(role.ID)
	if err != nil {
		return err
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	isNew := !dataFileExists(target)
	if err := writeJSON(ctx, target, role); err != nil {
		return err
	}
	roleDir, err := s.paths.roleDir(role.ID)
	if err != nil {
		return err
	}
	if err := ensureDirs(filepath.Join(roleDir, "attachments")); err != nil {
		return storageWriteFailed("failed to create role attachments directory", err)
	}
	if err := s.syncOrderAfterSave(ctx, s.paths.rolesRoot(), s.paths.roleOrderFile(), role.ID, isNew); err != nil {
		return err
	}
	return s.rebuildRoleIndex(ctx)
}

func (s *system) LoadRole(ctx context.Context, roleID string) (types.Role, error) {
	target, err := s.paths.roleDataFile(roleID)
	if err != nil {
		return types.Role{}, err
	}
	return readJSON[types.Role](ctx, target)
}

func (s *system) ListRoles(ctx context.Context) ([]types.RoleSummary, error) {
	roles, err := readObjects[types.Role](ctx, s.paths.rolesRoot())
	if err != nil {
		return nil, err
	}
	byID := make(map[string]types.RoleSummary, len(roles))
	for _, role := range roles {
		byID[role.ID] = types.RoleSummary{ID: role.ID, Name: role.Name, UpdatedAt: role.UpdatedAt}
	}
	ids, err := orderedObjectIDs(ctx, s.paths.rolesRoot(), s.paths.roleOrderFile())
	if err != nil {
		return nil, err
	}
	summaries := make([]types.RoleSummary, 0, len(ids))
	for _, id := range ids {
		if summary, ok := byID[id]; ok {
			summaries = append(summaries, summary)
		}
	}
	return summaries, nil
}

func (s *system) ReorderRoles(ctx context.Context, roleIDs []string) error {
	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	if err := s.reorderObjects(ctx, s.paths.rolesRoot(), s.paths.roleOrderFile(), roleIDs); err != nil {
		return err
	}
	return s.rebuildRoleIndex(ctx)
}

func (s *system) DeleteRole(ctx context.Context, roleID string) error {
	dir, err := s.paths.roleDir(roleID)
	if err != nil {
		return err
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	if err := moveToRecycle(ctx, s.paths, types.StorageItemRole, roleID, dir); err != nil {
		return err
	}
	if err := s.syncOrderAfterDelete(ctx, s.paths.rolesRoot(), s.paths.roleOrderFile()); err != nil {
		return err
	}
	return s.rebuildRoleIndex(ctx)
}
