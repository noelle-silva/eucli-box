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
			return false, "tool policy contains empty tool name", permissionInvalid("tool policy contains empty tool name", nil)
		}
		tools[name] = struct{}{}
	}
	_, listed := tools[toolName]
	switch policy.Mode {
	case types.ToolPolicyWhitelist:
		if !listed {
			return false, "tool is not in role whitelist", nil
		}
		return true, "tool is allowed by role whitelist", nil
	case types.ToolPolicyBlacklist:
		if listed {
			return false, "tool is blocked by role blacklist", nil
		}
		return true, "tool is not blocked by role blacklist", nil
	default:
		return false, "tool policy mode is invalid", permissionInvalid("tool policy mode is invalid", nil)
	}
}
