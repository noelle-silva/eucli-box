package datastorage

import (
	"context"

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
	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	isNew := !dataFileExists(target)
	if err := writeJSON(ctx, target, provider); err != nil {
		return err
	}
	if err := s.syncOrderAfterSave(ctx, s.paths.providersRoot(), s.paths.providerOrderFile(), provider.ID, isNew); err != nil {
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
	byID := make(map[string]types.ProviderSummary, len(providers))
	for _, provider := range providers {
		byID[provider.ID] = providerSummary(provider)
	}
	ids, err := orderedObjectIDs(ctx, s.paths.providersRoot(), s.paths.providerOrderFile())
	if err != nil {
		return nil, err
	}
	summaries := make([]types.ProviderSummary, 0, len(ids))
	for _, id := range ids {
		if summary, ok := byID[id]; ok {
			summaries = append(summaries, summary)
		}
	}
	return summaries, nil
}

func (s *system) ReorderProviders(ctx context.Context, providerIDs []string) error {
	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	if err := s.reorderObjects(ctx, s.paths.providersRoot(), s.paths.providerOrderFile(), providerIDs); err != nil {
		return err
	}
	return s.rebuildProviderIndex(ctx)
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
	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	if err := s.moveToRecycle(ctx, types.StorageItemProvider, providerID, dir); err != nil {
		return err
	}
	if err := s.syncOrderAfterDelete(ctx, s.paths.providersRoot(), s.paths.providerOrderFile()); err != nil {
		return err
	}
	return s.rebuildProviderIndex(ctx)
}
