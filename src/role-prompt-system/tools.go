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
	toolName = strings.TrimSpace(toolName)
	policy, err := s.GetToolPolicy(ctx, roleID)
	if err != nil {
		return "", err
	}
	if !slices.Contains(policy.Tools, toolName) {
		return "", roleToolModeMissing("tool is not in role whitelist", nil)
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
	seen := map[string]struct{}{}
	tools := make(map[string]struct{}, len(policy.Tools))
	for _, tool := range policy.Tools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			return roleInvalid("tool policy contains empty tool name", nil)
		}
		if _, ok := seen[tool]; ok {
			return roleInvalid("tool policy contains duplicate tool name", nil)
		}
		seen[tool] = struct{}{}
		tools[tool] = struct{}{}
	}
	for toolName, mode := range policy.RunModes {
		toolName = strings.TrimSpace(toolName)
		if toolName == "" {
			return roleInvalid("tool run mode contains empty tool name", nil)
		}
		if _, ok := tools[toolName]; !ok {
			return roleInvalid("tool policy run mode references a tool not in the whitelist", nil)
		}
		if mode != types.ToolRunDirect && mode != types.ToolRunAsk {
			return roleInvalid("tool run mode must be direct or ask", nil)
		}
	}
	for _, tool := range policy.Tools {
		if _, ok := policy.RunModes[strings.TrimSpace(tool)]; !ok {
			return roleInvalid("tool policy tool has no run mode configured", nil)
		}
	}
	nativeSeen := map[string]struct{}{}
	for _, tool := range policy.NativeTools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			return roleInvalid("tool policy contains empty native tool name", nil)
		}
		if _, ok := nativeSeen[tool]; ok {
			return roleInvalid("tool policy contains duplicate native tool name", nil)
		}
		if _, ok := tools[tool]; !ok {
			return roleInvalid("tool policy native tool references a tool not in the whitelist", nil)
		}
		nativeSeen[tool] = struct{}{}
	}
	return nil
}

func cloneToolPolicy(policy types.ToolPolicy) types.ToolPolicy {
	tools := slices.Clone(policy.Tools)
	nativeTools := slices.Clone(policy.NativeTools)
	runModes := make(map[string]types.ToolRunMode, len(policy.RunModes))
	for key, value := range policy.RunModes {
		runModes[key] = value
	}
	return types.ToolPolicy{Tools: tools, NativeTools: nativeTools, RunModes: runModes}
}
