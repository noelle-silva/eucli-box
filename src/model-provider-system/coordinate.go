package modelprovider

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) ResolveModel(ctx context.Context, coordinate types.ModelCoordinate) (types.Provider, types.ModelInfo, error) {
	if strings.TrimSpace(coordinate.Kind) == "model_group" || strings.TrimSpace(coordinate.GroupID) != "" {
		return s.resolveModelGroup(ctx, coordinate)
	}
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
