package scicalculator

import (
	"context"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	config, err := loadConfig(input.ToolDirectory)
	if err != nil {
		return failure("load sci_calculator config", err, nil)
	}
	request, err := parseRequest(input, config)
	if err != nil {
		return failure("parse sci_calculator request", err, nil)
	}
	result, err := runPythonCalculator(ctx, input.ToolDirectory, request)
	metadata := map[string]any{
		"expression":       request.Expression,
		"pythonExecutable": request.PythonExecutable,
		"maxOutputChars":   request.MaxOutputChars,
		"durationMs":       result.DurationMs,
		"exitCode":         result.ExitCode,
	}
	if strings.TrimSpace(request.Description) != "" {
		metadata["description"] = request.Description
	}
	if strings.TrimSpace(result.Stderr) != "" {
		metadata["stderr"] = result.Stderr
	}
	if err != nil {
		return failure("execute sci_calculator request", err, metadata)
	}
	content, contentTruncated := truncateText(result.Content, request.MaxOutputChars)
	metadata["truncated"] = contentTruncated
	if result.Status == types.ToolStatusSuccess {
		return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: metadata}
	}
	errorMessage := result.Error
	if strings.TrimSpace(errorMessage) == "" {
		errorMessage = content
	}
	errorMessage, errorTruncated := truncateText(errorMessage, request.MaxOutputChars)
	metadata["truncated"] = contentTruncated || errorTruncated
	metadata["error"] = errorMessage
	return types.ToolExecutionOutput{Status: types.ToolStatusFailed, Content: content, Error: errorMessage, Metadata: metadata}
}

func failure(scope string, err error, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	errorMessage := scope
	if err != nil {
		errorMessage = scope + ": " + err.Error()
	}
	metadata["error"] = errorMessage
	return types.ToolExecutionOutput{Status: types.ToolStatusFailed, Content: errorMessage, Error: errorMessage, Metadata: metadata}
}

func truncateText(value string, maxChars int) (string, bool) {
	runes := []rune(value)
	if maxChars <= 0 || len(runes) <= maxChars {
		return value, false
	}
	return string(runes[:maxChars]), true
}

func invalidCalculatorOutput(message string) error {
	return fmt.Errorf("calculator output is invalid: %s", message)
}
