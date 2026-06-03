package shellcommand

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"eucli-box/pkg/types"
)

type commandRequest struct {
	Command        string
	Provider       string
	Workdir        string
	Description    string
	TimeoutMs      int
	MaxOutputChars int
}

func parseRequest(input types.ToolExecutionInput, config Config) (commandRequest, error) {
	command, err := stringArgument(input.Arguments, "command", true)
	if err != nil {
		return commandRequest{}, err
	}
	provider, err := stringArgument(input.Arguments, "provider", false)
	if err != nil {
		return commandRequest{}, err
	}
	workdir, err := stringArgument(input.Arguments, "workdir", false)
	if err != nil {
		return commandRequest{}, err
	}
	description, err := stringArgument(input.Arguments, "description", false)
	if err != nil {
		return commandRequest{}, err
	}
	timeoutMs, err := intArgument(input.Arguments, "timeoutMs", config.Limits.DefaultTimeoutMs)
	if err != nil {
		return commandRequest{}, err
	}
	if timeoutMs <= 0 {
		return commandRequest{}, fmt.Errorf("timeoutMs must be greater than zero")
	}
	if timeoutMs > config.Limits.MaxTimeoutMs {
		timeoutMs = config.Limits.MaxTimeoutMs
	}
	maxOutputChars, err := intArgument(input.Arguments, "maxOutputChars", config.Limits.MaxOutputChars)
	if err != nil {
		return commandRequest{}, err
	}
	if maxOutputChars <= 0 {
		return commandRequest{}, fmt.Errorf("maxOutputChars must be greater than zero")
	}
	if maxOutputChars > config.Limits.MaxOutputChars {
		maxOutputChars = config.Limits.MaxOutputChars
	}
	return commandRequest{Command: command, Provider: provider, Workdir: workdir, Description: description, TimeoutMs: timeoutMs, MaxOutputChars: maxOutputChars}, nil
}

func stringArgument(args map[string]any, key string, required bool) (string, error) {
	value, ok := args[key]
	if !ok || value == nil {
		if required {
			return "", fmt.Errorf("argument %q is required", key)
		}
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("argument %q must be a string", key)
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
			return fallback, nil
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
