package scicalculator

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"eucli-box/pkg/types"
)

const pythonExecutableKey = "pythonExecutable"

type calculationRequest struct {
	Expression       string
	PythonExecutable string
	MaxOutputChars   int
	Description      string
}

func parseRequest(input types.ToolExecutionInput, config Config) (calculationRequest, error) {
	expression, err := stringArgument(input.Arguments, "expression", true)
	if err != nil {
		return calculationRequest{}, err
	}
	if len([]rune(expression)) > config.Limits.MaxExpressionChars {
		return calculationRequest{}, fmt.Errorf("argument \"expression\" must be at most %d characters", config.Limits.MaxExpressionChars)
	}
	pythonExecutable, err := configuredPythonExecutable(input, config, input.ToolBodyDirectory)
	if err != nil {
		return calculationRequest{}, err
	}
	if strings.TrimSpace(pythonExecutable) == "" {
		return calculationRequest{}, fmt.Errorf("pythonExecutable is required")
	}
	maxOutputChars, err := mergedInt(input, "maxOutputChars", config.Limits.MaxOutputChars)
	if err != nil {
		return calculationRequest{}, err
	}
	if maxOutputChars <= 0 || maxOutputChars > config.Limits.MaxOutputChars {
		return calculationRequest{}, fmt.Errorf("maxOutputChars must be between 1 and %d", config.Limits.MaxOutputChars)
	}
	description, err := mergedString(input, "description")
	if err != nil {
		return calculationRequest{}, err
	}
	return calculationRequest{Expression: expression, PythonExecutable: pythonExecutable, MaxOutputChars: maxOutputChars, Description: description}, nil
}

func configuredPythonExecutable(input types.ToolExecutionInput, config Config, toolBodyDir string) (string, error) {
	if bundled, ok, err := bundledPythonExecutable(config, toolBodyDir); err != nil {
		return "", err
	} else if ok {
		return bundled, nil
	}
	if value, ok := input.UserConfig[pythonExecutableKey]; ok && value != nil {
		return stringValue(value, pythonExecutableKey)
	}
	if strings.TrimSpace(config.PythonEnv) != "" {
		if value := strings.TrimSpace(os.Getenv(config.PythonEnv)); value != "" {
			return value, nil
		}
	}
	if value, ok := input.DefaultConfig[pythonExecutableKey]; ok && value != nil {
		return stringValue(value, pythonExecutableKey)
	}
	return strings.TrimSpace(config.DefaultPythonExecutable), nil
}

func bundledPythonExecutable(config Config, toolBodyDir string) (string, bool, error) {
	bundledPath := strings.TrimSpace(config.BundledPythonExecutable)
	if bundledPath == "" {
		return "", false, nil
	}
	if strings.TrimSpace(toolBodyDir) == "" {
		return "", false, nil
	}
	if filepath.IsAbs(bundledPath) || filepath.VolumeName(bundledPath) != "" {
		return "", false, fmt.Errorf("bundledPythonExecutable must be relative to tool directory")
	}
	resolved := filepath.Clean(filepath.Join(toolBodyDir, filepath.FromSlash(bundledPath)))
	if !pathWithin(toolBodyDir, resolved) {
		return "", false, fmt.Errorf("bundledPythonExecutable must stay inside tool body directory")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("stat bundled python executable: %w", err)
	}
	if info.IsDir() {
		return "", false, fmt.Errorf("bundledPythonExecutable points to a directory")
	}
	return resolved, true, nil
}

func pathWithin(base string, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
