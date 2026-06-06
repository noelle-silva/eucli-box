package everything

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"eucli-box/pkg/types"
)

type searchRequest struct {
	Query            string
	Description      string
	ScopePath        string
	ScopePaths       []string
	ScopeIndexPaths  []string
	ScopeMode        string
	InstanceName     string
	MaxResults       int
	TimeoutMs        int
	ConnectTimeoutMs int
	MaxOutputChars   int
}

func parseRequest(input types.ToolExecutionInput, config Config) (searchRequest, error) {
	if _, ok := input.Arguments["action"]; ok {
		return searchRequest{}, fmt.Errorf("argument %q is not supported", "action")
	}
	query, err := stringArgument(input.Arguments, "query", true)
	if err != nil {
		return searchRequest{}, err
	}
	description, err := mergedString(input, "description")
	if err != nil {
		return searchRequest{}, err
	}
	scopePath, err := mergedString(input, "scopePath")
	if err != nil {
		return searchRequest{}, err
	}
	scope, err := resolveSearchScope(input.HostWorkingDirectory, scopePath)
	if err != nil {
		return searchRequest{}, err
	}
	instanceName, err := mergedString(input, "instanceName")
	if err != nil {
		return searchRequest{}, err
	}
	maxResults, err := mergedInt(input, "maxResults", config.Limits.DefaultMaxResults)
	if err != nil {
		return searchRequest{}, err
	}
	if maxResults <= 0 || maxResults > config.Limits.MaxResults {
		return searchRequest{}, fmt.Errorf("maxResults must be between 1 and %d", config.Limits.MaxResults)
	}
	timeoutMs, err := mergedInt(input, "timeoutMs", config.Limits.DefaultTimeoutMs)
	if err != nil {
		return searchRequest{}, err
	}
	if timeoutMs <= 0 || timeoutMs > config.Limits.MaxTimeoutMs {
		return searchRequest{}, fmt.Errorf("timeoutMs must be between 1 and %d", config.Limits.MaxTimeoutMs)
	}
	maxOutputChars, err := mergedInt(input, "maxOutputChars", config.Limits.MaxOutputChars)
	if err != nil {
		return searchRequest{}, err
	}
	if maxOutputChars <= 0 || maxOutputChars > config.Limits.MaxOutputChars {
		return searchRequest{}, fmt.Errorf("maxOutputChars must be between 1 and %d", config.Limits.MaxOutputChars)
	}
	connectTimeoutMs := minInt(config.Limits.DefaultConnectTimeoutMs, timeoutMs)
	return searchRequest{Query: query, Description: description, ScopePath: scope.SearchPath, ScopePaths: scope.DisplayPaths, ScopeIndexPaths: scope.IndexPaths, ScopeMode: scope.Mode, InstanceName: instanceName, MaxResults: maxResults, TimeoutMs: timeoutMs, ConnectTimeoutMs: connectTimeoutMs, MaxOutputChars: maxOutputChars}, nil
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

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
