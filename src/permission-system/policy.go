package permission

import (
	"strings"

	"eucli-box/pkg/types"
)

func validateAction(action types.ToolAction) error {
	if strings.TrimSpace(action.ID) == "" {
		return permissionInvalid("tool action id is required", nil)
	}
	if strings.TrimSpace(action.ToolName) == "" {
		return permissionInvalid("tool name is required", nil)
	}
	return nil
}

func isAllowedByPolicy(policy types.ToolPolicy, toolName string) (bool, string, error) {
	tools := make(map[string]struct{}, len(policy.Tools))
	for _, tool := range policy.Tools {
		name := strings.TrimSpace(tool)
		if name == "" {
			return false, "permission internal error", permissionInvalid("tool policy contains empty tool name", nil)
		}
		tools[name] = struct{}{}
	}
	_, listed := tools[toolName]
	if !listed {
		return false, "tool is not in role whitelist", nil
	}
	return true, "tool is allowed by role whitelist", nil
}
