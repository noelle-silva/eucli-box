package webfetch

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"eucli-box/tools/web_fetch/internal/types"
)

// fetchRequest 是一次抓取动作的输入参数。
type fetchRequest struct {
	URL            string
	Description    string
	TimeoutMs      int
	MaxOutputChars int
}

func parseRequest(input types.ToolExecutionInput, config Config) (fetchRequest, error) {
	url, err := stringArgument(input.Arguments, "url", true)
	if err != nil {
		return fetchRequest{}, err
	}
	description, err := mergedString(input, "description")
	if err != nil {
		return fetchRequest{}, err
	}
	timeoutMs, err := intArgument(input.Arguments, "timeoutMs", config.DefaultTimeoutMs)
	if err != nil {
		return fetchRequest{}, err
	}
	if timeoutMs <= 0 {
		return fetchRequest{}, fmt.Errorf("timeoutMs must be greater than zero")
	}
	maxOutputChars, err := mergedInt(input, "maxOutputChars", config.MaxOutputChars)
	if err != nil {
		return fetchRequest{}, err
	}
	if maxOutputChars <= 0 || maxOutputChars > config.MaxOutputChars {
		return fetchRequest{}, fmt.Errorf("maxOutputChars must be between 1 and %d", config.MaxOutputChars)
	}
	return fetchRequest{URL: url, Description: description, TimeoutMs: timeoutMs, MaxOutputChars: maxOutputChars}, nil
}

func mergedString(input types.ToolExecutionInput, key string) (string, error) {
	if value, ok := input.Arguments[key]; ok && value != nil {
		return stringValue(value, key)
	}
	if value, ok := input.UserConfig[key]; ok && value != nil {
		return stringValue(value, key)
	}
	if value, ok := input.DefaultConfig[key]; ok && value != nil {
		return stringValue(value, key)
	}
	return "", nil
}

func mergedInt(input types.ToolExecutionInput, key string, fallback int) (int, error) {
	if value, ok := input.Arguments[key]; ok && value != nil {
		return intValue(value, key)
	}
	if value, ok := input.UserConfig[key]; ok && value != nil {
		return intValue(value, key)
	}
	if value, ok := input.DefaultConfig[key]; ok && value != nil {
		return intValue(value, key)
	}
	return fallback, nil
}

func stringArgument(args map[string]any, key string, required bool) (string, error) {
	value, ok := args[key]
	if !ok || value == nil {
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

func intArgument(args map[string]any, key string, fallback int) (int, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return fallback, nil
	}
	return intValue(value, key)
}

func stringValue(value any, key string) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("argument %q must be a string", key)
	}
	return strings.TrimSpace(text), nil
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
