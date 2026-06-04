package websearch

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"eucli-box/pkg/types"
)

type searchRequest struct {
	Provider                 string
	Query                    string
	Description              string
	MaxResults               int
	TimeoutMs                int
	MaxOutputChars           int
	IncludeContent           bool
	SearchDepth              string
	Topic                    string
	IncludeAnswer            bool
	IncludeImages            bool
	IncludeImageDescriptions bool
	IncludeRawContent        string
	Country                  string
	TimeRange                string
	StartDate                string
	EndDate                  string
	Domain                   string
	Tag                      string
	ContentTypes             []string
	Zone                     string
	Language                 string
	Params                   map[string]any
}

func parseRequest(input types.ToolExecutionInput, config Config) (searchRequest, error) {
	query, err := stringArgument(input.Arguments, "query", true)
	if err != nil {
		return searchRequest{}, err
	}
	provider, err := mergedString(input, "provider")
	if err != nil {
		return searchRequest{}, err
	}
	if strings.TrimSpace(provider) == "" {
		provider = config.DefaultProvider
	}
	description, err := mergedString(input, "description")
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
	includeContent, err := mergedBool(input, "includeContent", false)
	if err != nil {
		return searchRequest{}, err
	}
	searchDepth, err := mergedString(input, "searchDepth")
	if err != nil {
		return searchRequest{}, err
	}
	if strings.TrimSpace(searchDepth) == "" {
		searchDepth = "basic"
	}
	if !oneOf(searchDepth, "ultra-fast", "fast", "basic", "advanced") {
		return searchRequest{}, fmt.Errorf("searchDepth must be one of ultra-fast, fast, basic, advanced")
	}
	topic, err := mergedString(input, "topic")
	if err != nil {
		return searchRequest{}, err
	}
	if strings.TrimSpace(topic) == "" {
		topic = "general"
	}
	if !oneOf(topic, "general", "news", "finance") {
		return searchRequest{}, fmt.Errorf("topic must be one of general, news, finance")
	}
	includeAnswer, err := mergedBool(input, "includeAnswer", false)
	if err != nil {
		return searchRequest{}, err
	}
	includeImages, err := mergedBool(input, "includeImages", false)
	if err != nil {
		return searchRequest{}, err
	}
	includeImageDescriptions, err := mergedBool(input, "includeImageDescriptions", false)
	if err != nil {
		return searchRequest{}, err
	}
	includeRawContent, err := mergedString(input, "includeRawContent")
	if err != nil {
		return searchRequest{}, err
	}
	if includeRawContent != "" && !oneOf(includeRawContent, "text", "markdown") {
		return searchRequest{}, fmt.Errorf("includeRawContent must be text or markdown")
	}
	country, err := mergedString(input, "country")
	if err != nil {
		return searchRequest{}, err
	}
	timeRange, err := mergedString(input, "timeRange")
	if err != nil {
		return searchRequest{}, err
	}
	if timeRange != "" && !oneOf(timeRange, "day", "week", "month", "year", "d", "w", "m", "y") {
		return searchRequest{}, fmt.Errorf("timeRange must be one of day, week, month, year, d, w, m, y")
	}
	startDate, err := mergedString(input, "startDate")
	if err != nil {
		return searchRequest{}, err
	}
	endDate, err := mergedString(input, "endDate")
	if err != nil {
		return searchRequest{}, err
	}
	domain, err := mergedString(input, "domain")
	if err != nil {
		return searchRequest{}, err
	}
	tag, err := mergedString(input, "tag")
	if err != nil {
		return searchRequest{}, err
	}
	contentTypes, err := mergedStringList(input, "contentTypes")
	if err != nil {
		return searchRequest{}, err
	}
	zone, err := mergedString(input, "zone")
	if err != nil {
		return searchRequest{}, err
	}
	if zone != "" && !oneOf(zone, "cn", "intl") {
		return searchRequest{}, fmt.Errorf("zone must be cn or intl")
	}
	language, err := mergedString(input, "language")
	if err != nil {
		return searchRequest{}, err
	}
	params, err := mergedObject(input, "params")
	if err != nil {
		return searchRequest{}, err
	}
	return searchRequest{Provider: provider, Query: query, Description: description, MaxResults: maxResults, TimeoutMs: timeoutMs, MaxOutputChars: maxOutputChars, IncludeContent: includeContent, SearchDepth: searchDepth, Topic: topic, IncludeAnswer: includeAnswer, IncludeImages: includeImages, IncludeImageDescriptions: includeImageDescriptions, IncludeRawContent: includeRawContent, Country: country, TimeRange: timeRange, StartDate: startDate, EndDate: endDate, Domain: domain, Tag: tag, ContentTypes: contentTypes, Zone: zone, Language: language, Params: params}, nil
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

func mergedBool(input types.ToolExecutionInput, key string, fallback bool) (bool, error) {
	if value, ok := input.Arguments[key]; ok && value != nil {
		return boolValue(value, key)
	}
	if value, ok := input.UserConfig[key]; ok && value != nil {
		return boolValue(value, key)
	}
	if value, ok := input.DefaultConfig[key]; ok && value != nil {
		return boolValue(value, key)
	}
	return fallback, nil
}

func mergedStringList(input types.ToolExecutionInput, key string) ([]string, error) {
	if value, ok := input.Arguments[key]; ok && value != nil {
		return stringListValue(value, key)
	}
	if value, ok := input.UserConfig[key]; ok && value != nil {
		return stringListValue(value, key)
	}
	if value, ok := input.DefaultConfig[key]; ok && value != nil {
		return stringListValue(value, key)
	}
	return nil, nil
}

func mergedObject(input types.ToolExecutionInput, key string) (map[string]any, error) {
	if value, ok := input.Arguments[key]; ok && value != nil {
		return objectValue(value, key)
	}
	if value, ok := input.UserConfig[key]; ok && value != nil {
		return objectValue(value, key)
	}
	if value, ok := input.DefaultConfig[key]; ok && value != nil {
		return objectValue(value, key)
	}
	return nil, nil
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

func boolValue(value any, key string) (bool, error) {
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case string:
		trimmed := strings.ToLower(strings.TrimSpace(typed))
		if trimmed == "true" {
			return true, nil
		}
		if trimmed == "false" {
			return false, nil
		}
		return false, fmt.Errorf("argument %q must be a boolean", key)
	default:
		return false, fmt.Errorf("argument %q must be a boolean", key)
	}
}

func stringListValue(value any, key string) ([]string, error) {
	switch typed := value.(type) {
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("argument %q must contain only strings", key)
			}
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items, nil
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, nil
		}
		parts := strings.Split(trimmed, ",")
		items := make([]string, 0, len(parts))
		for _, part := range parts {
			if item := strings.TrimSpace(part); item != "" {
				items = append(items, item)
			}
		}
		return items, nil
	default:
		return nil, fmt.Errorf("argument %q must be a string array or comma-separated string", key)
	}
}

func objectValue(value any, key string) (map[string]any, error) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, nil
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return nil, fmt.Errorf("argument %q must be a JSON object", key)
		}
		return parsed, nil
	default:
		return nil, fmt.Errorf("argument %q must be an object", key)
	}
}

func oneOf(value string, allowed ...string) bool {
	value = strings.TrimSpace(value)
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
