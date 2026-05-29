package roleprompt

import (
	"context"

	"eucli-box/pkg/types"
)

func (s *system) BuildContext(ctx context.Context, roleID string, session types.Session, tools []types.ToolDefinition) (types.RoleContext, error) {
	role, err := s.LoadRole(ctx, roleID)
	if err != nil {
		return types.RoleContext{}, err
	}
	if err := validateRole(ctx, s.providers, role); err != nil {
		return types.RoleContext{}, err
	}
	return types.RoleContext{
		RoleID:      role.ID,
		RoleName:    role.Name,
		Avatar:      role.Avatar,
		Prompts:     sortedPrompts(role.Prompts),
		ModelConfig: role.ModelConfig,
		Messages:    cloneMessages(session.Messages),
		ToolPolicy:  cloneToolPolicy(role.ToolPolicy),
		Tools:       cloneTools(tools),
	}, nil
}

func cloneMessages(messages []types.Message) []types.Message {
	result := make([]types.Message, len(messages))
	copy(result, messages)
	return result
}

func cloneTools(tools []types.ToolDefinition) []types.ToolDefinition {
	result := make([]types.ToolDefinition, len(tools))
	copy(result, tools)
	return result
}
