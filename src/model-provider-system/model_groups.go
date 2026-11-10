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

// loadModelGroupModel 定位模型组里的暴露模型；只读取，不参与任何轮询选择。
func (s *system) loadModelGroupModel(ctx context.Context, coordinate types.ModelCoordinate) (types.ModelGroup, types.ModelGroupModel, error) {
	groupID := strings.TrimSpace(coordinate.GroupID)
	modelID := strings.TrimSpace(coordinate.ModelID)
	if groupID == "" {
		return types.ModelGroup{}, types.ModelGroupModel{}, providerInvalid("model group id is required", nil)
	}
	if modelID == "" {
		return types.ModelGroup{}, types.ModelGroupModel{}, providerInvalid("model group model id is required", nil)
	}
	groups, err := s.LoadModelGroups(ctx)
	if err != nil {
		return types.ModelGroup{}, types.ModelGroupModel{}, err
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
				return types.ModelGroup{}, types.ModelGroupModel{}, providerModelNotFound("model group model has no members", nil)
			}
			return group, model, nil
		}
	}
	return types.ModelGroup{}, types.ModelGroupModel{}, providerModelNotFound("model group coordinate does not exist", nil)
}

// selectGroupMember 按轮询类策略选出本轮成员并解析；这是模型组唯一的轮询选择入口。
func (s *system) selectGroupMember(ctx context.Context, group types.ModelGroup, model types.ModelGroupModel) (planCandidate, error) {
	weights := make([]int, 0, len(model.Members))
	for _, member := range model.Members {
		weights = append(weights, member.Weight)
	}
	index, err := s.pickRotatedIndex("group:"+group.ID+":"+model.ID, model.Strategy, weights)
	if err != nil {
		return planCandidate{}, err
	}
	member := model.Members[index]
	provider, resolved, err := s.resolveProviderCoordinate(ctx, types.ModelCoordinate{ProviderID: member.ProviderID, ModelID: member.ModelID})
	if err != nil {
		return planCandidate{}, err
	}
	return planCandidate{provider: provider, model: applyGroupReasoningProfile(model, resolved)}, nil
}

// buildFallbackChain 把 fallback 组成员按配置顺序展开成候选链：
// 第一个永远是首选，往下都是它的兜底。
// 解析不出来的成员直接跳过——它本身已经坏了，正是兜底链要绕开的东西。
func (s *system) buildFallbackChain(ctx context.Context, model types.ModelGroupModel) (requestPlan, error) {
	candidates := make([]planCandidate, 0, len(model.Members))
	var firstErr error
	for _, member := range model.Members {
		provider, resolved, err := s.resolveProviderCoordinate(ctx, types.ModelCoordinate{ProviderID: member.ProviderID, ModelID: member.ModelID})
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		candidates = append(candidates, planCandidate{provider: provider, model: applyGroupReasoningProfile(model, resolved)})
	}
	if len(candidates) == 0 {
		return requestPlan{}, providerModelNotFound("no fallback member is available", firstErr)
	}
	return requestPlan{candidates: candidates}, nil
}

// applyGroupReasoningProfile 用模型组暴露的能力声明覆盖成员自身的推理配置：
// 对外的模型是一个整体，推理能力以组上的声明为准。
func applyGroupReasoningProfile(model types.ModelGroupModel, resolved types.ModelInfo) types.ModelInfo {
	if model.SupportsReasoning {
		resolved.SupportsReasoning = true
		resolved.DefaultReasoningEffort = types.NormalizeReasoningEffort(model.DefaultReasoningEffort, types.DefaultReasoningEffort)
	} else {
		resolved.SupportsReasoning = false
		resolved.DefaultReasoningEffort = ""
	}
	return resolved
}

// validateModelGroupCoordinate 校验模型组坐标可用：组与暴露模型存在，且每个成员模型都可解析。
// 该路径不推进轮询游标，可在配置校验等场合安全调用。
func (s *system) validateModelGroupCoordinate(ctx context.Context, coordinate types.ModelCoordinate) error {
	_, model, err := s.loadModelGroupModel(ctx, coordinate)
	if err != nil {
		return err
	}
	return s.validateGroupMembers(ctx, model.Members)
}

// validateGroupMembers 逐个确认成员模型可解析；成员坐标是供应商坐标，因此不牵动轮询。
func (s *system) validateGroupMembers(ctx context.Context, members []types.ModelGroupMember) error {
	for _, member := range members {
		if _, _, err := s.resolveProviderCoordinate(ctx, types.ModelCoordinate{ProviderID: member.ProviderID, ModelID: member.ModelID}); err != nil {
			return providerInvalid("model group member model is not available", err)
		}
	}
	return nil
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
			if model.SupportsReasoning && !types.IsReasoningEffort(model.DefaultReasoningEffort) {
				return providerInvalid("model group exposed model defaultReasoningEffort is invalid", nil)
			}
			seenModels[model.ID] = struct{}{}
			if len(model.Members) == 0 {
				return providerInvalid("model group exposed model member is required", nil)
			}
			if err := s.validateGroupMembers(ctx, model.Members); err != nil {
				return err
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
			if model.SupportsReasoning {
				model.DefaultReasoningEffort = types.NormalizeReasoningEffort(model.DefaultReasoningEffort, types.DefaultReasoningEffort)
			} else {
				model.DefaultReasoningEffort = ""
			}
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
