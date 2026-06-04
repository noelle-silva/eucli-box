package modelprovider

import (
	"context"
	"fmt"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) LoadModelRequestConfig(ctx context.Context) (types.ModelRequestConfig, error) {
	config, err := s.storage.LoadModelRequestConfig(ctx)
	if err != nil {
		return types.ModelRequestConfig{}, providerStorageFailed("failed to load model request config", err)
	}
	config = normalizeModelRequestConfig(config)
	if err := validateModelRequestConfig(config); err != nil {
		return types.ModelRequestConfig{}, err
	}
	return config, nil
}

func (s *system) SaveModelRequestConfig(ctx context.Context, config types.ModelRequestConfig) (types.ModelRequestConfig, error) {
	config = normalizeModelRequestConfig(config)
	if err := validateModelRequestConfig(config); err != nil {
		return types.ModelRequestConfig{}, err
	}
	saved, err := s.storage.SaveModelRequestConfig(ctx, config)
	if err != nil {
		return types.ModelRequestConfig{}, providerStorageFailed("failed to save model request config", err)
	}
	return normalizeModelRequestConfig(saved), nil
}

func (s *system) modelRequestConfig(ctx context.Context) (types.ModelRequestConfig, error) {
	return s.LoadModelRequestConfig(ctx)
}

func normalizeModelRequestConfig(config types.ModelRequestConfig) types.ModelRequestConfig {
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

func validateModelRequestConfig(config types.ModelRequestConfig) error {
	if err := validateTimeoutMs("listModelsTimeoutMs", config.ListModelsTimeoutMs, types.ModelRequestListModelsTimeoutMinMs, types.ModelRequestListModelsTimeoutMaxMs); err != nil {
		return err
	}
	if err := validateTimeoutMs("completionTimeoutMs", config.CompletionTimeoutMs, types.ModelRequestCompletionTimeoutMinMs, types.ModelRequestCompletionTimeoutMaxMs); err != nil {
		return err
	}
	if err := validateTimeoutMs("streamIdleTimeoutMs", config.StreamIdleTimeoutMs, types.ModelRequestStreamIdleTimeoutMinMs, types.ModelRequestStreamIdleTimeoutMaxMs); err != nil {
		return err
	}
	return nil
}

func validateTimeoutMs(name string, value int, min int, max int) error {
	if value < min || value > max {
		return providerInvalid(fmt.Sprintf("%s must be between %d and %d milliseconds", name, min, max), nil)
	}
	return nil
}

func timeoutFromMs(ms int) time.Duration {
	return time.Duration(ms) * time.Millisecond
}
