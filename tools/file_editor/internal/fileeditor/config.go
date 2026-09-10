package fileeditor

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"eucli-box/pkg/types"
)

const defaultMaxFileBytes = 10 * 1024 * 1024

type Config struct {
	MaxFileBytes int64
}

func loadConfig(input types.ToolExecutionInput) (Config, error) {
	maxFileBytes, err := mergedInt64(input, "maxFileBytes", defaultMaxFileBytes)
	if err != nil {
		return Config{}, err
	}
	if maxFileBytes <= 0 {
		return Config{}, fmt.Errorf("maxFileBytes must be greater than zero")
	}
	return Config{MaxFileBytes: maxFileBytes}, nil
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
		"maxFileBytes",
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
