package modelprovider

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) ResolveModel(ctx context.Context, coordinate types.ModelCoordinate) (types.Provider, types.ModelInfo, error) {
	if strings.TrimSpace(coordinate.ProviderID) == "" {
		return types.Provider{}, types.ModelInfo{}, providerInvalid("provider id is required", nil)
	}
	if strings.TrimSpace(coordinate.ModelID) == "" {
		return types.Provider{}, types.ModelInfo{}, providerInvalid("model id is required", nil)
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
