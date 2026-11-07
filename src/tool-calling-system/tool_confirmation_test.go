package toolcalling

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

// fakeConfirmationBridge 记录被转呈的确认事实，并按设定回传结论。
type fakeConfirmationBridge struct {
	plan   types.ToolRunPlan
	fact   types.ToolConfirmationFact
	result types.ToolConfirmationOutcome
	err    error
}

func (f *fakeConfirmationBridge) RelayToolConfirmation(_ context.Context, plan types.ToolRunPlan, fact types.ToolConfirmationFact) (types.ToolConfirmationOutcome, error) {
	f.plan = plan
	f.fact = fact
	return f.result, f.err
}

// capabilityTestConfirmationTool 是声明并获授外部确认转呈能力的工具定义。
func capabilityTestConfirmationTool() types.ToolDefinition {
	return types.ToolDefinition{
		ID:   "confirm-worker",
		Name: "confirm-worker",
		Capabilities: []types.ToolCapability{
			{ID: types.ToolCapabilityToolConfirmation, Access: types.ToolCapabilityAccessWrite, Name: "转呈外部确认"},
		},
		CapabilityGrants: map[string]bool{
			types.ToolCapabilityGrantKey(types.ToolCapabilityToolConfirmation, types.ToolCapabilityAccessWrite): true,
		},
	}
}

func confirmationCapabilityRequest(t *testing.T, fact types.ToolConfirmationFact) toolcontrol.CapabilityRequest {
	t.Helper()
	payload, err := json.Marshal(fact)
	if err != nil {
		t.Fatalf("marshal confirmation fact: %v", err)
	}
	return toolcontrol.CapabilityRequest{Capability: types.ToolCapabilityToolConfirmation, Access: types.ToolCapabilityAccessWrite, Payload: payload}
}

func testConfirmationFact() types.ToolConfirmationFact {
	return types.ToolConfirmationFact{DecisionID: "decision-1", ToolName: "file_editor", Reason: "路径越出工作区围栏", Arguments: map[string]any{"path": "E:/outside/a.txt"}, SessionID: "session-2", RoleID: "role-b"}
}

// TestRelayToolConfirmationForwardsFacts 钉住通用机制：工具把别处待确认交上来，
// 宿主拿到的是完整事实与「发起这次调用的运行」。
func TestRelayToolConfirmationForwardsFacts(t *testing.T) {
	storage := newFakeToolStorage()
	bridge := &fakeConfirmationBridge{result: types.ToolConfirmationOutcome{Status: types.ToolConfirmationStatusApproved, DecisionID: "decision-1"}}
	plan := capabilityTestPlan(storage, capabilityTestConfirmationTool())
	plan.Scope.RunID = "run-parent"
	session := newCapabilitySession(plan, storage, nil, bridge)

	result := session.handle(context.Background(), confirmationCapabilityRequest(t, testConfirmationFact()))
	if result.Status != toolcontrol.CapabilityStatusSuccess {
		t.Fatalf("relay result = %#v", result)
	}
	if bridge.plan.Scope.RunID != "run-parent" {
		t.Fatalf("calling run must be forwarded, got %#v", bridge.plan.Scope)
	}
	if bridge.fact.DecisionID != "decision-1" || bridge.fact.ToolName != "file_editor" || bridge.fact.SessionID != "session-2" {
		t.Fatalf("forwarded fact = %#v", bridge.fact)
	}
	outcome, ok := result.Payload.(types.ToolConfirmationOutcome)
	if !ok || outcome.Status != types.ToolConfirmationStatusApproved {
		t.Fatalf("outcome = %#v", result.Payload)
	}
}

// TestRelayToolConfirmationReturnsBridgeError 验证宿主给出的失败原因原样回到工具。
func TestRelayToolConfirmationReturnsBridgeError(t *testing.T) {
	storage := newFakeToolStorage()
	bridge := &fakeConfirmationBridge{err: errors.New("calling run was not found")}
	session := newCapabilitySession(capabilityTestPlan(storage, capabilityTestConfirmationTool()), storage, nil, bridge)

	result := session.handle(context.Background(), confirmationCapabilityRequest(t, testConfirmationFact()))
	if result.Status != toolcontrol.CapabilityStatusFailed || result.Error != "calling run was not found" {
		t.Fatalf("relay result = %#v", result)
	}
}

// TestRelayToolConfirmationWithoutBridgeFails 验证未接入实现方时明确失败。
func TestRelayToolConfirmationWithoutBridgeFails(t *testing.T) {
	storage := newFakeToolStorage()
	session := newCapabilitySession(capabilityTestPlan(storage, capabilityTestConfirmationTool()), storage, nil, nil)

	result := session.handle(context.Background(), confirmationCapabilityRequest(t, testConfirmationFact()))
	if result.Status != toolcontrol.CapabilityStatusFailed || result.Error == "" {
		t.Fatalf("relay result = %#v", result)
	}
}

// TestRelayToolConfirmationRequiresGrant 验证转呈确认是索取类能力：未授权一律拦截。
func TestRelayToolConfirmationRequiresGrant(t *testing.T) {
	storage := newFakeToolStorage()
	tool := capabilityTestConfirmationTool()
	tool.CapabilityGrants = nil
	bridge := &fakeConfirmationBridge{}
	session := newCapabilitySession(capabilityTestPlan(storage, tool), storage, nil, bridge)

	result := session.handle(context.Background(), confirmationCapabilityRequest(t, testConfirmationFact()))
	if result.Status != toolcontrol.CapabilityStatusDenied {
		t.Fatalf("relay result = %#v", result)
	}
	if bridge.fact.DecisionID != "" {
		t.Fatalf("denied request must not reach the bridge, got %#v", bridge.fact)
	}
}
