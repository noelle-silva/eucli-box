package modelprovider

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

// ResolveModel 解析一次模型坐标，并在坐标指向模型组时推进该组的轮询游标：
// 它是"这一轮轮到谁"的唯一出口。
// 只想知道坐标能不能用的调用方（配置校验等）必须走 ValidateCoordinate，
// 否则校验行为会吃掉一次轮询顺序。
func (s *system) ResolveModel(ctx context.Context, coordinate types.ModelCoordinate) (types.Provider, types.ModelInfo, error) {
	if isModelGroupCoordinate(coordinate) {
		return s.resolveModelGroup(ctx, coordinate)
	}
	return s.resolveProviderCoordinate(ctx, coordinate)
}

// ValidateCoordinate 校验坐标当前是否可用：不参与轮询选择，不推进任何轮询游标。
func (s *system) ValidateCoordinate(ctx context.Context, coordinate types.ModelCoordinate) error {
	if isModelGroupCoordinate(coordinate) {
		return s.validateModelGroupCoordinate(ctx, coordinate)
	}
	_, _, err := s.resolveProviderCoordinate(ctx, coordinate)
	return err
}

// isModelGroupCoordinate 判断坐标是否指向模型组。
func isModelGroupCoordinate(coordinate types.ModelCoordinate) bool {
	return strings.TrimSpace(coordinate.Kind) == types.ModelCoordinateKindGroup ||
		strings.TrimSpace(coordinate.GroupID) != ""
}

// resolveProviderCoordinate 解析具体供应商下的注册模型；
// 只处理供应商坐标，不涉及模型组，也没有任何轮询副作用。
func (s *system) resolveProviderCoordinate(ctx context.Context, coordinate types.ModelCoordinate) (types.Provider, types.ModelInfo, error) {
	if strings.TrimSpace(coordinate.ModelID) == "" {
		return types.Provider{}, types.ModelInfo{}, providerInvalid("model id is required", nil)
	}
	if strings.TrimSpace(coordinate.ProviderName) != "" {
		return s.resolveByProviderName(ctx, coordinate)
	}
	if strings.TrimSpace(coordinate.ProviderID) == "" {
		return types.Provider{}, types.ModelInfo{}, providerInvalid("provider id is required", nil)
	}
	provider, err := s.LoadProvider(ctx, coordinate.ProviderID)
	if err != nil {
		return types.Provider{}, types.ModelInfo{}, err
	}
	source, err := resolveRegisteredModel(provider, coordinate.ModelID)
	if err != nil {
		return types.Provider{}, types.ModelInfo{}, err
	}
	return provider, source, nil
}

func (s *system) resolveByProviderName(ctx context.Context, coordinate types.ModelCoordinate) (types.Provider, types.ModelInfo, error) {
	if strings.TrimSpace(coordinate.ProviderID) != "" {
		provider, err := s.LoadProvider(ctx, coordinate.ProviderID)
		if err == nil {
			model, modelErr := resolveRegisteredModel(provider, coordinate.ModelID)
			if modelErr == nil {
				return provider, model, nil
			}
		}
	}
	providers, err := s.storage.ListProviders(ctx)
	if err != nil {
		return types.Provider{}, types.ModelInfo{}, providerStorageFailed("failed to list providers for name lookup", err)
	}
	for _, summary := range providers {
		if summary.Name == coordinate.ProviderName {
			provider, err := s.LoadProvider(ctx, summary.ID)
			if err != nil {
				continue
			}
			model, modelErr := resolveRegisteredModel(provider, coordinate.ModelID)
			if modelErr == nil {
				return provider, model, nil
			}
		}
	}
	return types.Provider{}, types.ModelInfo{}, providerNotFound("provider not found by name", nil)
}

func resolveRegisteredModel(provider types.Provider, registeredID string) (types.ModelInfo, error) {
	id := strings.TrimSpace(registeredID)
	if id == "" {
		return types.ModelInfo{}, providerInvalid("registered model id is required", nil)
	}
	for _, registered := range provider.RegisteredModels {
		if strings.TrimSpace(registered.ID) != id {
			continue
		}
		sourceID := strings.TrimSpace(registered.SourceModelID)
		if sourceID == "" {
			return types.ModelInfo{}, providerModelNotFound("registered model has empty sourceModelId", nil)
		}
		name := strings.TrimSpace(registered.Name)
		if name == "" {
			name = sourceID
		}
		return types.ModelInfo{ID: sourceID, Name: name, SupportsReasoning: registered.SupportsReasoning, DefaultReasoningEffort: registered.DefaultReasoningEffort}, nil
	}
	return types.ModelInfo{}, providerModelNotFound("registered model does not exist", nil)
}
