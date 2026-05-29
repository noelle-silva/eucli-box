package modelprovider

import (
	"context"
	"net/url"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) SaveProvider(ctx context.Context, provider types.Provider) error {
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
	provider.BaseURL = normalizeBaseURL(provider.BaseURL)
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
	return provider, nil
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
	if strings.TrimSpace(provider.Key) == "" {
		return providerInvalid("provider key is required", nil)
	}
	if _, err := adapterFor(provider.Protocol); err != nil {
		return err
	}
	return nil
}
