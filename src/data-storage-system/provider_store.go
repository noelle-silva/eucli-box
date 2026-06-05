package datastorage

import (
	"context"
	"sort"

	"eucli-box/pkg/types"
)

func (s *system) SaveProvider(ctx context.Context, provider types.Provider) error {
	if _, err := cleanID(provider.ID); err != nil {
		return err
	}
	target, err := s.paths.providerDataFile(provider.ID)
	if err != nil {
		return err
	}
	if err := writeJSON(ctx, target, provider); err != nil {
		return err
	}
	return s.rebuildProviderIndex(ctx)
}

func (s *system) LoadProvider(ctx context.Context, providerID string) (types.Provider, error) {
	target, err := s.paths.providerDataFile(providerID)
	if err != nil {
		return types.Provider{}, err
	}
	return readJSON[types.Provider](ctx, target)
}

func (s *system) ListProviders(ctx context.Context) ([]types.ProviderSummary, error) {
	providers, err := readObjects[types.Provider](ctx, s.paths.providersRoot())
	if err != nil {
		return nil, err
	}
	summaries := make([]types.ProviderSummary, 0, len(providers))
	for _, provider := range providers {
		summaries = append(summaries, providerSummary(provider))
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })
	return summaries, nil
}

func providerSummary(provider types.Provider) types.ProviderSummary {
	apiKeyCount := len(provider.APIKeys)
	enabledAPIKeyCount := 0
	for _, key := range provider.APIKeys {
		if key.Enabled {
			enabledAPIKeyCount++
		}
	}
	if apiKeyCount == 0 && provider.Key != "" {
		apiKeyCount = 1
		enabledAPIKeyCount = 1
	}
	return types.ProviderSummary{
		ID:                   provider.ID,
		Name:                 provider.Name,
		Protocol:             provider.Protocol,
		APIKeyCount:          apiKeyCount,
		EnabledAPIKeyCount:   enabledAPIKeyCount,
		RegisteredModelCount: len(provider.RegisteredModels),
		UpdatedAt:            provider.UpdatedAt,
	}
}

func (s *system) DeleteProvider(ctx context.Context, providerID string) error {
	dir, err := s.paths.providerDir(providerID)
	if err != nil {
		return err
	}
	if err := moveToRecycle(ctx, s.paths, types.StorageItemProvider, providerID, dir); err != nil {
		return err
	}
	return s.rebuildProviderIndex(ctx)
}
