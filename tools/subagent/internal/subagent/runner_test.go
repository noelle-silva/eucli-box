package subagent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"eucli-box/tools/subagent/internal/toolcontrol"
	"eucli-box/tools/subagent/internal/types"
)

// fakeSession 是会话能力服务的测试替身：记录转发的请求并按设定应答。
type fakeSession struct {
	request types.ToolMessageRequest
	result  toolcontrol.CapabilityResult
	err     error
}

func (f *fakeSession) Request(_ context.Context, capability string, access string, payload any) (toolcontrol.CapabilityResult, error) {
	f.request, _ = payload.(types.ToolMessageRequest)
	return f.result, f.err
}

func runWith(session SessionService, arguments map[string]any) types.ToolExecutionOutput {
	input := types.ToolExecutionInput{ActionID: "t", ToolName: "subagent", Arguments: arguments}
	return Execute(context.Background(), input, session)
}

// TestRequestRequiresRoleAndMessage 钉住工具自己的最小校验：
// 发给谁与说什么缺一不可，缺失在本地当场拒绝，不打扰宿主。
func TestRequestRequiresRoleAndMessage(t *testing.T) {
	output := runWith(&fakeSession{}, map[string]any{"message": "在吗"})
	if output.Status != types.ToolStatusFailed || !strings.Contains(output.Error, "roleId") {
		t.Fatalf("missing roleId must fail locally: %#v", output)
	}

	output = runWith(&fakeSession{}, map[string]any{"roleId": "role-b"})
	if output.Status != types.ToolStatusFailed || !strings.Contains(output.Error, "message") {
		t.Fatalf("missing message must fail locally: %#v", output)
	}
}

// TestRequestCarriesSessionLocator 验证会话定位原样透传：
// 会话标识留空即新建，填写即续接，工具不改写、不补默认值。
func TestRequestCarriesSessionLocator(t *testing.T) {
	session := &fakeSession{result: toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolMessageResult{SessionID: "session-2", MessageID: "message-9", RoleID: "role-b", Message: "收到"}}}
	output := runWith(session, map[string]any{"roleId": "role-b", "workspaceId": "workspace-1", "sessionId": "session-2", "message": "在吗"})
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("success result = %#v", output)
	}
	want := types.ToolMessageRequest{RoleID: "role-b", WorkspaceID: "workspace-1", SessionID: "session-2", Message: "在吗"}
	if session.request != want {
		t.Fatalf("forwarded request = %#v, want %#v", session.request, want)
	}
	if !strings.Contains(output.Content, "收到") {
		t.Fatalf("content must carry the reply, got %q", output.Content)
	}
}

// TestCapabilityFailureKeepsHostReason 验证宿主给出的失败原因原样带回。
func TestCapabilityFailureKeepsHostReason(t *testing.T) {
	session := &fakeSession{result: toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusFailed, Error: "角色不存在：role-x"}}
	output := runWith(session, map[string]any{"roleId": "role-x", "message": "在吗"})
	if output.Status != types.ToolStatusFailed || !strings.Contains(output.Error, "角色不存在：role-x") {
		t.Fatalf("host reason must be preserved: %#v", output)
	}
}

// TestDeniedCapabilityExplainsGrant 验证未授权时给出可操作的提示。
func TestDeniedCapabilityExplainsGrant(t *testing.T) {
	session := &fakeSession{result: toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusDenied}}
	output := runWith(session, map[string]any{"roleId": "role-b", "message": "在吗"})
	if output.Status != types.ToolStatusFailed || !strings.Contains(output.Error, "未授权") {
		t.Fatalf("denied capability must explain the grant: %#v", output)
	}
}

// TestMissingControlChannelFailsLoudly 验证独立运行时明确失败，不伪造回复。
func TestMissingControlChannelFailsLoudly(t *testing.T) {
	output := runWith(nil, map[string]any{"roleId": "role-b", "message": "在吗"})
	if output.Status != types.ToolStatusFailed || !strings.Contains(output.Error, "控制通道") {
		t.Fatalf("missing control channel must fail loudly: %#v", output)
	}
}

// TestRequestErrorSurfaces 验证通道层错误不被吞掉。
func TestRequestErrorSurfaces(t *testing.T) {
	output := runWith(&fakeSession{err: errors.New("tool control client is closed")}, map[string]any{"roleId": "role-b", "message": "在吗"})
	if output.Status != types.ToolStatusFailed || !strings.Contains(output.Error, "closed") {
		t.Fatalf("transport error must surface: %#v", output)
	}
}

// runInWorkspace 带注入的工作区标识执行一次动作：模拟工作区会话里的工具输入。
func runInWorkspace(session SessionService, workspaceID string, arguments map[string]any) types.ToolExecutionOutput {
	input := types.ToolExecutionInput{ActionID: "t", ToolName: "subagent", Arguments: arguments, WorkspaceID: workspaceID}
	return Execute(context.Background(), input, session)
}

// TestSendFallsBackToInjectedWorkspace 钉住回落语义：工作区会话里调用方
// 不必重复交代工作区，宿主注入的标识就是默认落脚点。
func TestSendFallsBackToInjectedWorkspace(t *testing.T) {
	session := &fakeSession{result: toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolMessageResult{SessionID: "session-1", MessageID: "message-1", RoleID: "role-b", Message: "收到"}}}
	output := runInWorkspace(session, "workspace-1", map[string]any{"roleId": "role-b", "message": "在吗"})
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("send result = %#v", output)
	}
	if session.request.WorkspaceID != "workspace-1" {
		t.Fatalf("send must fall back to the injected workspace, got %#v", session.request)
	}
}

// TestSendExplicitWorkspaceWins 验证调用方写明的工作区优先于注入值，
// 保留「故意发给别的工作区」的可能。
func TestSendExplicitWorkspaceWins(t *testing.T) {
	session := &fakeSession{result: toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolMessageResult{SessionID: "session-1", MessageID: "message-1", RoleID: "role-b", Message: "收到"}}}
	output := runInWorkspace(session, "workspace-1", map[string]any{"roleId": "role-b", "message": "在吗", "workspaceId": "workspace-2"})
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("send result = %#v", output)
	}
	if session.request.WorkspaceID != "workspace-2" {
		t.Fatalf("explicit workspace must win, got %#v", session.request)
	}
}

// TestSendOutsideWorkspaceStaysPlain 验证没有注入时照常走普通会话。
func TestSendOutsideWorkspaceStaysPlain(t *testing.T) {
	session := &fakeSession{result: toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolMessageResult{SessionID: "session-1", MessageID: "message-1", RoleID: "role-b", Message: "收到"}}}
	output := runInWorkspace(session, "", map[string]any{"roleId": "role-b", "message": "在吗"})
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("send result = %#v", output)
	}
	if session.request.WorkspaceID != "" {
		t.Fatalf("plain session must stay plain, got %#v", session.request)
	}
}
