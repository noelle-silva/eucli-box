package roleprompt

import (
	"context"
	"slices"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) GetToolPolicy(ctx context.Context, roleID string) (types.ToolPolicy, error) {
	role, err := s.LoadRole(ctx, roleID)
	if err != nil {
		return types.ToolPolicy{}, err
	}
	if err := validateToolPolicy(role.ToolPolicy); err != nil {
		return types.ToolPolicy{}, err
	}
	return cloneToolPolicy(role.ToolPolicy), nil
}

func (s *system) GetToolRunMode(ctx context.Context, roleID string, toolName string) (types.ToolRunMode, error) {
	if strings.TrimSpace(toolName) == "" {
		return "", roleInvalid("tool name is required", nil)
	}
	policy, err := s.GetToolPolicy(ctx, roleID)
	if err != nil {
		return "", err
	}
	mode, ok := policy.RunModes[toolName]
	if !ok {
		return "", roleToolModeMissing("tool run mode is missing", nil)
	}
	if mode != types.ToolRunDirect && mode != types.ToolRunAsk {
		return "", roleToolModeMissing("tool run mode is invalid", nil)
	}
	return mode, nil
}

func validateToolPolicy(policy types.ToolPolicy) error {
	if policy.Mode != types.ToolPolicyWhitelist && policy.Mode != types.ToolPolicyBlacklist {
		return roleInvalid("tool policy mode must be whitelist or blacklist", nil)
	}
	seen := map[string]struct{}{}
	for _, tool := range policy.Tools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			return roleInvalid("tool policy contains empty tool name", nil)
		}
		if _, ok := seen[tool]; ok {
			return roleInvalid("tool policy contains duplicate tool name", nil)
		}
		seen[tool] = struct{}{}
	}
	for toolName, mode := range policy.RunModes {
		if strings.TrimSpace(toolName) == "" {
			return roleInvalid("tool run mode contains empty tool name", nil)
		}
		if mode != types.ToolRunDirect && mode != types.ToolRunAsk {
			return roleInvalid("tool run mode must be direct or ask", nil)
		}
	}
	if policy.Mode == types.ToolPolicyWhitelist {
		for _, tool := range policy.Tools {
			if _, ok := policy.RunModes[tool]; !ok {
				return roleInvalid("tool policy tool has no run mode configured", nil)
			}
		}
	}
	return nil
}

func cloneToolPolicy(policy types.ToolPolicy) types.ToolPolicy {
	tools := slices.Clone(policy.Tools)
	runModes := make(map[string]types.ToolRunMode, len(policy.RunModes))
	for key, value := range policy.RunModes {
		runModes[key] = value
	}
	return types.ToolPolicy{Mode: policy.Mode, Tools: tools, RunModes: runModes}
}
