package toolcalling

import (
	"context"
	"encoding/json"
	"testing"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

// capabilityTestDirectoryTool 是声明并获授角色名册能力、且声明工作区标识注入的工具。
func capabilityTestDirectoryTool() types.ToolDefinition {
	return types.ToolDefinition{
		ID:   "directory-worker",
		Name: "directory-worker",
		Capabilities: []types.ToolCapability{
			{ID: types.ToolCapabilityWorkspaceIdentity, Access: types.ToolCapabilityAccessRead, Name: "工作区标识"},
			{ID: types.ToolCapabilityRoleDirectory, Access: types.ToolCapabilityAccessRead, Name: "读取角色名册"},
		},
		CapabilityGrants: map[string]bool{
			types.ToolCapabilityGrantKey(types.ToolCapabilityRoleDirectory, types.ToolCapabilityAccessRead): true,
		},
	}
}

// directoryStorage 准备两个角色，其中一个只注册在目标工作区里。
func directoryStorage(t *testing.T) *fakeToolStorage {
	t.Helper()
	storage := newFakeToolStorage()
	storage.roles["role-in"] = types.Role{ID: "role-in", Name: "圈内角色", Description: "注册在这个工作区的角色"}
	storage.roles["role-out"] = types.Role{ID: "role-out", Name: "圈外角色", Description: "没注册在这个工作区"}
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", Name: "工作区", RegisteredRoleIDs: []string{"role-in"}}
	return storage
}

func roleDirectoryRequest(t *testing.T, request types.ToolRoleDirectoryRequest) toolcontrol.CapabilityRequest {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal role directory request: %v", err)
	}
	return toolcontrol.CapabilityRequest{Capability: types.ToolCapabilityRoleDirectory, Access: types.ToolCapabilityAccessRead, Payload: payload}
}

func readDirectory(t *testing.T, result toolcontrol.CapabilityResult) types.ToolRoleDirectoryResult {
	t.Helper()
	if result.Status != toolcontrol.CapabilityStatusSuccess {
		t.Fatalf("role directory result = %#v", result)
	}
	payload, err := json.Marshal(result.Payload)
	if err != nil {
		t.Fatalf("marshal result payload: %v", err)
	}
	var directory types.ToolRoleDirectoryResult
	if err := json.Unmarshal(payload, &directory); err != nil {
		t.Fatalf("unmarshal role directory: %v", err)
	}
	return directory
}

// TestRoleDirectoryIsLimitedByWorkspace 钉住范围语义：带工作区标识时
// 只返回注册在该工作区里的角色，不把全局角色混进来。
func TestRoleDirectoryIsLimitedByWorkspace(t *testing.T) {
	storage := directoryStorage(t)
	session := newCapabilitySession(capabilityTestPlan(storage, capabilityTestDirectoryTool()), storage, nil)

	directory := readDirectory(t, session.handle(context.Background(), roleDirectoryRequest(t, types.ToolRoleDirectoryRequest{WorkspaceID: "workspace-1"})))
	if directory.WorkspaceID != "workspace-1" {
		t.Fatalf("scope = %q", directory.WorkspaceID)
	}
	if len(directory.Roles) != 1 || directory.Roles[0].ID != "role-in" {
		t.Fatalf("workspace directory = %#v", directory.Roles)
	}
	if directory.Roles[0].Name != "圈内角色" || directory.Roles[0].Description != "注册在这个工作区的角色" {
		t.Fatalf("role info must carry name and description, got %#v", directory.Roles[0])
	}
}

// TestRoleDirectoryWithoutWorkspaceReturnsEveryRole 验证不带工作区标识时
// 返回宿主注册的全部角色。
func TestRoleDirectoryWithoutWorkspaceReturnsEveryRole(t *testing.T) {
	storage := directoryStorage(t)
	session := newCapabilitySession(capabilityTestPlan(storage, capabilityTestDirectoryTool()), storage, nil)

	directory := readDirectory(t, session.handle(context.Background(), roleDirectoryRequest(t, types.ToolRoleDirectoryRequest{})))
	if directory.WorkspaceID != "" {
		t.Fatalf("global scope must not claim a workspace, got %q", directory.WorkspaceID)
	}
	if len(directory.Roles) != 2 {
		t.Fatalf("global directory = %#v", directory.Roles)
	}
}

// TestRoleDirectorySkipsVanishedRole 验证注册表里残留的已删角色不阻断名册，
// 也不会以空壳条目出现。
func TestRoleDirectorySkipsVanishedRole(t *testing.T) {
	storage := directoryStorage(t)
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", RegisteredRoleIDs: []string{"role-in", "role-gone"}}
	session := newCapabilitySession(capabilityTestPlan(storage, capabilityTestDirectoryTool()), storage, nil)

	directory := readDirectory(t, session.handle(context.Background(), roleDirectoryRequest(t, types.ToolRoleDirectoryRequest{WorkspaceID: "workspace-1"})))
	if len(directory.Roles) != 1 || directory.Roles[0].ID != "role-in" {
		t.Fatalf("vanished role must be skipped, got %#v", directory.Roles)
	}
}

// TestRoleDirectoryRequiresGrant 验证名册是索取类能力：未授权一律拦截。
func TestRoleDirectoryRequiresGrant(t *testing.T) {
	storage := directoryStorage(t)
	tool := capabilityTestDirectoryTool()
	tool.CapabilityGrants = nil
	session := newCapabilitySession(capabilityTestPlan(storage, tool), storage, nil)

	result := session.handle(context.Background(), roleDirectoryRequest(t, types.ToolRoleDirectoryRequest{}))
	if result.Status != toolcontrol.CapabilityStatusDenied {
		t.Fatalf("role directory must require a grant, got %#v", result)
	}
}

// TestWorkspaceIdentityNeedsNoGrant 验证工作区标识是注入类能力：
// 它随执行主动提供，不存在可授权的开关。
func TestWorkspaceIdentityNeedsNoGrant(t *testing.T) {
	if types.ToolCapabilityGrantRequired(types.ToolCapabilityWorkspaceIdentity, types.ToolCapabilityAccessRead) {
		t.Fatal("workspace identity must not require a user grant")
	}
	if !types.ToolCapabilityGrantRequired(types.ToolCapabilityRoleDirectory, types.ToolCapabilityAccessRead) {
		t.Fatal("role directory must require a user grant")
	}
}
