package filereader

import (
	"context"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("file_reader cancelled", err, nil)
	}
	if err := rejectConfigArguments(input); err != nil {
		return failure("parse file_reader request", err, nil)
	}
	config, err := loadConfig(input)
	if err != nil {
		return failure("load file_reader config", err, nil)
	}
	policy, err := newPathPolicy(input.HostWorkingDirectory)
	if err != nil {
		return failure("resolve file_reader base directory", err, nil)
	}
	action, err := stringArgument(input, "action", true)
	if err != nil {
		return failure("parse file_reader request", err, nil)
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
	default:
		return failure("parse file_reader request", fmt.Errorf("unsupported action %q", action), map[string]any{"action": action})
	}
}

func baseMetadata(action string, resolved ResolvedPath) map[string]any {
	return map[string]any{
		"action":       action,
		"path":         resolved.Display,
		"absolutePath": resolved.Absolute,
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
