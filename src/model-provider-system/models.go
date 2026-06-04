package modelprovider

import (
	"context"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) RefreshModels(ctx context.Context, providerID string) ([]types.ModelInfo, error) {
	provider, err := s.LoadProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	adapter, err := adapterFor(provider.Protocol)
	if err != nil {
		return nil, err
	}
	requestConfig, err := s.modelRequestConfig(ctx)
	if err != nil {
		return nil, err
	}
	req, err := adapter.BuildListModelsRequest(provider, int64(timeoutFromMs(requestConfig.ListModelsTimeoutMs)))
	if err != nil {
		return nil, err
	}
	response, err := s.network.Do(ctx, req)
	if err != nil {
		return nil, providerNetworkFailed("failed to request provider model list", err)
	}
	models, err := adapter.ParseListModelsResponse(response)
	if err != nil {
		return nil, err
	}
	provider.Models = models
	provider.UpdatedAt = time.Now().UTC()
	if err := s.storage.SaveProvider(ctx, provider); err != nil {
		return nil, providerStorageFailed("failed to save refreshed model list", err)
	}
	return models, nil
}
