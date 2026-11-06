package subagent

import (
	"context"
	"strings"
	"testing"

	"eucli-box/tools/subagent/internal/toolcontrol"
	"eucli-box/tools/subagent/internal/types"
)

// directorySession 记录名册请求的载荷，并按设定应答。
type directorySession struct {
	request types.ToolRoleDirectoryRequest
	payload types.ToolRoleDirectoryResult
}

func (d *directorySession) Request(_ context.Context, capability string, access string, payload any) (toolcontrol.CapabilityResult, error) {
	if capability != types.ToolCapabilityRoleDirectory || access != types.ToolCapabilityAccessRead {
		return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusFailed, Error: "unexpected capability"}, nil
	}
	d.request, _ = payload.(types.ToolRoleDirectoryRequest)
	return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: d.payload}, nil
}

// TestListCarriesInjectedWorkspace 钉住注入语义：工作区会话里宿主注入标识，
// 名册请求必须带着它，不能由工具自己猜。
func TestListCarriesInjectedWorkspace(t *testing.T) {
	session := &directorySession{payload: types.ToolRoleDirectoryResult{WorkspaceID: "workspace-1", Roles: []types.ToolRoleInfo{{ID: "r_nova", Name: "Nova", Description: "女仆"}}}}
	input := types.ToolExecutionInput{Arguments: map[string]any{"action": "list"}, WorkspaceID: "workspace-1"}

	output := Execute(context.Background(), input, session)
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("list result = %#v", output)
	}
	if session.request.WorkspaceID != "workspace-1" {
		t.Fatalf("forwarded workspaceId = %q", session.request.WorkspaceID)
	}
	if !strings.Contains(output.Content, "Nova") || !strings.Contains(output.Content, "女仆") {
		t.Fatalf("content must carry role name and description, got %q", output.Content)
	}
	if !strings.Contains(output.Content, "workspace-1") {
		t.Fatalf("content must name the scope, got %q", output.Content)
	}
}

// TestListWithoutWorkspaceAsksForEveryRole 验证不在工作区会话里时不带工作区标识，
// 交由宿主返回全部角色。
func TestListWithoutWorkspaceAsksForEveryRole(t *testing.T) {
	session := &directorySession{payload: types.ToolRoleDirectoryResult{Roles: []types.ToolRoleInfo{{ID: "r_nova", Name: "Nova"}, {ID: "r_ice", Name: "小冰", Description: "测试助手"}}}}
	input := types.ToolExecutionInput{Arguments: map[string]any{"action": "list"}}

	output := Execute(context.Background(), input, session)
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("list result = %#v", output)
	}
	if session.request.WorkspaceID != "" {
		t.Fatalf("workspaceId must stay empty outside a workspace, got %q", session.request.WorkspaceID)
	}
	if !strings.Contains(output.Content, "全部角色") || !strings.Contains(output.Content, "小冰") {
		t.Fatalf("content = %q", output.Content)
	}
	if strings.Contains(output.Content, "workspace") {
		t.Fatalf("global scope must not claim a workspace, got %q", output.Content)
	}
}

// TestListHandlesEmptyDirectory 验证空名册如实说明，不伪造条目。
func TestListHandlesEmptyDirectory(t *testing.T) {
	session := &directorySession{payload: types.ToolRoleDirectoryResult{WorkspaceID: "workspace-9"}}
	output := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"action": "list"}, WorkspaceID: "workspace-9"}, session)
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("list result = %#v", output)
	}
	if !strings.Contains(output.Content, "一个也没有") {
		t.Fatalf("empty directory must say so, got %q", output.Content)
	}
}

// TestUnknownActionRejected 验证未知动作当场失败，不退化成发送。
func TestUnknownActionRejected(t *testing.T) {
	output := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"action": "delete"}}, &fakeSession{})
	if output.Status != types.ToolStatusFailed || !strings.Contains(output.Error, "send 或 list") {
		t.Fatalf("unknown action must fail loudly: %#v", output)
	}
}

// TestSendStillDefaultWithoutAction 验证未指定动作时仍按发送处理，旧调用不受影响。
func TestSendStillDefaultWithoutAction(t *testing.T) {
	session := &fakeSession{result: toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolMessageResult{SessionID: "session-1", MessageID: "message-1", RoleID: "role-b", Message: "收到"}}}
	output := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"roleId": "role-b", "message": "在吗"}}, session)
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("default action = %#v", output)
	}
	if session.request.RoleID != "role-b" {
		t.Fatalf("default action must send, got %#v", session.request)
	}
}
