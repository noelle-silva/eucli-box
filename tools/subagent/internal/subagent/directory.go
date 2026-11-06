package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"eucli-box/tools/subagent/internal/types"
)

// executeList 交出角色名册：工作区标识来自宿主注入——在工作区会话里就有，
// 不在工作区会话里拿不到，此时向宿主索取全局名册。
func executeList(ctx context.Context, input types.ToolExecutionInput, session SessionService) types.ToolExecutionOutput {
	if session == nil {
		return failure("当前没有宿主控制通道，无法读取角色名册", nil, nil)
	}
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	result, err := session.Request(ctx, types.ToolCapabilityRoleDirectory, types.ToolCapabilityAccessRead, types.ToolRoleDirectoryRequest{WorkspaceID: workspaceID})
	if err != nil {
		return failure("读取角色名册失败", err, nil)
	}
	if err := capabilityFailure(result); err != nil {
		return failure("读取角色名册失败", err, nil)
	}
	payload, err := json.Marshal(result.Payload)
	if err != nil {
		return failure("角色名册响应无效", err, nil)
	}
	var directory types.ToolRoleDirectoryResult
	if err := json.Unmarshal(payload, &directory); err != nil {
		return failure("角色名册响应无效", err, nil)
	}
	return types.ToolExecutionOutput{
		Status:   types.ToolStatusSuccess,
		Content:  roleDirectoryText(directory),
		Metadata: map[string]any{"workspaceId": directory.WorkspaceID, "roleCount": len(directory.Roles), "roles": directory.Roles},
	}
}

// roleDirectoryText 把名册排成模型直接可读的清单：一行一个角色，标注范围。
func roleDirectoryText(directory types.ToolRoleDirectoryResult) string {
	scope := "宿主注册的全部角色"
	if strings.TrimSpace(directory.WorkspaceID) != "" {
		scope = fmt.Sprintf("工作区 %s 注册的角色", strings.TrimSpace(directory.WorkspaceID))
	}
	if len(directory.Roles) == 0 {
		return scope + "：一个也没有。"
	}
	lines := []string{fmt.Sprintf("%s共 %d 个：", scope, len(directory.Roles))}
	for _, role := range directory.Roles {
		description := strings.TrimSpace(role.Description)
		if description == "" {
			description = "（无描述）"
		}
		lines = append(lines, fmt.Sprintf("- %s（%s）：%s", role.Name, role.ID, description))
	}
	return strings.Join(lines, "\n")
}
