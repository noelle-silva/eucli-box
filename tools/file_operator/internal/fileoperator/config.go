package fileoperator

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"eucli-box/pkg/types"
)

const (
	defaultMaxFileBytes    = 10 * 1024 * 1024
	defaultReadLines       = 2000
	defaultMaxReadLines    = 2000
	defaultMaxLineChars    = 2000
	defaultMaxOutputChars  = 50000
	defaultMaxSearchResult = 100
)

type Config struct {
	WorkspaceRoots   []string
	MaxFileBytes     int64
	DefaultReadLines int
	MaxReadLines     int
	MaxLineChars     int
	MaxOutputChars   int
	MaxSearchResults int
}

func loadConfig(input types.ToolExecutionInput) (Config, error) {
	config := Config{
		MaxFileBytes:     defaultMaxFileBytes,
		DefaultReadLines: defaultReadLines,
		MaxReadLines:     defaultMaxReadLines,
		MaxLineChars:     defaultMaxLineChars,
		MaxOutputChars:   defaultMaxOutputChars,
		MaxSearchResults: defaultMaxSearchResult,
	}
	var err error
	config.WorkspaceRoots, err = mergedStringSlice(input, "workspaceRoots")
	if err != nil {
		return Config{}, err
	}
	if config.MaxFileBytes, err = mergedInt64(input, "maxFileBytes", config.MaxFileBytes); err != nil {
		return Config{}, err
	}
	if config.DefaultReadLines, err = mergedInt(input, "defaultReadLines", config.DefaultReadLines); err != nil {
		return Config{}, err
	}
	if config.MaxReadLines, err = mergedInt(input, "maxReadLines", config.MaxReadLines); err != nil {
		return Config{}, err
	}
	if config.MaxLineChars, err = mergedInt(input, "maxLineChars", config.MaxLineChars); err != nil {
		return Config{}, err
	}
	if config.MaxOutputChars, err = mergedInt(input, "maxOutputChars", config.MaxOutputChars); err != nil {
		return Config{}, err
	}
	if config.MaxSearchResults, err = mergedInt(input, "maxSearchResults", config.MaxSearchResults); err != nil {
		return Config{}, err
	}
	if config.MaxFileBytes <= 0 {
		return Config{}, fmt.Errorf("maxFileBytes must be greater than zero")
	}
	if config.DefaultReadLines <= 0 {
		return Config{}, fmt.Errorf("defaultReadLines must be greater than zero")
	}
	if config.MaxReadLines <= 0 {
		return Config{}, fmt.Errorf("maxReadLines must be greater than zero")
	}
	if config.DefaultReadLines > config.MaxReadLines {
		config.DefaultReadLines = config.MaxReadLines
	}
	if config.MaxLineChars <= 0 {
		return Config{}, fmt.Errorf("maxLineChars must be greater than zero")
	}
	if config.MaxOutputChars <= 0 {
		return Config{}, fmt.Errorf("maxOutputChars must be greater than zero")
	}
	if config.MaxSearchResults <= 0 {
		return Config{}, fmt.Errorf("maxSearchResults must be greater than zero")
	}
	return config, nil
}

func mergedValue(input types.ToolExecutionInput, key string) (any, bool) {
	if value, ok := input.UserConfig[key]; ok && value != nil {
		return value, true
	}
	if value, ok := input.DefaultConfig[key]; ok && value != nil {
		return value, true
	}
	return nil, false
}

func rejectConfigArguments(input types.ToolExecutionInput) error {
	configKeys := []string{
		"workspaceRoots",
		"maxFileBytes",
		"defaultReadLines",
		"maxReadLines",
		"maxLineChars",
		"maxSearchResults",
	}
	for _, key := range configKeys {
		if _, ok := argumentValue(input, key); ok {
			return fmt.Errorf("argument %q is configuration-only and cannot be supplied by a tool call", key)
		}
	}
	return nil
}

func argumentValue(input types.ToolExecutionInput, key string) (any, bool) {
	value, ok := input.Arguments[key]
	return value, ok && value != nil
}

func mergedString(input types.ToolExecutionInput, key string) (string, error) {
	value, ok := mergedValue(input, key)
	if !ok {
		return "", nil
	}
	return stringValue(value, key)
}

func stringArgument(input types.ToolExecutionInput, key string, required bool) (string, error) {
	value, ok := argumentValue(input, key)
	if !ok {
		if required {
			return "", fmt.Errorf("argument %q is required", key)
		}
		return "", nil
	}
	text, err := stringValue(value, key)
	if err != nil {
		return "", err
	}
	if required && strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("argument %q is required", key)
	}
	return text, nil
}

func rawStringArgument(input types.ToolExecutionInput, key string, required bool) (string, error) {
	value, ok := argumentValue(input, key)
	if !ok {
		if required {
			return "", fmt.Errorf("argument %q is required", key)
		}
		return "", nil
	}
	return stringValue(value, key)
}

func boolArgument(input types.ToolExecutionInput, key string, fallback bool) (bool, error) {
	value, ok := argumentValue(input, key)
	if !ok {
		return fallback, nil
	}
	return boolValue(value, key)
}

func intArgument(input types.ToolExecutionInput, key string, fallback int) (int, error) {
	value, ok := argumentValue(input, key)
	if !ok {
		return fallback, nil
	}
	return intValue(value, key)
}

func mergedInt(input types.ToolExecutionInput, key string, fallback int) (int, error) {
	value, ok := mergedValue(input, key)
	if !ok {
		return fallback, nil
	}
	return intValue(value, key)
}

func mergedInt64(input types.ToolExecutionInput, key string, fallback int64) (int64, error) {
	value, ok := mergedValue(input, key)
	if !ok {
		return fallback, nil
	}
	parsed, err := intValue(value, key)
	if err != nil {
		return 0, err
	}
	return int64(parsed), nil
}

func mergedStringSlice(input types.ToolExecutionInput, key string) ([]string, error) {
	value, ok := mergedValue(input, key)
	if !ok {
		return nil, nil
	}
	switch typed := value.(type) {
	case []string:
		return cleanStringSlice(typed), nil
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("config %q must contain only strings", key)
			}
			items = append(items, text)
		}
		return cleanStringSlice(items), nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, nil
		}
		return cleanStringSlice(strings.Split(typed, string(filepath.ListSeparator))), nil
	default:
		return nil, fmt.Errorf("config %q must be a string array", key)
	}
}

func cleanStringSlice(items []string) []string {
	cleaned := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			cleaned = append(cleaned, item)
		}
	}
	return cleaned
}

func stringValue(value any, key string) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("argument %q must be a string", key)
	}
	return text, nil
}

func boolValue(value any, key string) (bool, error) {
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return false, nil
		}
		parsed, err := strconv.ParseBool(trimmed)
		if err != nil {
			return false, fmt.Errorf("argument %q must be a boolean", key)
		}
		return parsed, nil
	default:
		return false, fmt.Errorf("argument %q must be a boolean", key)
	}
}

func intValue(value any, key string) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		if math.Trunc(typed) != typed {
			return 0, fmt.Errorf("argument %q must be an integer", key)
		}
		return int(typed), nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, fmt.Errorf("argument %q must be an integer", key)
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, fmt.Errorf("argument %q must be an integer", key)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("argument %q must be an integer", key)
	}
}
