package fileoperator

import (
	"context"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("file_operator cancelled", err, nil)
	}
	if err := rejectConfigArguments(input); err != nil {
		return failure("parse file_operator request", err, nil)
	}
	config, err := loadConfig(input)
	if err != nil {
		return failure("load file_operator config", err, nil)
	}
	policy, err := newPathPolicy(input.HostWorkingDirectory, config)
	if err != nil {
		return failure("resolve file_operator workspace roots", err, nil)
	}
	action, err := stringArgument(input, "action", true)
	if err != nil {
		return failure("parse file_operator request", err, nil)
	}
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "read":
		return runRead(input, config, policy)
	case "list":
		return runList(input, config, policy)
	case "glob":
		return runGlob(ctx, input, config, policy)
	case "grep":
		return runGrep(ctx, input, config, policy)
	case "write":
		return runWrite(input, config, policy)
	case "edit":
		return runEdit(input, config, policy)
	case "apply_patch":
		return runApplyPatch(input, config, policy)
	default:
		return failure("parse file_operator request", fmt.Errorf("unsupported action %q", action), map[string]any{"action": action})
	}
}

func baseMetadata(action string, resolved ResolvedPath) map[string]any {
	return map[string]any{
		"action":        action,
		"path":          resolved.Display,
		"absolutePath":  resolved.Absolute,
		"workspaceRoot": resolved.Root,
	}
}

func effectiveMaxOutput(input types.ToolExecutionInput, config Config) (int, error) {
	maxOutput, err := intArgument(input, "maxOutputChars", config.MaxOutputChars)
	if err != nil {
		return 0, err
	}
	if maxOutput <= 0 || maxOutput > config.MaxOutputChars {
		maxOutput = config.MaxOutputChars
	}
	return maxOutput, nil
}

func effectiveReadWindow(input types.ToolExecutionInput, config Config) (int, int, error) {
	offset, err := intArgument(input, "offset", 1)
	if err != nil {
		return 0, 0, err
	}
	if offset <= 0 {
		offset = 1
	}
	limit, err := intArgument(input, "limit", config.DefaultReadLines)
	if err != nil {
		return 0, 0, err
	}
	if limit <= 0 {
		limit = config.DefaultReadLines
	}
	if limit > config.MaxReadLines {
		limit = config.MaxReadLines
	}
	return offset, limit, nil
}
