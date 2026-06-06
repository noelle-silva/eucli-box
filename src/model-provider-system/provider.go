package modelprovider

import (
	"context"
	"net/url"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) SaveProvider(ctx context.Context, provider types.Provider) error {
	provider = normalizeProvider(provider)
	if err := validateProvider(provider); err != nil {
		return err
	}
	providers, err := s.storage.ListProviders(ctx)
	if err != nil {
		return providerStorageFailed("failed to list providers for name uniqueness check", err)
	}
	for _, p := range providers {
		if p.Name == provider.Name && p.ID != provider.ID {
			return providerInvalid("provider name already exists", nil)
		}
	}
	now := time.Now().UTC()
	if provider.CreatedAt.IsZero() {
		provider.CreatedAt = now
	}
	provider.UpdatedAt = now
	if err := s.storage.SaveProvider(ctx, provider); err != nil {
		return providerStorageFailed("failed to save provider", err)
	}
	return nil
}

func (s *system) LoadProvider(ctx context.Context, providerID string) (types.Provider, error) {
	if strings.TrimSpace(providerID) == "" {
		return types.Provider{}, providerInvalid("provider id is required", nil)
	}
	provider, err := s.storage.LoadProvider(ctx, providerID)
	if err != nil {
		return types.Provider{}, providerNotFound("failed to load provider", err)
	}
	return normalizeProvider(provider), nil
}

func (s *system) ListProviders(ctx context.Context) ([]types.ProviderSummary, error) {
	providers, err := s.storage.ListProviders(ctx)
	if err != nil {
		return nil, providerStorageFailed("failed to list providers", err)
	}
	return providers, nil
}

func (s *system) DeleteProvider(ctx context.Context, providerID string) error {
	if strings.TrimSpace(providerID) == "" {
		return providerInvalid("provider id is required", nil)
	}
	if err := s.storage.DeleteProvider(ctx, providerID); err != nil {
		return providerStorageFailed("failed to delete provider", err)
	}
	return nil
}

func validateProvider(provider types.Provider) error {
	if strings.TrimSpace(provider.ID) == "" {
		return providerInvalid("provider id is required", nil)
	}
	if strings.TrimSpace(provider.Name) == "" {
		return providerInvalid("provider name is required", nil)
	}
	if strings.TrimSpace(provider.BaseURL) == "" {
		return providerInvalid("provider base url is required", nil)
	}
	parsed, err := url.ParseRequestURI(provider.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return providerInvalid("provider base url is invalid", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return providerInvalid("provider base url must use http or https", nil)
	}
	if _, err := adapterFor(provider.Protocol); err != nil {
		return err
	}
	if len(provider.APIKeys) == 0 {
		return providerInvalid("provider api key is required", nil)
	}
	seenKeyIDs := map[string]struct{}{}
	seenKeyNames := map[string]struct{}{}
	for _, key := range provider.APIKeys {
		if strings.TrimSpace(key.ID) == "" {
			return providerInvalid("provider api key id is required", nil)
		}
		if strings.TrimSpace(key.Name) == "" {
			return providerInvalid("provider api key name is required", nil)
		}
		if strings.TrimSpace(key.Key) == "" {
			return providerInvalid("provider api key value is required", nil)
		}
		if _, ok := seenKeyIDs[key.ID]; ok {
			return providerInvalid("provider api key id must be unique", nil)
		}
		if _, ok := seenKeyNames[key.Name]; ok {
			return providerInvalid("provider api key name must be unique", nil)
		}
		seenKeyIDs[key.ID] = struct{}{}
		seenKeyNames[key.Name] = struct{}{}
	}
	seenModelIDs := map[string]struct{}{}
	for _, model := range provider.RegisteredModels {
		if strings.TrimSpace(model.ID) == "" {
			return providerInvalid("registered model id is required", nil)
		}
		if strings.TrimSpace(model.Name) == "" {
			return providerInvalid("registered model name is required", nil)
		}
		if strings.TrimSpace(model.SourceModelID) == "" {
			return providerInvalid("registered model sourceModelId is required", nil)
		}
		if _, ok := seenModelIDs[model.ID]; ok {
			return providerInvalid("registered model id must be unique", nil)
		}
		if model.SupportsReasoning && !types.IsReasoningEffort(model.DefaultReasoningEffort) {
			return providerInvalid("registered model defaultReasoningEffort is invalid", nil)
		}
		seenModelIDs[model.ID] = struct{}{}
	}
	return nil
}

func normalizeProvider(provider types.Provider) types.Provider {
	now := time.Now().UTC()
	provider.ID = strings.TrimSpace(provider.ID)
	provider.Name = strings.TrimSpace(provider.Name)
	provider.BaseURL = normalizeBaseURL(provider.BaseURL)
	provider.Key = strings.TrimSpace(provider.Key)
	provider.APIKeyStrategy = normalizeRotationStrategy(provider.APIKeyStrategy)

	apiKeys := make([]types.ProviderAPIKey, 0, len(provider.APIKeys)+1)
	if len(provider.APIKeys) == 0 && provider.Key != "" {
		apiKeys = append(apiKeys, types.ProviderAPIKey{ID: "legacy", Name: "默认 Key", Key: provider.Key, Enabled: true, Weight: 1, CreatedAt: now, UpdatedAt: now})
	}
	for _, key := range provider.APIKeys {
		key.ID = strings.TrimSpace(key.ID)
		key.Name = strings.TrimSpace(key.Name)
		key.Key = strings.TrimSpace(key.Key)
		key.Weight = positiveWeight(key.Weight)
		if key.CreatedAt.IsZero() {
			key.CreatedAt = now
		}
		if key.UpdatedAt.IsZero() {
			key.UpdatedAt = key.CreatedAt
		}
		apiKeys = append(apiKeys, key)
	}
	provider.APIKeys = apiKeys
	provider.Key = ""

	models := make([]types.ModelInfo, 0, len(provider.Models))
	for _, model := range provider.Models {
		model.ID = strings.TrimSpace(model.ID)
		model.Name = strings.TrimSpace(model.Name)
		if model.ID == "" {
			continue
		}
		if model.Name == "" {
			model.Name = model.ID
		}
		models = append(models, model)
	}
	provider.Models = models

	registered := make([]types.ProviderRegisteredModel, 0, len(provider.RegisteredModels))
	for _, model := range provider.RegisteredModels {
		model.ID = strings.TrimSpace(model.ID)
		model.Name = strings.TrimSpace(model.Name)
		model.SourceModelID = strings.TrimSpace(model.SourceModelID)
		if model.SupportsReasoning {
			model.DefaultReasoningEffort = types.NormalizeReasoningEffort(model.DefaultReasoningEffort, types.DefaultReasoningEffort)
		} else {
			model.DefaultReasoningEffort = ""
		}
		if model.Name == "" {
			model.Name = model.ID
		}
		if model.CreatedAt.IsZero() {
			model.CreatedAt = now
		}
		if model.UpdatedAt.IsZero() {
			model.UpdatedAt = model.CreatedAt
		}
		registered = append(registered, model)
	}
	provider.RegisteredModels = registered
	return provider
}

func (s *system) providerWithSelectedKey(provider types.Provider) (types.Provider, error) {
	provider = normalizeProvider(provider)
	candidates := make([]types.ProviderAPIKey, 0, len(provider.APIKeys))
	weights := make([]int, 0, len(provider.APIKeys))
	for _, key := range provider.APIKeys {
		if !key.Enabled {
			continue
		}
		if strings.TrimSpace(key.Key) == "" {
			return types.Provider{}, providerInvalid("enabled provider api key is empty", nil)
		}
		candidates = append(candidates, key)
		weights = append(weights, key.Weight)
	}
	if len(candidates) == 0 {
		return types.Provider{}, providerInvalid("provider has no enabled api key", nil)
	}
	index, err := s.pickProviderKeyIndex(provider.ID, provider.APIKeyStrategy, weights)
	if err != nil {
		return types.Provider{}, err
	}
	provider.Key = candidates[index].Key
	return provider, nil
}
