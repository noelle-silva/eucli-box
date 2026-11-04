package toolcalling

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

// fakeMessageSystem 记录能力层转发的发送意图，并按调用方设定的结果回传。
type fakeMessageSystem struct {
	request types.ToolMessageRequest
	result  types.ToolMessageResult
	err     error
}

func (f *fakeMessageSystem) DeliverToolMessage(ctx context.Context, request types.ToolMessageRequest) (types.ToolMessageResult, error) {
	f.request = request
	return f.result, f.err
}

// capabilityTestMessageTool 是声明并获授「发送会话消息」能力的工具定义。
func capabilityTestMessageTool() types.ToolDefinition {
	return types.ToolDefinition{
		ID:   "message-worker",
		Name: "message-worker",
		Capabilities: []types.ToolCapability{
			{ID: types.ToolCapabilitySessionMessage, Access: types.ToolCapabilityAccessWrite, Name: "发送会话消息"},
		},
		CapabilityGrants: map[string]bool{
			types.ToolCapabilityGrantKey(types.ToolCapabilitySessionMessage, types.ToolCapabilityAccessWrite): true,
		},
	}
}

func messageCapabilityRequest(t *testing.T, request types.ToolMessageRequest) toolcontrol.CapabilityRequest {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal tool message request: %v", err)
	}
	return toolcontrol.CapabilityRequest{Capability: types.ToolCapabilitySessionMessage, Access: types.ToolCapabilityAccessWrite, Payload: payload}
}

// TestSendSessionMessageForwardsIntent 钉住能力层的唯一职责：
// 把工具提交的发送意图原样交给实现方，并把实现方的结果原样回传。
func TestSendSessionMessageForwardsIntent(t *testing.T) {
	storage := newFakeToolStorage()
	messenger := &fakeMessageSystem{result: types.ToolMessageResult{SessionID: "session-2", MessageID: "message-9", RoleID: "role-b", Message: "收到啦"}}
	session := newCapabilitySession(capabilityTestPlan(storage, capabilityTestMessageTool()), storage, messenger)
	request := types.ToolMessageRequest{RoleID: "role-b", WorkspaceID: "workspace-1", SessionID: "session-2", Message: "你在吗"}

	result := session.handle(context.Background(), messageCapabilityRequest(t, request))
	if result.Status != toolcontrol.CapabilityStatusSuccess {
		t.Fatalf("send session message result = %#v", result)
	}
	if messenger.request != request {
		t.Fatalf("forwarded request = %#v, want %#v", messenger.request, request)
	}
	payload, ok := result.Payload.(types.ToolMessageResult)
	if !ok {
		t.Fatalf("result payload = %#v", result.Payload)
	}
	if payload.MessageID != "message-9" || payload.Message != "收到啦" {
		t.Fatalf("result payload = %#v", payload)
	}
}

// TestSendSessionMessageReturnsImplementationError 验证实现方的失败原因
// 原样回到工具手里，能力层不翻译、不掩盖。
func TestSendSessionMessageReturnsImplementationError(t *testing.T) {
	storage := newFakeToolStorage()
	messenger := &fakeMessageSystem{err: errors.New("角色不存在：role-x")}
	session := newCapabilitySession(capabilityTestPlan(storage, capabilityTestMessageTool()), storage, messenger)

	result := session.handle(context.Background(), messageCapabilityRequest(t, types.ToolMessageRequest{RoleID: "role-x", Message: "在吗"}))
	if result.Status != toolcontrol.CapabilityStatusFailed {
		t.Fatalf("send session message result = %#v", result)
	}
	if result.Error != "角色不存在：role-x" {
		t.Fatalf("failure error = %q", result.Error)
	}
}

// TestSendSessionMessageFailsWithoutImplementation 验证未接入实现方时明确失败，
// 不静默伪造一条结果。
func TestSendSessionMessageFailsWithoutImplementation(t *testing.T) {
	storage := newFakeToolStorage()
	session := newCapabilitySession(capabilityTestPlan(storage, capabilityTestMessageTool()), storage, nil)

	result := session.handle(context.Background(), messageCapabilityRequest(t, types.ToolMessageRequest{RoleID: "role-b", Message: "在吗"}))
	if result.Status != toolcontrol.CapabilityStatusFailed {
		t.Fatalf("send session message result = %#v", result)
	}
	if result.Error == "" {
		t.Fatal("failure must explain the missing message capability")
	}
}

// TestSendSessionMessageRequiresGrant 验证用户未授权时宿主统一拦截：
// 能力已声明但未获授权，请求不得到达实现方。
func TestSendSessionMessageRequiresGrant(t *testing.T) {
	storage := newFakeToolStorage()
	tool := capabilityTestMessageTool()
	tool.CapabilityGrants = nil
	messenger := &fakeMessageSystem{}
	session := newCapabilitySession(capabilityTestPlan(storage, tool), storage, messenger)

	result := session.handle(context.Background(), messageCapabilityRequest(t, types.ToolMessageRequest{RoleID: "role-b", Message: "在吗"}))
	if result.Status != toolcontrol.CapabilityStatusDenied {
		t.Fatalf("send session message result = %#v", result)
	}
	if messenger.request.RoleID != "" {
		t.Fatalf("denied request must not reach the implementation, got %#v", messenger.request)
	}
}
