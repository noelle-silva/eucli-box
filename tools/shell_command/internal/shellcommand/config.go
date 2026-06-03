package shellcommand

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"eucli-box/pkg/types"
)

type Config struct {
	DefaultProvider             string           `json:"defaultProvider"`
	AllowModelProviderSelection bool             `json:"allowModelProviderSelection"`
	Providers                   []ProviderConfig `json:"providers"`
	Limits                      LimitsConfig     `json:"limits"`
}

type ProviderConfig struct {
	ID          string             `json:"id"`
	Kind        string             `json:"kind"`
	Mode        string             `json:"mode"`
	Enabled     bool               `json:"enabled"`
	Priority    int                `json:"priority"`
	Encoding    string             `json:"encoding"`
	Executables []types.ToolBinary `json:"executables"`
}

type LimitsConfig struct {
	DefaultTimeoutMs int `json:"defaultTimeoutMs"`
	MaxTimeoutMs     int `json:"maxTimeoutMs"`
	MaxOutputChars   int `json:"maxOutputChars"`
}

type selectedProvider struct {
	Config     ProviderConfig
	Executable string
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
	if err := validateConfig(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func validateConfig(config Config) error {
	defaultProvider := strings.TrimSpace(config.DefaultProvider)
	if defaultProvider == "" {
		return fmt.Errorf("defaultProvider is required")
	}
	if config.Limits.DefaultTimeoutMs <= 0 {
		return fmt.Errorf("limits.defaultTimeoutMs must be greater than zero")
	}
	if config.Limits.MaxTimeoutMs < config.Limits.DefaultTimeoutMs {
		return fmt.Errorf("limits.maxTimeoutMs must be greater than or equal to limits.defaultTimeoutMs")
	}
	if config.Limits.MaxOutputChars <= 0 {
		return fmt.Errorf("limits.maxOutputChars must be greater than zero")
	}
	seen := map[string]struct{}{}
	defaultEnabled := false
	for _, provider := range config.Providers {
		id := strings.TrimSpace(provider.ID)
		if id == "" {
			return fmt.Errorf("provider id is required")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate provider id %q", id)
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(provider.Kind) == "" {
			return fmt.Errorf("provider %q kind is required", id)
		}
		if provider.Mode != "bundled" {
			return fmt.Errorf("provider %q mode must be bundled", id)
		}
		if strings.ToLower(strings.TrimSpace(provider.Encoding)) != "utf-8" {
			return fmt.Errorf("provider %q encoding must be utf-8", id)
		}
		if len(provider.Executables) == 0 {
			return fmt.Errorf("provider %q must declare executables", id)
		}
		if id == defaultProvider && provider.Enabled {
			defaultEnabled = true
		}
	}
	if !defaultEnabled {
		return fmt.Errorf("defaultProvider %q must exist and be enabled", defaultProvider)
	}
	return nil
}

func selectProvider(config Config, requestedProvider string, toolDirectory string) (selectedProvider, error) {
	providerID := strings.TrimSpace(requestedProvider)
	if providerID == "" {
		providerID = strings.TrimSpace(config.DefaultProvider)
	} else if !config.AllowModelProviderSelection && providerID != config.DefaultProvider {
		return selectedProvider{}, fmt.Errorf("provider selection is disabled")
	}
	for _, provider := range config.Providers {
		if strings.TrimSpace(provider.ID) != providerID {
			continue
		}
		if !provider.Enabled {
			return selectedProvider{}, fmt.Errorf("provider %q is disabled", providerID)
		}
		executable, err := resolveBundledExecutable(toolDirectory, provider)
		if err != nil {
			return selectedProvider{}, err
		}
		return selectedProvider{Config: provider, Executable: executable}, nil
	}
	return selectedProvider{}, fmt.Errorf("provider %q is not configured", providerID)
}

func resolveBundledExecutable(toolDirectory string, provider ProviderConfig) (string, error) {
	for _, candidate := range provider.Executables {
		if candidate.GOOS != runtime.GOOS || candidate.GOARCH != runtime.GOARCH {
			continue
		}
		path := strings.TrimSpace(candidate.Path)
		if path == "" {
			return "", fmt.Errorf("provider %q executable path is required", provider.ID)
		}
		if filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
			return "", fmt.Errorf("provider %q bundled executable path must be relative", provider.ID)
		}
		resolved := filepath.Clean(filepath.Join(toolDirectory, path))
		if !pathWithin(toolDirectory, resolved) {
			return "", fmt.Errorf("provider %q executable escapes tool directory", provider.ID)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return "", fmt.Errorf("provider %q bundled executable is missing at %s", provider.ID, path)
		}
		if info.IsDir() {
			return "", fmt.Errorf("provider %q executable path is a directory", provider.ID)
		}
		return resolved, nil
	}
	return "", fmt.Errorf("provider %q has no executable for %s/%s", provider.ID, runtime.GOOS, runtime.GOARCH)
}

func pathWithin(base string, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
