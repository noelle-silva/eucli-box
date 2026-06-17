package roleprompt

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	if strings.TrimSpace(session.GroupID) != "" {
		return s.buildGroupContext(ctx, role, session, tools)
	}
	if strings.TrimSpace(session.WorkspaceID) != "" {
		return s.buildWorkspaceContext(ctx, role, session, tools)
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
		HookPromptPresetID: strings.TrimSpace(role.HookPromptPresetID),
		Tools:       cloneTools(tools),
		NativeTools: filterToolsByNames(tools, role.ToolPolicy.NativeTools),
	}, nil
}

func (s *system) buildWorkspaceContext(ctx context.Context, role types.Role, session types.Session, tools []types.ToolDefinition) (types.RoleContext, error) {
	if session.RoleID == "" || session.RoleID != role.ID {
		return types.RoleContext{}, roleInvalid("workspace session role does not match requested role", nil)
	}
	workspace, err := s.storage.LoadWorkspace(ctx, session.WorkspaceID)
	if err != nil {
		return types.RoleContext{}, roleStorageFailed("failed to load workspace", err)
	}
	return types.RoleContext{
		RoleID:      role.ID,
		RoleName:    role.Name,
		Avatar:      role.Avatar,
		Prompts:     workspaceContextPrompts(workspace, role),
		ModelConfig: role.ModelConfig,
		Messages:    cloneMessages(session.Messages),
		ToolPolicy:  cloneToolPolicy(role.ToolPolicy),
		HookPromptPresetID: strings.TrimSpace(role.HookPromptPresetID),
		Tools:       cloneTools(tools),
		NativeTools: filterToolsByNames(tools, role.ToolPolicy.NativeTools),
	}, nil
}

func workspaceContextPrompts(workspace types.Workspace, role types.Role) []types.PromptMessage {
	rolePrompts := sortedPrompts(role.Prompts)
	workspacePrompt := workspacePromptContent(workspace)
	if workspacePrompt == "" {
		return rolePrompts
	}
	now := time.Now().UTC()
	prompts := make([]types.PromptMessage, 0, len(rolePrompts)+1)
	prompts = append(prompts, rolePrompts...)
	order := 0
	if len(rolePrompts) > 0 {
		order = rolePrompts[len(rolePrompts)-1].Order + 1
	}
	prompts = append(prompts, types.PromptMessage{ID: "workspace-" + workspace.ID + "-prompt", Role: "system", Content: workspacePrompt, Order: order, CreatedAt: now, UpdatedAt: now})
	return prompts
}

func workspacePromptContent(workspace types.Workspace) string {
	blocks := []string{}
	name := strings.TrimSpace(workspace.Name)
	if name != "" {
		blocks = append(blocks, "当前工作区："+name)
	}
	if len(workspace.Directories) > 0 {
		lines := []string{"工作区注册目录："}
		for _, directory := range workspace.Directories {
			alias := strings.TrimSpace(directory.Alias)
			if alias == "" {
				alias = "未命名目录"
			}
			line := "- " + alias + "：" + strings.TrimSpace(directory.Path)
			if description := strings.TrimSpace(directory.Description); description != "" {
				line += "（" + description + "）"
			}
			lines = append(lines, line)
		}
		lines = append(lines, "路径围栏说明：文件读写和命令工作目录应优先保持在上述注册目录内；如需超出范围，系统会先询问用户。")
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	if prompt := strings.TrimSpace(workspace.Prompt); prompt != "" {
		blocks = append(blocks, "工作区自定义提示词：\n"+prompt)
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n"))
}

func (s *system) buildGroupContext(ctx context.Context, role types.Role, session types.Session, tools []types.ToolDefinition) (types.RoleContext, error) {
	group, err := s.storage.LoadChatGroup(ctx, session.GroupID)
	if err != nil {
		return types.RoleContext{}, roleStorageFailed("failed to load group", err)
	}
	if !chatGroupContainsRole(group, role.ID) {
		return types.RoleContext{}, roleInvalid("requested role is not a group member", nil)
	}
	roleNames, err := s.chatGroupRoleNames(ctx, group)
	if err != nil {
		return types.RoleContext{}, err
	}
	messages, err := groupContextMessages(session.Messages, roleNames, role.ID, role.Name)
	if err != nil {
		return types.RoleContext{}, err
	}
	return types.RoleContext{
		RoleID:      role.ID,
		RoleName:    role.Name,
		Avatar:      role.Avatar,
		Prompts:     groupContextPrompts(group, role),
		ModelConfig: role.ModelConfig,
		Messages:    messages,
		ToolPolicy:  cloneToolPolicy(role.ToolPolicy),
		HookPromptPresetID: strings.TrimSpace(role.HookPromptPresetID),
		Tools:       cloneTools(tools),
		NativeTools: filterToolsByNames(tools, role.ToolPolicy.NativeTools),
	}, nil
}

func chatGroupContainsRole(group types.ChatGroup, roleID string) bool {
	roleID = strings.TrimSpace(roleID)
	for _, memberRoleID := range group.MemberRoleIDs {
		if strings.TrimSpace(memberRoleID) == roleID {
			return true
		}
	}
	return false
}

func (s *system) chatGroupRoleNames(ctx context.Context, group types.ChatGroup) (map[string]string, error) {
	names := map[string]string{}
	for _, roleID := range group.MemberRoleIDs {
		roleID = strings.TrimSpace(roleID)
		if roleID == "" {
			continue
		}
		role, err := s.LoadRole(ctx, roleID)
		if err != nil {
			return nil, err
		}
		names[roleID] = role.Name
	}
	return names, nil
}

func groupContextPrompts(group types.ChatGroup, role types.Role) []types.PromptMessage {
	rolePrompts := sortedPrompts(role.Prompts)
	groupPrompt := strings.TrimSpace(group.Prompt)
	if groupPrompt == "" {
		return rolePrompts
	}
	now := time.Now().UTC()
	prompts := make([]types.PromptMessage, 0, len(rolePrompts)+1)
	prompts = append(prompts, types.PromptMessage{ID: "group-" + group.ID + "-prompt", Role: "system", Content: groupPrompt, Order: -1, CreatedAt: now, UpdatedAt: now})
	prompts = append(prompts, rolePrompts...)
	return prompts
}

func groupContextMessages(messages []types.Message, roleNames map[string]string, currentRoleID string, currentRoleName string) ([]types.Message, error) {
	result := make([]types.Message, 0, len(messages)+1)
	for _, message := range messages {
		message = cloneGroupContextMessage(message)
		switch message.Type {
		case "assistant":
			speakerRoleID := strings.TrimSpace(message.SpeakerRoleID)
			if speakerRoleID == "" {
				return nil, roleInvalid("group assistant message speakerRoleId is required", nil)
			}
			speakerName := strings.TrimSpace(roleNames[speakerRoleID])
			if speakerName == "" {
				return nil, roleInvalid("group assistant message speakerRoleId is not a group member", nil)
			}
			message.Content = fmt.Sprintf("[%s的发言]\n%s", speakerName, message.Content)
		case "user":
			message.Content = "[用户的发言]\n" + message.Content
		}
		result = append(result, message)
	}
	scheduler := groupSchedulerMessage(currentRoleID, currentRoleName)
	result = append(result, scheduler)
	return result, nil
}

func cloneGroupContextMessage(message types.Message) types.Message {
	if len(message.Parts) > 0 {
		message.Parts = append([]types.MessagePart(nil), message.Parts...)
	}
	if len(message.Attachments) > 0 {
		message.Attachments = append([]types.MessageAttachment(nil), message.Attachments...)
	}
	return message
}

func groupSchedulerMessage(roleID string, roleName string) types.Message {
	now := time.Now().UTC()
	content := fmt.Sprintf("[系统调度]\n现在轮到你发言。你的角色是：%s（%s）。请只以这个角色身份继续群聊。", strings.TrimSpace(roleName), strings.TrimSpace(roleID))
	return types.Message{ID: "group-scheduler-" + strings.TrimSpace(roleID), Type: "user", Content: content, CreatedAt: now, UpdatedAt: now}
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
