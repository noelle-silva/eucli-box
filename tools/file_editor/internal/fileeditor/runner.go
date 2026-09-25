package fileeditor

import (
	"context"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("file_editor cancelled", err, nil)
	}
	if err := rejectConfigArguments(input); err != nil {
		return failure("parse file_editor request", err, nil)
	}
	config, err := loadConfig(input)
	if err != nil {
		return failure("load file_editor config", err, nil)
	}
	policy, err := newPathPolicy(types.ToolPathBaseDirectory(input))
	if err != nil {
		return failure("resolve file_editor base directory", err, nil)
	}
	action, err := stringArgument(input, "action", true)
	if err != nil {
		return failure("parse file_editor request", err, nil)
	}
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "write":
		return runWrite(input, config, policy)
	case "edit":
		return runEdit(input, config, policy)
	case "apply_patch":
		return runApplyPatch(input, config, policy)
	default:
		return failure("parse file_editor request", fmt.Errorf("unsupported action %q", action), map[string]any{"action": action})
	}
}

func baseMetadata(action string, resolved ResolvedPath) map[string]any {
	return map[string]any{
		"action":       action,
		"path":         resolved.Display,
		"absolutePath": resolved.Absolute,
	}
}
