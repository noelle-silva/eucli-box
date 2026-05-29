package datastorage

import (
	"context"
	"sort"

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
	if err := writeJSON(ctx, target, role); err != nil {
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
	summaries := make([]types.RoleSummary, 0, len(roles))
	for _, role := range roles {
		summaries = append(summaries, types.RoleSummary{ID: role.ID, Name: role.Name, Avatar: role.Avatar, UpdatedAt: role.UpdatedAt})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })
	return summaries, nil
}

func (s *system) DeleteRole(ctx context.Context, roleID string) error {
	dir, err := s.paths.roleDir(roleID)
	if err != nil {
		return err
	}
	if err := moveToRecycle(ctx, s.paths, types.StorageItemRole, roleID, dir); err != nil {
		return err
	}
	return s.rebuildRoleIndex(ctx)
}
