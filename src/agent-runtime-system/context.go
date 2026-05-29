package agentruntime

import (
	"context"

	"eucli-box/pkg/types"
)

func (s *system) buildRoleContext(ctx context.Context, roleID string, session types.Session) (types.RoleContext, error) {
	tools, err := s.availableTools(ctx, roleID)
	if err != nil {
		return types.RoleContext{}, err
	}
	roleContext, err := s.roles.BuildContext(ctx, roleID, session, tools)
	if err != nil {
		return types.RoleContext{}, runtimeRoleFailed("failed to build role context", err)
	}
	return roleContext, nil
}

func (s *system) availableTools(ctx context.Context, roleID string) ([]types.ToolDefinition, error) {
	policy, err := s.roles.GetToolPolicy(ctx, roleID)
	if err != nil {
		return nil, runtimeRoleFailed("failed to read role tool policy", err)
	}
	summaries, err := s.tools.ListTools(ctx)
	if err != nil {
		return nil, runtimeToolFailed("failed to list tools", err)
	}
	tools := make([]types.ToolDefinition, 0, len(summaries))
	for _, summary := range summaries {
		if !toolAllowedByPolicy(policy, summary.Name) && !toolAllowedByPolicy(policy, summary.ID) {
			continue
		}
		tool, err := s.tools.LoadTool(ctx, summary.ID)
		if err != nil {
			return nil, runtimeToolFailed("failed to load available tool", err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func toolAllowedByPolicy(policy types.ToolPolicy, toolName string) bool {
	listed := false
	for _, item := range policy.Tools {
		if item == toolName {
			listed = true
			break
		}
	}
	if policy.Mode == types.ToolPolicyWhitelist {
		return listed
	}
	if policy.Mode == types.ToolPolicyBlacklist {
		return !listed
	}
	return false
}
