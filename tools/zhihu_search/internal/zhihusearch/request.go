package zhihusearch

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"eucli-box/pkg/types"
)

type searchRequest struct {
	Query           string
	SearchType      string
	Description     string
	Count           int
	TimeoutMs       int
	MaxOutputChars  int
	BaseURL         string
	ZhihuSearchURL  string
	GlobalSearchURL string
}

func parseRequest(input types.ToolExecutionInput, config Config) (searchRequest, error) {
	query, err := firstString(input.Arguments, []string{"query", "q", "text", "Query"}, true)
	if err != nil {
		return searchRequest{}, err
	}
	searchType, err := mergedString(input, []string{"searchType", "search_type", "type", "scope", "command", "SearchType"})
	if err != nil {
		return searchRequest{}, err
	}
	if searchType == "" {
		searchType = searchTypeZhihu
	}
	searchType, err = normalizeSearchType(searchType)
	if err != nil {
		return searchRequest{}, err
	}
	description, err := mergedString(input, []string{"description"})
	if err != nil {
		return searchRequest{}, err
	}
	count, err := mergedInt(input, []string{"count", "maxResults", "max_results", "Count"}, config.Limits.DefaultCount)
	if err != nil {
		return searchRequest{}, err
	}
	count = clampCount(count, searchType, config)
	timeoutMs, err := mergedInt(input, []string{"timeoutMs", "timeout_ms"}, config.Limits.DefaultTimeoutMs)
	if err != nil {
		return searchRequest{}, err
	}
	if timeoutMs <= 0 || timeoutMs > config.Limits.MaxTimeoutMs {
		return searchRequest{}, fmt.Errorf("timeoutMs must be between 1 and %d", config.Limits.MaxTimeoutMs)
	}
	maxOutputChars, err := mergedInt(input, []string{"maxOutputChars", "max_output_chars"}, config.Limits.MaxOutputChars)
	if err != nil {
		return searchRequest{}, err
	}
	if maxOutputChars <= 0 || maxOutputChars > config.Limits.MaxOutputChars {
		return searchRequest{}, fmt.Errorf("maxOutputChars must be between 1 and %d", config.Limits.MaxOutputChars)
	}
	baseURL, err := configuredString(input, []string{"baseURL", "base_url"})
	if err != nil {
		return searchRequest{}, err
	}
	if baseURL == "" {
		baseURL = config.BaseURL
	}
	if err := validateAbsoluteURL("baseURL", baseURL); err != nil {
		return searchRequest{}, err
	}
	zhihuSearchURL, err := configuredString(input, []string{"zhihuSearchURL", "zhihu_search_url"})
	if err != nil {
		return searchRequest{}, err
	}
	globalSearchURL, err := configuredString(input, []string{"globalSearchURL", "global_search_url"})
	if err != nil {
		return searchRequest{}, err
	}
	for name, value := range map[string]string{"zhihuSearchURL": zhihuSearchURL, "globalSearchURL": globalSearchURL} {
		if value != "" {
			if err := validateAbsoluteURL(name, value); err != nil {
				return searchRequest{}, err
			}
		}
	}
	return searchRequest{Query: query, SearchType: searchType, Description: description, Count: count, TimeoutMs: timeoutMs, MaxOutputChars: maxOutputChars, BaseURL: baseURL, ZhihuSearchURL: zhihuSearchURL, GlobalSearchURL: globalSearchURL}, nil
}

func normalizeSearchType(value string) (string, error) {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
	switch normalized {
	case "zhihu", "site", "content", searchTypeZhihu:
		return searchTypeZhihu, nil
	case "global", "all", searchTypeGlobal:
		return searchTypeGlobal, nil
	default:
		return "", fmt.Errorf("searchType must be zhihu_search or global_search")
	}
}

func clampCount(count int, searchType string, config Config) int {
	if count < 1 {
		count = config.Limits.DefaultCount
	}
	maxCount := config.Limits.ZhihuSearchMaxCount
	if searchType == searchTypeGlobal {
		maxCount = config.Limits.GlobalSearchMaxCount
	}
	if count > maxCount {
		return maxCount
	}
	return count
}

func mergedString(input types.ToolExecutionInput, keys []string) (string, error) {
	for _, source := range []map[string]any{input.Arguments, input.UserConfig, input.DefaultConfig} {
		value, key, ok := firstValue(source, keys)
		if ok && value != nil {
			return stringValue(value, key)
		}
	}
	return "", nil
}

func mergedInt(input types.ToolExecutionInput, keys []string, fallback int) (int, error) {
	for _, source := range []map[string]any{input.Arguments, input.UserConfig, input.DefaultConfig} {
		value, key, ok := firstValue(source, keys)
		if ok && value != nil {
			return intValue(value, key)
		}
	}
	return fallback, nil
}

func configuredString(input types.ToolExecutionInput, keys []string) (string, error) {
	for _, source := range []map[string]any{input.UserConfig, input.DefaultConfig} {
		value, key, ok := firstValue(source, keys)
		if ok && value != nil {
			return stringValue(value, key)
		}
	}
	return "", nil
}

func firstString(source map[string]any, keys []string, required bool) (string, error) {
	value, key, ok := firstValue(source, keys)
	if !ok || value == nil {
		if required {
			return "", fmt.Errorf("argument %q is required", keys[0])
		}
		return "", nil
	}
	text, err := stringValue(value, key)
	if err != nil {
		return "", err
	}
	if required && text == "" {
		return "", fmt.Errorf("argument %q is required", keys[0])
	}
	return text, nil
}

func firstValue(source map[string]any, keys []string) (any, string, bool) {
	for _, key := range keys {
		if value, ok := source[key]; ok {
			return value, key, true
		}
	}
	return nil, "", false
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
