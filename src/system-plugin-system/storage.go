package systemplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) loadUserConfig(ctx context.Context, pluginID string) (types.SystemPluginUserConfig, error) {
	path, err := s.userConfigFile(pluginID)
	if err != nil {
		return types.SystemPluginUserConfig{}, err
	}
	if err := ctx.Err(); err != nil {
		return types.SystemPluginUserConfig{}, pluginReadFailed("read cancelled", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return types.SystemPluginUserConfig{}, nil
		}
		return types.SystemPluginUserConfig{}, pluginReadFailed("failed to read system plugin user config", err)
	}
	var config types.SystemPluginUserConfig
	if err := json.Unmarshal(payload, &config); err != nil {
		return types.SystemPluginUserConfig{}, pluginReadFailed("failed to decode system plugin user config", err)
	}
	return normalizeUserConfig(config), nil
}

func (s *system) SavePluginUserConfig(ctx context.Context, pluginID string, config types.SystemPluginUserConfig) (types.SystemPluginView, error) {
	record, err := s.findRecord(ctx, pluginID)
	if err != nil {
		return types.SystemPluginView{}, err
	}
	config = normalizeUserConfig(config)
	if err := validateUserConfig(record.manifest, config); err != nil {
		return types.SystemPluginView{}, err
	}
	path, err := s.userConfigFile(record.manifest.ID)
	if err != nil {
		return types.SystemPluginView{}, err
	}
	if err := writeJSONFile(ctx, path, config); err != nil {
		return types.SystemPluginView{}, err
	}
	if record.manifest.LifecycleType == types.SystemPluginLifecycleCachedHeartbeat {
		s.clearCachedValues(record.manifest.ID)
		if err := s.refreshCachedPlugin(ctx, record.manifest.ID); err != nil {
			s.setFailure(record.manifest.ID, err.Error())
		}
	}
	return s.LoadPlugin(ctx, record.manifest.ID)
}

func (s *system) userConfigFile(pluginID string) (string, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return "", pluginInvalid("system plugin id is required", nil)
	}
	path := filepath.Join(s.dataDir, pluginID, "config.json")
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", pluginInvalid("failed to resolve system plugin user config path", err)
	}
	if !pathWithin(s.dataDir, abs) {
		return "", pluginInvalid("system plugin user config path must stay inside data directory", nil)
	}
	return filepath.Clean(abs), nil
}

func normalizeUserConfig(config types.SystemPluginUserConfig) types.SystemPluginUserConfig {
	userConfig := copyMap(config.UserConfig)
	nameOverrides := map[string]string{}
	for key, value := range config.PlaceholderNameOverrides {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		nameOverrides[key] = value
	}
	if len(nameOverrides) == 0 {
		nameOverrides = nil
	}
	return types.SystemPluginUserConfig{UserConfig: userConfig, PlaceholderNameOverrides: nameOverrides}
}

func validateUserConfig(manifest types.SystemPluginManifest, config types.SystemPluginUserConfig) error {
	knownInterfaces := map[string]struct{}{}
	for _, item := range manifest.PlaceholderInterfaces {
		knownInterfaces[item.ID] = struct{}{}
	}
	for interfaceID := range config.PlaceholderNameOverrides {
		if _, ok := knownInterfaces[interfaceID]; !ok {
			return pluginInvalid("system plugin placeholder name override references unknown interface", nil)
		}
	}
	if err := validateConfigValues(manifest.ConfigSchema, config.UserConfig); err != nil {
		return err
	}
	return nil
}

func validateConfigValues(schema map[string]any, values map[string]any) error {
	return validateConfigObject("", schema, values)
}

func validateConfigObject(path string, schema map[string]any, values map[string]any) error {
	properties := objectMap(schema["properties"])
	if len(properties) == 0 || len(values) == 0 {
		return nil
	}
	for key, value := range values {
		fieldSchema := objectMap(properties[key])
		if len(fieldSchema) == 0 {
			continue
		}
		fieldPath := joinConfigPath(path, key)
		if err := validateConfigValue(fieldPath, fieldSchema, value); err != nil {
			return err
		}
	}
	return nil
}

func validateConfigValue(path string, schema map[string]any, value any) error {
	allowedValues := stringSet(schema["enum"])
	if len(allowedValues) == 0 {
		if childValues := objectMap(value); len(childValues) > 0 {
			return validateConfigObject(path, schema, childValues)
		}
		return nil
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if _, ok := allowedValues[text]; !ok {
		return pluginInvalid("system plugin config value is outside declared options: "+path, nil)
	}
	return nil
}

func joinConfigPath(parent string, key string) string {
	key = strings.TrimSpace(key)
	if parent == "" {
		return key
	}
	if key == "" {
		return parent
	}
	return parent + "." + key
}

func objectMap(value any) map[string]any {
	if out, ok := value.(map[string]any); ok {
		return out
	}
	return nil
}

func stringSet(value any) map[string]struct{} {
	out := map[string]struct{}{}
	switch items := value.(type) {
	case []any:
		for _, item := range items {
			addStringSetItem(out, item)
		}
	case []string:
		for _, item := range items {
			addStringSetItem(out, item)
		}
	default:
		return nil
	}
	return out
}

func addStringSetItem(out map[string]struct{}, item any) {
	text := strings.TrimSpace(fmt.Sprint(item))
	if text == "" {
		return
	}
	out[text] = struct{}{}
}

func writeJSONFile(ctx context.Context, target string, value any) error {
	if err := ctx.Err(); err != nil {
		return pluginWriteFailed("write cancelled", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return pluginWriteFailed("failed to create system plugin config directory", err)
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return pluginWriteFailed("failed to encode system plugin config", err)
	}
	payload = append(payload, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(target), ".tmp-*.json")
	if err != nil {
		return pluginWriteFailed("failed to create temporary config file", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return pluginWriteFailed("failed to write temporary config file", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return pluginWriteFailed("failed to sync temporary config file", err)
	}
	if err := tmp.Close(); err != nil {
		return pluginWriteFailed("failed to close temporary config file", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return pluginWriteFailed("failed to replace system plugin config", err)
	}
	return nil
}
