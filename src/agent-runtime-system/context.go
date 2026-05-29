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
	filter := make(map[string]struct{}, len(policy.Tools))
	for _, tool := range policy.Tools {
		filter[tool] = struct{}{}
	}
	tools := make([]types.ToolDefinition, 0, len(summaries))
	for _, summary := range summaries {
		_, idOk := filter[summary.ID]
		_, nameOk := filter[summary.Name]
		matched := idOk || nameOk
		if policy.Mode == types.ToolPolicyBlacklist {
			matched = !matched
		}
		if !matched {
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
