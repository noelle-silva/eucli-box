package context7

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	SearchEndpoint   string       `json:"searchEndpoint"`
	ContextEndpoint  string       `json:"contextEndpoint"`
	APIKeyEnv        string       `json:"apiKeyEnv"`
	APIKeyUserConfig string       `json:"apiKeyUserConfig"`
	AnonymousAllowed bool         `json:"anonymousAllowed"`
	Limits           LimitsConfig `json:"limits"`
}

type LimitsConfig struct {
	DefaultTimeoutMs int `json:"defaultTimeoutMs"`
	MaxTimeoutMs     int `json:"maxTimeoutMs"`
	MaxOutputChars   int `json:"maxOutputChars"`
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
	config.SearchEndpoint = strings.TrimSpace(config.SearchEndpoint)
	config.ContextEndpoint = strings.TrimSpace(config.ContextEndpoint)
	config.APIKeyEnv = strings.TrimSpace(config.APIKeyEnv)
	config.APIKeyUserConfig = strings.TrimSpace(config.APIKeyUserConfig)
	if err := validateEndpoint("searchEndpoint", config.SearchEndpoint); err != nil {
		return Config{}, err
	}
	if err := validateEndpoint("contextEndpoint", config.ContextEndpoint); err != nil {
		return Config{}, err
	}
	if config.Limits.DefaultTimeoutMs <= 0 {
		return Config{}, fmt.Errorf("limits.defaultTimeoutMs must be greater than zero")
	}
	if config.Limits.MaxTimeoutMs < config.Limits.DefaultTimeoutMs {
		return Config{}, fmt.Errorf("limits.maxTimeoutMs must be greater than or equal to limits.defaultTimeoutMs")
	}
	if config.Limits.MaxOutputChars <= 0 {
		return Config{}, fmt.Errorf("limits.maxOutputChars must be greater than zero")
	}
	if config.APIKeyEnv == "" && config.APIKeyUserConfig == "" && !config.AnonymousAllowed {
		return Config{}, fmt.Errorf("api key source is required when anonymous access is disabled")
	}
	return config, nil
}

func validateEndpoint(name string, endpoint string) error {
	if endpoint == "" {
		return fmt.Errorf("%s is required", name)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s must be an absolute URL", name)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", name)
	}
	return nil
}
