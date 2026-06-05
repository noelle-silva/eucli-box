package modelprovider

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) LoadModelGroups(ctx context.Context) ([]types.ModelGroup, error) {
	groups, err := s.storage.LoadModelGroups(ctx)
	if err != nil {
		return nil, providerStorageFailed("failed to load model groups", err)
	}
	return normalizeModelGroups(groups), nil
}

func (s *system) SaveModelGroups(ctx context.Context, groups []types.ModelGroup) ([]types.ModelGroup, error) {
	normalized := normalizeModelGroups(groups)
	if err := s.validateModelGroups(ctx, normalized); err != nil {
		return nil, err
	}
	saved, err := s.storage.SaveModelGroups(ctx, normalized)
	if err != nil {
		return nil, providerStorageFailed("failed to save model groups", err)
	}
	return saved, nil
}

func (s *system) resolveModelGroup(ctx context.Context, coordinate types.ModelCoordinate) (types.Provider, types.ModelInfo, error) {
	groupID := strings.TrimSpace(coordinate.GroupID)
	modelID := strings.TrimSpace(coordinate.ModelID)
	if groupID == "" {
		return types.Provider{}, types.ModelInfo{}, providerInvalid("model group id is required", nil)
	}
	if modelID == "" {
		return types.Provider{}, types.ModelInfo{}, providerInvalid("model group model id is required", nil)
	}
	groups, err := s.LoadModelGroups(ctx)
	if err != nil {
		return types.Provider{}, types.ModelInfo{}, err
	}
	for _, group := range groups {
		if group.ID != groupID {
			continue
		}
		for _, model := range group.Models {
			if model.ID != modelID {
				continue
			}
			if len(model.Members) == 0 {
				return types.Provider{}, types.ModelInfo{}, providerModelNotFound("model group model has no members", nil)
			}
			weights := make([]int, 0, len(model.Members))
			for _, member := range model.Members {
				weights = append(weights, member.Weight)
			}
			index, err := s.pickRotatedIndex("group:"+group.ID+":"+model.ID, model.Strategy, weights)
			if err != nil {
				return types.Provider{}, types.ModelInfo{}, err
			}
			member := model.Members[index]
			return s.ResolveModel(ctx, types.ModelCoordinate{ProviderID: member.ProviderID, ModelID: member.ModelID})
		}
	}
	return types.Provider{}, types.ModelInfo{}, providerModelNotFound("model group coordinate does not exist", nil)
}

func (s *system) validateModelGroups(ctx context.Context, groups []types.ModelGroup) error {
	seenGroups := map[string]struct{}{}
	seenGroupNames := map[string]struct{}{}
	for _, group := range groups {
		if group.ID == "" {
			return providerInvalid("model group id is required", nil)
		}
		if group.Name == "" {
			return providerInvalid("model group name is required", nil)
		}
		if _, ok := seenGroups[group.ID]; ok {
			return providerInvalid("model group id must be unique", nil)
		}
		if _, ok := seenGroupNames[group.Name]; ok {
			return providerInvalid("model group name must be unique", nil)
		}
		seenGroups[group.ID] = struct{}{}
		seenGroupNames[group.Name] = struct{}{}

		seenModels := map[string]struct{}{}
		for _, model := range group.Models {
			if model.ID == "" {
				return providerInvalid("model group exposed model id is required", nil)
			}
			if model.Name == "" {
				return providerInvalid("model group exposed model name is required", nil)
			}
			if _, ok := seenModels[model.ID]; ok {
				return providerInvalid("model group exposed model id must be unique", nil)
			}
			seenModels[model.ID] = struct{}{}
			if len(model.Members) == 0 {
				return providerInvalid("model group exposed model member is required", nil)
			}
			for _, member := range model.Members {
				if _, _, err := s.ResolveModel(ctx, types.ModelCoordinate{ProviderID: member.ProviderID, ModelID: member.ModelID}); err != nil {
					return providerInvalid("model group member model is not available", err)
				}
			}
		}
	}
	return nil
}

func normalizeModelGroups(groups []types.ModelGroup) []types.ModelGroup {
	now := time.Now().UTC()
	out := make([]types.ModelGroup, 0, len(groups))
	for _, group := range groups {
		group.ID = strings.TrimSpace(group.ID)
		group.Name = strings.TrimSpace(group.Name)
		if group.CreatedAt.IsZero() {
			group.CreatedAt = now
		}
		if group.UpdatedAt.IsZero() {
			group.UpdatedAt = group.CreatedAt
		}
		models := make([]types.ModelGroupModel, 0, len(group.Models))
		for _, model := range group.Models {
			model.ID = strings.TrimSpace(model.ID)
			model.Name = strings.TrimSpace(model.Name)
			model.Strategy = normalizeRotationStrategy(model.Strategy)
			if model.CreatedAt.IsZero() {
				model.CreatedAt = now
			}
			if model.UpdatedAt.IsZero() {
				model.UpdatedAt = model.CreatedAt
			}
			members := make([]types.ModelGroupMember, 0, len(model.Members))
			for _, member := range model.Members {
				member.ProviderID = strings.TrimSpace(member.ProviderID)
				member.ModelID = strings.TrimSpace(member.ModelID)
				member.Weight = positiveWeight(member.Weight)
				members = append(members, member)
			}
			model.Members = members
			models = append(models, model)
		}
		group.Models = models
		out = append(out, group)
	}
	return out
}
