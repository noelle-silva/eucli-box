package modelprovider

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) ResolveModel(ctx context.Context, coordinate types.ModelCoordinate) (types.Provider, types.ModelInfo, error) {
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
	for _, model := range provider.Models {
		if model.ID == coordinate.ModelID {
			return provider, model, nil
		}
	}
	return types.Provider{}, types.ModelInfo{}, providerModelNotFound("model coordinate does not exist", nil)
}

func (s *system) resolveByProviderName(ctx context.Context, coordinate types.ModelCoordinate) (types.Provider, types.ModelInfo, error) {
	if strings.TrimSpace(coordinate.ProviderID) != "" {
		provider, err := s.LoadProvider(ctx, coordinate.ProviderID)
		if err == nil {
			for _, model := range provider.Models {
				if model.ID == coordinate.ModelID {
					return provider, model, nil
				}
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
			for _, model := range provider.Models {
				if model.ID == coordinate.ModelID {
					return provider, model, nil
				}
			}
		}
	}
	return types.Provider{}, types.ModelInfo{}, providerNotFound("provider not found by name", nil)
}
