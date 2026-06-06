package zhihusearch

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
	searchTypeZhihu  = "zhihu_search"
	searchTypeGlobal = "global_search"
)

type Config struct {
	BaseURL          string            `json:"baseURL"`
	APIKeyEnv        string            `json:"apiKeyEnv"`
	APIKeyUserConfig string            `json:"apiKeyUserConfig"`
	Endpoints        map[string]string `json:"endpoints"`
	Limits           LimitsConfig      `json:"limits"`
}

type LimitsConfig struct {
	DefaultTimeoutMs     int `json:"defaultTimeoutMs"`
	MaxTimeoutMs         int `json:"maxTimeoutMs"`
	DefaultCount         int `json:"defaultCount"`
	ZhihuSearchMaxCount  int `json:"zhihuSearchMaxCount"`
	GlobalSearchMaxCount int `json:"globalSearchMaxCount"`
	MaxOutputChars       int `json:"maxOutputChars"`
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
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.APIKeyEnv = strings.TrimSpace(config.APIKeyEnv)
	config.APIKeyUserConfig = strings.TrimSpace(config.APIKeyUserConfig)
	if err := validateAbsoluteURL("baseURL", config.BaseURL); err != nil {
		return Config{}, err
	}
	if config.APIKeyEnv == "" || config.APIKeyUserConfig == "" {
		return Config{}, fmt.Errorf("api key sources are required")
	}
	if config.Endpoints == nil {
		return Config{}, fmt.Errorf("endpoints are required")
	}
	for _, searchType := range []string{searchTypeZhihu, searchTypeGlobal} {
		endpoint := strings.TrimSpace(config.Endpoints[searchType])
		if endpoint == "" {
			return Config{}, fmt.Errorf("endpoint %q is required", searchType)
		}
		if isAbsoluteURL(endpoint) {
			if err := validateAbsoluteURL("endpoint "+searchType, endpoint); err != nil {
				return Config{}, err
			}
		} else if !strings.HasPrefix(endpoint, "/") {
			return Config{}, fmt.Errorf("endpoint %q must be absolute URL or absolute path", searchType)
		}
		config.Endpoints[searchType] = endpoint
	}
	if config.Limits.DefaultTimeoutMs <= 0 {
		return Config{}, fmt.Errorf("limits.defaultTimeoutMs must be greater than zero")
	}
	if config.Limits.MaxTimeoutMs < config.Limits.DefaultTimeoutMs {
		return Config{}, fmt.Errorf("limits.maxTimeoutMs must be greater than or equal to limits.defaultTimeoutMs")
	}
	if config.Limits.DefaultCount <= 0 {
		return Config{}, fmt.Errorf("limits.defaultCount must be greater than zero")
	}
	if config.Limits.ZhihuSearchMaxCount < config.Limits.DefaultCount {
		return Config{}, fmt.Errorf("limits.zhihuSearchMaxCount must be greater than or equal to limits.defaultCount")
	}
	if config.Limits.GlobalSearchMaxCount < config.Limits.ZhihuSearchMaxCount {
		return Config{}, fmt.Errorf("limits.globalSearchMaxCount must be greater than or equal to limits.zhihuSearchMaxCount")
	}
	if config.Limits.MaxOutputChars <= 0 {
		return Config{}, fmt.Errorf("limits.maxOutputChars must be greater than zero")
	}
	return config, nil
}

func validateAbsoluteURL(name string, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s must be an absolute URL", name)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", name)
	}
	return nil
}

func isAbsoluteURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}
