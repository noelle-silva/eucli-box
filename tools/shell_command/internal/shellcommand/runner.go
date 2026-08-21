package shellcommand

import (
	"context"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	return ExecuteWithOutputHook(ctx, input, nil)
}

// ExecuteWithOutputHook runs the command and streams raw output chunks (stdout
// and stderr, as written) to hook. The hook is optional: it must not block the
// command execution and may be called concurrently by the two stream copy
// goroutines.
func ExecuteWithOutputHook(ctx context.Context, input types.ToolExecutionInput, hook func(payload []byte)) types.ToolExecutionOutput {
	config, err := loadConfig(input.ToolBodyDirectory)
	if err != nil {
		return failure("load shell_command config", err, nil)
	}
	request, err := parseRequest(input, config)
	if err != nil {
		return failure("parse shell_command request", err, nil)
	}
	if block, blocked := checkHardlineCommand(request.Command); blocked {
		return hardlineDeniedOutput(block)
	}
	provider, err := selectProvider(config, request.Provider, input.ToolBodyDirectory)
	if err != nil {
		return failure("select shell_command provider", err, map[string]any{"provider": effectiveProviderName(config, request.Provider)})
	}
	workdir, err := resolveWorkdir(input.HostWorkingDirectory, request.Workdir)
	if err != nil {
		return failure("resolve shell_command workdir", err, map[string]any{"provider": provider.Config.ID})
	}
	result := runProviderCommand(ctx, provider, request, workdir, hook)
	metadata := map[string]any{
		"stdout":                       result.Stdout,
		"stderr":                       result.Stderr,
		"combinedOutput":               result.CombinedOutput,
		"exitCode":                     result.ExitCode,
		"timedOut":                     result.TimedOut,
		"durationMs":                   result.DurationMs,
		"workdir":                      workdir,
		"provider":                     provider.Config.ID,
		"truncated":                    result.Truncated,
		"maxOutputChars":               request.MaxOutputChars,
		"encoding":                     provider.Config.Encoding,
		"outputBytesTotal":             result.CombinedBytes,
		"outputBytesStdout":            result.StdoutBytes,
		"outputBytesStderr":            result.StderrBytes,
		"outputLines":                  result.CombinedLines,
		"invalidUTF8":                  result.InvalidUTF8,
		"utf8ReplacementCount":         result.UTF8ReplacementCount,
		"stdoutInvalidUTF8":            result.StdoutInvalidUTF8,
		"stderrInvalidUTF8":            result.StderrInvalidUTF8,
		"stdoutUTF8ReplacementCount":   result.StdoutUTF8ReplacementCount,
		"stderrUTF8ReplacementCount":   result.StderrUTF8ReplacementCount,
		"combinedInvalidUTF8":          result.CombinedInvalidUTF8,
		"combinedUTF8ReplacementCount": result.CombinedUTF8ReplacementCount,
	}
	if result.FailureKind != "" {
		metadata["failureKind"] = result.FailureKind
	}
	if result.TerminationError != "" {
		metadata["terminationError"] = result.TerminationError
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
	content := resultContent(result)
	if result.InvalidUTF8 {
		warning := fmt.Sprintf("[shell_command warning] Command output contained non-UTF-8 bytes; %d byte(s) were replaced with '?'.", result.UTF8ReplacementCount)
		if strings.TrimSpace(content) == "" {
			return warning
		}
		return warning + "\n" + content
	}
	return content
}

func resultContent(result processResult) string {
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
