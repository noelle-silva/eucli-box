package shellcommand

import (
	"context"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	config, err := loadConfig(input.ToolDirectory)
	if err != nil {
		return failure("load shell_command config", err, nil)
	}
	request, err := parseRequest(input, config)
	if err != nil {
		return failure("parse shell_command request", err, nil)
	}
	provider, err := selectProvider(config, request.Provider, input.ToolDirectory)
	if err != nil {
		return failure("select shell_command provider", err, map[string]any{"provider": effectiveProviderName(config, request.Provider)})
	}
	workdir, err := resolveWorkdir(input.HostWorkingDirectory, request.Workdir)
	if err != nil {
		return failure("resolve shell_command workdir", err, map[string]any{"provider": provider.Config.ID})
	}
	result := runProviderCommand(ctx, provider, request, workdir)
	metadata := map[string]any{
		"stdout":         result.Stdout,
		"stderr":         result.Stderr,
		"combinedOutput": result.CombinedOutput,
		"exitCode":       result.ExitCode,
		"timedOut":       result.TimedOut,
		"durationMs":     result.DurationMs,
		"workdir":        workdir,
		"provider":       provider.Config.ID,
		"truncated":      result.Truncated,
		"maxOutputChars": request.MaxOutputChars,
	}
	if strings.TrimSpace(request.Description) != "" {
		metadata["description"] = request.Description
	}
	if result.Error != "" {
		metadata["error"] = result.Error
		return types.ToolExecutionOutput{Status: types.ToolStatusFailed, Content: outputContent(result), Error: result.Error, Metadata: metadata}
	}
	if result.ExitCode != 0 {
		errorMessage := fmt.Sprintf("command exited with code %d", result.ExitCode)
		metadata["error"] = errorMessage
		return types.ToolExecutionOutput{Status: types.ToolStatusFailed, Content: outputContent(result), Error: errorMessage, Metadata: metadata}
	}
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: outputContent(result), Metadata: metadata}
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

func outputContent(result processResult) string {
	if strings.TrimSpace(result.CombinedOutput) != "" {
		return result.CombinedOutput
	}
	if result.Error != "" {
		return result.Error
	}
	return "Command completed with no output."
}

func effectiveProviderName(config Config, requestedProvider string) string {
	requestedProvider = strings.TrimSpace(requestedProvider)
	if requestedProvider != "" {
		return requestedProvider
	}
	return config.DefaultProvider
}
