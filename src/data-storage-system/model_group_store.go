package datastorage

import (
	"context"

	"eucli-box/pkg/types"
)

func (s *system) LoadModelGroups(ctx context.Context) ([]types.ModelGroup, error) {
	target := s.paths.modelGroupsFile()
	if !dataFileExists(target) {
		return []types.ModelGroup{}, nil
	}
	groups, err := readJSON[[]types.ModelGroup](ctx, target)
	if err != nil {
		return nil, err
	}
	if groups == nil {
		return []types.ModelGroup{}, nil
	}
	return groups, nil
}

func (s *system) SaveModelGroups(ctx context.Context, groups []types.ModelGroup) ([]types.ModelGroup, error) {
	if groups == nil {
		groups = []types.ModelGroup{}
	}
	if err := writeJSON(ctx, s.paths.modelGroupsFile(), groups); err != nil {
		return nil, err
	}
	return groups, nil
}
