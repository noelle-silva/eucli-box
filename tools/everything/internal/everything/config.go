package everything

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"eucli-box/pkg/types"
)

type Config struct {
	DefaultProvider string           `json:"defaultProvider"`
	ESPathEnv       string           `json:"esPathEnv"`
	Providers       []ProviderConfig `json:"providers"`
	Runtime         RuntimeConfig    `json:"runtime"`
	Limits          LimitsConfig     `json:"limits"`
}

type ProviderConfig struct {
	ID                 string             `json:"id"`
	Mode               string             `json:"mode"`
	Enabled            bool               `json:"enabled"`
	Executables        []types.ToolBinary `json:"executables"`
	RuntimeExecutables []types.ToolBinary `json:"runtimeExecutables"`
}

type RuntimeConfig struct {
	Directory           string `json:"directory"`
	DefaultInstanceName string `json:"defaultInstanceName"`
	ReadyTimeoutMs      int    `json:"readyTimeoutMs"`
	ProbeIntervalMs     int    `json:"probeIntervalMs"`
}

type LimitsConfig struct {
	DefaultTimeoutMs        int `json:"defaultTimeoutMs"`
	MaxTimeoutMs            int `json:"maxTimeoutMs"`
	DefaultConnectTimeoutMs int `json:"defaultConnectTimeoutMs"`
	DefaultMaxResults       int `json:"defaultMaxResults"`
	MaxResults              int `json:"maxResults"`
	MaxOutputChars          int `json:"maxOutputChars"`
}

type selectedProvider struct {
	ID                string
	ESExecutable      string
	RuntimeExecutable string
	ExecutableSource  string
	RuntimeSource     string
	Bundled           bool
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
	config.DefaultProvider = strings.TrimSpace(config.DefaultProvider)
	if config.DefaultProvider == "" {
		return Config{}, fmt.Errorf("defaultProvider is required")
	}
	config.ESPathEnv = strings.TrimSpace(config.ESPathEnv)
	if config.ESPathEnv == "" {
		return Config{}, fmt.Errorf("esPathEnv is required")
	}
	if err := validateRuntimeConfig(config.Runtime, config.Limits); err != nil {
		return Config{}, err
	}
	if err := validateLimits(config.Limits); err != nil {
		return Config{}, err
	}
	if err := validateProviders(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func validateRuntimeConfig(runtimeConfig RuntimeConfig, limits LimitsConfig) error {
	directory := strings.TrimSpace(runtimeConfig.Directory)
	if directory == "" {
		return fmt.Errorf("runtime.directory is required")
	}
	if filepath.IsAbs(directory) || filepath.VolumeName(directory) != "" {
		return fmt.Errorf("runtime.directory must be relative")
	}
	if strings.TrimSpace(runtimeConfig.DefaultInstanceName) == "" {
		return fmt.Errorf("runtime.defaultInstanceName is required")
	}
	if runtimeConfig.ReadyTimeoutMs <= 0 {
		return fmt.Errorf("runtime.readyTimeoutMs must be greater than zero")
	}
	if runtimeConfig.ProbeIntervalMs <= 0 || runtimeConfig.ProbeIntervalMs > runtimeConfig.ReadyTimeoutMs {
		return fmt.Errorf("runtime.probeIntervalMs must be between 1 and runtime.readyTimeoutMs")
	}
	if limits.MaxTimeoutMs > 0 && runtimeConfig.ReadyTimeoutMs > limits.MaxTimeoutMs {
		return fmt.Errorf("runtime.readyTimeoutMs must be less than or equal to limits.maxTimeoutMs")
	}
	return nil
}

func validateLimits(limits LimitsConfig) error {
	if limits.DefaultTimeoutMs <= 0 {
		return fmt.Errorf("limits.defaultTimeoutMs must be greater than zero")
	}
	if limits.MaxTimeoutMs < limits.DefaultTimeoutMs {
		return fmt.Errorf("limits.maxTimeoutMs must be greater than or equal to limits.defaultTimeoutMs")
	}
	if limits.DefaultConnectTimeoutMs <= 0 || limits.DefaultConnectTimeoutMs > limits.MaxTimeoutMs {
		return fmt.Errorf("limits.defaultConnectTimeoutMs must be between 1 and limits.maxTimeoutMs")
	}
	if limits.DefaultMaxResults <= 0 {
		return fmt.Errorf("limits.defaultMaxResults must be greater than zero")
	}
	if limits.MaxResults < limits.DefaultMaxResults {
		return fmt.Errorf("limits.maxResults must be greater than or equal to limits.defaultMaxResults")
	}
	if limits.MaxOutputChars <= 0 {
		return fmt.Errorf("limits.maxOutputChars must be greater than zero")
	}
	return nil
}

func validateProviders(config Config) error {
	if len(config.Providers) == 0 {
		return fmt.Errorf("providers are required")
	}
	seen := map[string]struct{}{}
	defaultEnabled := false
	for index := range config.Providers {
		provider := &config.Providers[index]
		provider.ID = strings.TrimSpace(provider.ID)
		provider.Mode = strings.TrimSpace(provider.Mode)
		if provider.ID == "" {
			return fmt.Errorf("provider id is required")
		}
		if _, ok := seen[provider.ID]; ok {
			return fmt.Errorf("duplicate provider id %q", provider.ID)
		}
		seen[provider.ID] = struct{}{}
		if provider.Mode != "bundled" {
			return fmt.Errorf("provider %q mode must be bundled", provider.ID)
		}
		if len(provider.Executables) == 0 {
			return fmt.Errorf("provider %q must declare es executables", provider.ID)
		}
		if len(provider.RuntimeExecutables) == 0 {
			return fmt.Errorf("provider %q must declare runtime executables", provider.ID)
		}
		if provider.ID == config.DefaultProvider && provider.Enabled {
			defaultEnabled = true
		}
	}
	if !defaultEnabled {
		return fmt.Errorf("defaultProvider %q must exist and be enabled", config.DefaultProvider)
	}
	return nil
}

func resolveSearchProvider(config Config, input types.ToolExecutionInput) (selectedProvider, error) {
	if value, err := optionalString(input.UserConfig, "esPath"); err != nil {
		return selectedProvider{}, err
	} else if value != "" {
		executable, source, err := resolveConfiguredExecutable(value, "userConfig.esPath")
		return selectedProvider{ID: "external", ESExecutable: executable, ExecutableSource: source, RuntimeSource: "external"}, err
	}
	if value := strings.TrimSpace(os.Getenv(config.ESPathEnv)); value != "" {
		executable, source, err := resolveConfiguredExecutable(value, config.ESPathEnv)
		return selectedProvider{ID: "external", ESExecutable: executable, ExecutableSource: source, RuntimeSource: "external"}, err
	}
	return resolveBundledProvider(config, input.ToolDirectory)
}

func resolveBundledProvider(config Config, toolDirectory string) (selectedProvider, error) {
	for _, provider := range config.Providers {
		if provider.ID != config.DefaultProvider {
			continue
		}
		if !provider.Enabled {
			return selectedProvider{}, fmt.Errorf("provider %q is disabled", provider.ID)
		}
		esExecutable, err := resolveBundledExecutable(toolDirectory, provider.ID, provider.Executables, "es")
		if err != nil {
			return selectedProvider{}, err
		}
		runtimeExecutable, err := resolveBundledExecutable(toolDirectory, provider.ID, provider.RuntimeExecutables, "runtime")
		if err != nil {
			return selectedProvider{}, err
		}
		return selectedProvider{ID: provider.ID, ESExecutable: esExecutable, RuntimeExecutable: runtimeExecutable, ExecutableSource: "bundled", RuntimeSource: "bundled", Bundled: true}, nil
	}
	return selectedProvider{}, fmt.Errorf("provider %q is not configured", config.DefaultProvider)
}

func resolveBundledExecutable(toolDirectory string, providerID string, executables []types.ToolBinary, label string) (string, error) {
	if strings.TrimSpace(toolDirectory) == "" {
		return "", fmt.Errorf("toolDirectory is required")
	}
	for _, candidate := range executables {
		if candidate.GOOS != runtime.GOOS || candidate.GOARCH != runtime.GOARCH {
			continue
		}
		path := strings.TrimSpace(candidate.Path)
		if path == "" {
			return "", fmt.Errorf("provider %q %s executable path is required", providerID, label)
		}
		if filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
			return "", fmt.Errorf("provider %q %s executable path must be relative", providerID, label)
		}
		resolved := filepath.Clean(filepath.Join(toolDirectory, filepath.FromSlash(path)))
		if !pathWithin(toolDirectory, resolved) {
			return "", fmt.Errorf("provider %q %s executable escapes tool directory", providerID, label)
		}
		if err := ensureExecutableFile(resolved, fmt.Sprintf("provider %q %s executable", providerID, label)); err != nil {
			return "", err
		}
		return resolved, nil
	}
	return "", fmt.Errorf("provider %q has no %s executable for %s/%s", providerID, label, runtime.GOOS, runtime.GOARCH)
}

func optionalString(values map[string]any, key string) (string, error) {
	if values == nil {
		return "", nil
	}
	value, ok := values[key]
	if !ok || value == nil {
		return "", nil
	}
	return stringValue(value, key)
}

func resolveConfiguredExecutable(value string, source string) (string, string, error) {
	path := strings.TrimSpace(value)
	if path == "" {
		return "", "", nil
	}
	if filepath.Base(path) == path && filepath.VolumeName(path) == "" {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return "", "", fmt.Errorf("%s command %q was not found on PATH", source, path)
		}
		return validateExecutablePath(resolved, source)
	}
	return validateExecutablePath(path, source)
}

func validateExecutablePath(path string, source string) (string, string, error) {
	if !filepath.IsAbs(path) {
		return "", "", fmt.Errorf("%s must be an absolute executable path or a command name on PATH", source)
	}
	resolved := filepath.Clean(path)
	if err := ensureExecutableFile(resolved, source); err != nil {
		return "", "", err
	}
	return resolved, source, nil
}

func ensureExecutableFile(path string, source string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s does not exist: %w", source, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s path is a directory", source)
	}
	return nil
}

func pathWithin(base string, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
