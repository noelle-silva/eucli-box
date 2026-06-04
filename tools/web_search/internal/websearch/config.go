package websearch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	providerKindTavily    = "tavily"
	providerKindAnySearch = "anysearch"
)

type Config struct {
	DefaultProvider             string           `json:"defaultProvider"`
	AllowModelProviderSelection bool             `json:"allowModelProviderSelection"`
	Providers                   []ProviderConfig `json:"providers"`
	Limits                      LimitsConfig     `json:"limits"`
}

type ProviderConfig struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	Enabled          bool   `json:"enabled"`
	Endpoint         string `json:"endpoint"`
	APIKeyEnv        string `json:"apiKeyEnv,omitempty"`
	APIKeyUserConfig string `json:"apiKeyUserConfig,omitempty"`
	AnonymousAllowed bool   `json:"anonymousAllowed,omitempty"`
	MaxResults       int    `json:"maxResults"`
}

type LimitsConfig struct {
	DefaultTimeoutMs  int `json:"defaultTimeoutMs"`
	MaxTimeoutMs      int `json:"maxTimeoutMs"`
	DefaultMaxResults int `json:"defaultMaxResults"`
	MaxResults        int `json:"maxResults"`
	MaxOutputChars    int `json:"maxOutputChars"`
}

func loadConfig(toolDirectory string) (Config, error) {
	if strings.TrimSpace(toolDirectory) == "" {
		return Config{}, fmt.Errorf("toolDirectory is required")
	}
	payload, err := os.ReadFile(filepath.Join(toolDirectory, "config.json"))
	if err != nil {
		return Config{}, fmt.Errorf("read config.json: %w", err)
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode config.json: %w", err)
	}
	return normalizeConfig(config)
}

func normalizeConfig(config Config) (Config, error) {
	defaultProvider := strings.TrimSpace(config.DefaultProvider)
	if defaultProvider == "" {
		return Config{}, fmt.Errorf("defaultProvider is required")
	}
	if config.Limits.DefaultTimeoutMs <= 0 {
		return Config{}, fmt.Errorf("limits.defaultTimeoutMs must be greater than zero")
	}
	if config.Limits.MaxTimeoutMs < config.Limits.DefaultTimeoutMs {
		return Config{}, fmt.Errorf("limits.maxTimeoutMs must be greater than or equal to limits.defaultTimeoutMs")
	}
	if config.Limits.DefaultMaxResults <= 0 {
		return Config{}, fmt.Errorf("limits.defaultMaxResults must be greater than zero")
	}
	if config.Limits.MaxResults < config.Limits.DefaultMaxResults {
		return Config{}, fmt.Errorf("limits.maxResults must be greater than or equal to limits.defaultMaxResults")
	}
	if config.Limits.MaxOutputChars <= 0 {
		return Config{}, fmt.Errorf("limits.maxOutputChars must be greater than zero")
	}
	seen := map[string]struct{}{}
	defaultEnabled := false
	for index := range config.Providers {
		provider := &config.Providers[index]
		provider.ID = strings.TrimSpace(provider.ID)
		provider.Kind = strings.TrimSpace(provider.Kind)
		provider.Endpoint = strings.TrimSpace(provider.Endpoint)
		if provider.ID == "" {
			return Config{}, fmt.Errorf("provider id is required")
		}
		if _, ok := seen[provider.ID]; ok {
			return Config{}, fmt.Errorf("duplicate provider id %q", provider.ID)
		}
		seen[provider.ID] = struct{}{}
		if provider.Kind != providerKindTavily && provider.Kind != providerKindAnySearch {
			return Config{}, fmt.Errorf("provider %q kind %q is unsupported", provider.ID, provider.Kind)
		}
		if err := validateEndpoint(provider.ID, provider.Endpoint); err != nil {
			return Config{}, err
		}
		if provider.MaxResults <= 0 {
			provider.MaxResults = config.Limits.MaxResults
		}
		if provider.MaxResults > config.Limits.MaxResults {
			return Config{}, fmt.Errorf("provider %q maxResults exceeds limits.maxResults", provider.ID)
		}
		if provider.Kind == providerKindTavily && strings.TrimSpace(provider.APIKeyEnv) == "" && strings.TrimSpace(provider.APIKeyUserConfig) == "" {
			return Config{}, fmt.Errorf("provider %q must declare an API key source", provider.ID)
		}
		if provider.ID == defaultProvider && provider.Enabled {
			defaultEnabled = true
		}
	}
	if !defaultEnabled {
		return Config{}, fmt.Errorf("defaultProvider %q must exist and be enabled", defaultProvider)
	}
	return config, nil
}

func validateEndpoint(providerID string, endpoint string) error {
	if endpoint == "" {
		return fmt.Errorf("provider %q endpoint is required", providerID)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("provider %q endpoint must be an absolute URL", providerID)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("provider %q endpoint must use http or https", providerID)
	}
	return nil
}

func selectProvider(config Config, requestedProvider string) (ProviderConfig, error) {
	providerID := strings.TrimSpace(requestedProvider)
	if providerID == "" {
		providerID = strings.TrimSpace(config.DefaultProvider)
	} else if !config.AllowModelProviderSelection && providerID != strings.TrimSpace(config.DefaultProvider) {
		return ProviderConfig{}, fmt.Errorf("provider selection is disabled")
	}
	for _, provider := range config.Providers {
		if provider.ID != providerID {
			continue
		}
		if !provider.Enabled {
			return ProviderConfig{}, fmt.Errorf("provider %q is disabled", providerID)
		}
		return provider, nil
	}
	return ProviderConfig{}, fmt.Errorf("provider %q is not configured", providerID)
}

func effectiveProviderName(config Config, requestedProvider string) string {
	requestedProvider = strings.TrimSpace(requestedProvider)
	if requestedProvider != "" {
		return requestedProvider
	}
	return config.DefaultProvider
}
