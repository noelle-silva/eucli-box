package toolcalling

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

// readRoleDirectory 按请求范围交出角色名册：工作区标识留空时给出全部角色，
// 带上时只给注册在该工作区里的角色。名册是只读事实，工具只能看，不能改。
func (c *capabilitySession) readRoleDirectory(ctx context.Context, payload json.RawMessage) toolcontrol.CapabilityResult {
	var request types.ToolRoleDirectoryRequest
	if len(bytes.TrimSpace(payload)) > 0 {
		if err := json.Unmarshal(payload, &request); err != nil {
			return capabilityFailed("角色名册请求格式无效")
		}
	}
	workspaceID := strings.TrimSpace(request.WorkspaceID)
	roleIDs, result := c.roleIDsInScope(ctx, workspaceID)
	if result != nil {
		return *result
	}
	return capabilitySuccess(types.ToolRoleDirectoryResult{WorkspaceID: workspaceID, Roles: c.roleInfos(ctx, roleIDs)})
}

// roleIDsInScope 解析本次名册范围：工作区注册表或全部角色清单。
// 返回非空结果表示范围本身无法成立，直接作为失败回执。
func (c *capabilitySession) roleIDsInScope(ctx context.Context, workspaceID string) ([]string, *toolcontrol.CapabilityResult) {
	if workspaceID == "" {
		summaries, err := c.storage.ListRoles(ctx)
		if err != nil {
			failure := capabilityFailed("读取角色清单失败：" + err.Error())
			return nil, &failure
		}
		roleIDs := make([]string, 0, len(summaries))
		for _, summary := range summaries {
			roleIDs = append(roleIDs, summary.ID)
		}
		return roleIDs, nil
	}
	workspace, err := c.storage.LoadWorkspace(ctx, workspaceID)
	if err != nil {
		failure := capabilityFailed("读取工作区失败：" + err.Error())
		return nil, &failure
	}
	return workspace.RegisteredRoleIDs, nil
}

// roleInfos 把角色标识解析成名册项：注册表里残留的已删角色跳过，
// 不让一条陈旧引用挡住整份名册；顺序保持范围给出的顺序。
func (c *capabilitySession) roleInfos(ctx context.Context, roleIDs []string) []types.ToolRoleInfo {
	roles := []types.ToolRoleInfo{}
	seen := make(map[string]struct{}, len(roleIDs))
	for _, roleID := range roleIDs {
		roleID = strings.TrimSpace(roleID)
		if roleID == "" {
			continue
		}
		if _, ok := seen[roleID]; ok {
			continue
		}
		seen[roleID] = struct{}{}
		role, err := c.storage.LoadRole(ctx, roleID)
		if err != nil {
			continue
		}
		roles = append(roles, types.ToolRoleInfo{ID: role.ID, Name: role.Name, Description: role.Description})
	}
	return roles
}
