package context7

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"eucli-box/pkg/types"
)

const (
	actionSearch = "search"
	actionDocs   = "docs"
	maxTextParam = 500
)

type lookupRequest struct {
	Action         string
	LibraryName    string
	LibraryID      string
	Query          string
	Fast           bool
	TimeoutMs      int
	MaxOutputChars int
	Description    string
}

func parseRequest(input types.ToolExecutionInput, config Config) (lookupRequest, error) {
	action, err := mergedString(input, "action")
	if err != nil {
		return lookupRequest{}, err
	}
	if action != actionSearch && action != actionDocs {
		return lookupRequest{}, fmt.Errorf("action must be search or docs")
	}
	query, err := stringArgument(input.Arguments, "query", true)
	if err != nil {
		return lookupRequest{}, err
	}
	if err := validateTextLimit("query", query); err != nil {
		return lookupRequest{}, err
	}
	description, err := mergedString(input, "description")
	if err != nil {
		return lookupRequest{}, err
	}
	fast, err := mergedBool(input, "fast", false)
	if err != nil {
		return lookupRequest{}, err
	}
	timeoutMs, err := mergedInt(input, "timeoutMs", config.Limits.DefaultTimeoutMs)
	if err != nil {
		return lookupRequest{}, err
	}
	if timeoutMs <= 0 || timeoutMs > config.Limits.MaxTimeoutMs {
		return lookupRequest{}, fmt.Errorf("timeoutMs must be between 1 and %d", config.Limits.MaxTimeoutMs)
	}
	maxOutputChars, err := mergedInt(input, "maxOutputChars", config.Limits.MaxOutputChars)
	if err != nil {
		return lookupRequest{}, err
	}
	if maxOutputChars <= 0 || maxOutputChars > config.Limits.MaxOutputChars {
		return lookupRequest{}, fmt.Errorf("maxOutputChars must be between 1 and %d", config.Limits.MaxOutputChars)
	}
	request := lookupRequest{Action: action, Query: query, Fast: fast, TimeoutMs: timeoutMs, MaxOutputChars: maxOutputChars, Description: description}
	switch action {
	case actionSearch:
		libraryName, err := stringArgument(input.Arguments, "libraryName", true)
		if err != nil {
			return lookupRequest{}, err
		}
		if err := validateTextLimit("libraryName", libraryName); err != nil {
			return lookupRequest{}, err
		}
		request.LibraryName = libraryName
	case actionDocs:
		libraryID, err := stringArgument(input.Arguments, "libraryId", true)
		if err != nil {
			return lookupRequest{}, err
		}
		if err := validateLibraryID(libraryID); err != nil {
			return lookupRequest{}, err
		}
		request.LibraryID = libraryID
	}
	return request, nil
}

func validateTextLimit(key string, value string) error {
	if len([]rune(strings.TrimSpace(value))) > maxTextParam {
		return fmt.Errorf("argument %q must be at most %d characters", key, maxTextParam)
	}
	return nil
}

func validateLibraryID(libraryID string) error {
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return fmt.Errorf("argument \"libraryId\" is required")
	}
	if !strings.HasPrefix(libraryID, "/") || strings.Contains(libraryID, " ") {
		return fmt.Errorf("argument \"libraryId\" must be a Context7 id such as /owner/repo")
	}
	return validateTextLimit("libraryId", libraryID)
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
