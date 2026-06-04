package roleprompt

import (
	"context"
	"strings"

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
	if session.RoleID == "" || session.RoleID != roleID {
		return types.RoleContext{}, roleInvalid("session role does not match requested role", nil)
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
		NativeTools: filterToolsByNames(tools, role.ToolPolicy.NativeTools),
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

func filterToolsByNames(tools []types.ToolDefinition, names []string) []types.ToolDefinition {
	if len(tools) == 0 || len(names) == 0 {
		return nil
	}
	byName := map[string]types.ToolDefinition{}
	for _, tool := range tools {
		if id := cleanName(tool.ID); id != "" {
			byName[id] = tool
		}
		if name := cleanName(tool.Name); name != "" {
			byName[name] = tool
		}
	}
	result := make([]types.ToolDefinition, 0, len(names))
	seen := map[string]struct{}{}
	for _, name := range names {
		name = cleanName(name)
		tool, ok := byName[name]
		if !ok {
			continue
		}
		key := cleanName(tool.Name)
		if key == "" {
			key = cleanName(tool.ID)
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, tool)
	}
	return result
}

func cleanName(value string) string {
	return strings.TrimSpace(value)
}
