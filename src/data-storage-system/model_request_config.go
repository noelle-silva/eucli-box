package datastorage

import (
	"context"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) LoadModelRequestConfig(ctx context.Context) (types.ModelRequestConfig, error) {
	return loadMetaConfig(ctx, s.paths.modelRequestConfigFile(), defaultModelRequestConfig(), normalizeModelRequestConfigForStorage)
}

func (s *system) SaveModelRequestConfig(ctx context.Context, config types.ModelRequestConfig) (types.ModelRequestConfig, error) {
	config = normalizeModelRequestConfigForStorage(config)
	config.UpdatedAt = time.Now().UTC()
	if err := writeJSON(ctx, s.paths.modelRequestConfigFile(), config); err != nil {
		return types.ModelRequestConfig{}, err
	}
	return config, nil
}

func defaultModelRequestConfig() types.ModelRequestConfig {
	return normalizeModelRequestConfigForStorage(types.ModelRequestConfig{})
}

func normalizeModelRequestConfigForStorage(config types.ModelRequestConfig) types.ModelRequestConfig {
	if config.ListModelsTimeoutMs == 0 {
		config.ListModelsTimeoutMs = types.ModelRequestListModelsTimeoutDefaultMs
	}
	if config.CompletionTimeoutMs == 0 {
		config.CompletionTimeoutMs = types.ModelRequestCompletionTimeoutDefaultMs
	}
	if config.StreamIdleTimeoutMs == 0 {
		config.StreamIdleTimeoutMs = types.ModelRequestStreamIdleTimeoutDefaultMs
	}
	if config.UpdatedAt.IsZero() {
		config.UpdatedAt = time.Now().UTC()
	}
	return config
}
