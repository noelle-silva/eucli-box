package everything

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"eucli-box/pkg/types"
)

// searchRequest 是一次已校验的搜索请求。
type searchRequest struct {
	Query          string
	Description    string
	ScopePath      string
	MaxResults     int
	TimeoutMs      int
	MaxOutputChars int
}

// parseSearchRequest 解析并校验一次搜索请求；所有非法输入都快速失败。
func parseSearchRequest(input types.ToolExecutionInput, config Config) (searchRequest, error) {
	query, err := stringArgument(input.Arguments, "query", true)
	if err != nil {
		return searchRequest{}, err
	}
	description, err := stringArgument(input.Arguments, "description", false)
	if err != nil {
		return searchRequest{}, err
	}
	scopePath, err := stringArgument(input.Arguments, "scopePath", false)
	if err != nil {
		return searchRequest{}, err
	}
	resolvedScope, err := resolveScopePath(input.HostWorkingDirectory, scopePath)
	if err != nil {
		return searchRequest{}, err
	}
	maxResults, err := intArgument(input.Arguments, "maxResults", 0)
	if err != nil {
		return searchRequest{}, err
	}
	if _, provided := input.Arguments["maxResults"]; provided && maxResults <= 0 {
		return searchRequest{}, fmt.Errorf("argument \"maxResults\" must be greater than zero")
	}
	timeoutMs, err := intArgument(input.Arguments, "timeoutMs", 0)
	if err != nil {
		return searchRequest{}, err
	}
	if timeoutMs < 0 {
		return searchRequest{}, fmt.Errorf("argument \"timeoutMs\" must not be negative")
	}
	maxOutputChars, err := effectiveMaxOutputChars(input, config)
	if err != nil {
		return searchRequest{}, err
	}
	return searchRequest{
		Query:          query,
		Description:    description,
		ScopePath:      resolvedScope,
		MaxResults:     maxResults,
		TimeoutMs:      timeoutMs,
		MaxOutputChars: maxOutputChars,
	}, nil
}

// stringArgument 读取字符串参数；required 为真时空值快速失败。
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

// intArgument 读取整数参数；缺省返回 fallback。
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
