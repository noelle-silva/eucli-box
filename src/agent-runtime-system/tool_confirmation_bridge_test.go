package agentruntime

import (
	"context"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// newConfirmationBridgeTestSystem 装配一个只用于确认转呈的最小运行时：
// 一个运行、一份会话、一个工具部件，足够覆盖投影与恢复。
func newConfirmationBridgeTestSystem(t *testing.T) (*system, *runRecord) {
	t.Helper()
	storage := newFakeRuntimeStorage()
	runtime := &system{
		storage:              storage,
		runs:                 map[string]*runRecord{},
		subscribers:          map[*eventSubscriber]struct{}{},
		confirmationOutcomes: map[string]types.ToolConfirmationOutcome{},
	}
	record := &runRecord{
		runID:             "run-parent",
		roleID:            "role-1",
		state:             types.RunState{ID: "run-parent", RoleID: "role-1", SessionID: "session-1", Status: types.RunStatusRunning},
		activeAssistantID: "message-1",
		session: types.Session{
			ID:     "session-1",
			RoleID: "role-1",
			Messages: []types.Message{{
				ID:   "message-1",
				Type: "assistant",
				Parts: []types.MessagePart{{
					ID:       "call-1",
					Type:     "tool",
					ToolName: "subagent",
					State:    "running",
				}},
			}},
		},
	}
	runtime.runs[record.runID] = record
	return runtime, record
}

func testBlockedAction() types.ToolAction {
	return types.ToolAction{ID: "action-1", CallRef: "call-1", ToolName: "subagent", Arguments: map[string]any{"roleId": "role-b", "message": "去写文件"}}
}

func testBlockedFact() types.ToolConfirmationFact {
	return types.ToolConfirmationFact{
		DecisionID: "decision-1",
		ToolName:   "file_editor",
		Reason:     "路径越出工作区围栏",
		Arguments:  map[string]any{"path": "E:/outside/a.txt"},
		SessionID:  "session-2",
		RoleID:     "role-b",
	}
}

// TestProjectToolConfirmationShowsBlockedWork 钉住投影语义：
// 待确认落在调用方那次工具部件上，展示内容换成被拦下的那件事。
func TestProjectToolConfirmationShowsBlockedWork(t *testing.T) {
	runtime, record := newConfirmationBridgeTestSystem(t)

	if err := runtime.projectToolConfirmation(context.Background(), record.runID, testBlockedAction(), testBlockedFact()); err != nil {
		t.Fatalf("projectToolConfirmation() error = %v", err)
	}
	part := record.session.Messages[0].Parts[0]
	if part.State != "needs_confirmation" {
		t.Fatalf("part state = %q", part.State)
	}
	if part.Decision == nil || part.Decision.ID != "decision-1" {
		t.Fatalf("part decision = %#v", part.Decision)
	}
	if part.ToolName != "subagent › file_editor" {
		t.Fatalf("part tool name = %q", part.ToolName)
	}
	if got := part.Input["path"]; got != "E:/outside/a.txt" {
		t.Fatalf("part input = %#v", part.Input)
	}
	if !strings.Contains(part.Decision.Reason, "路径越出工作区围栏") {
		t.Fatalf("decision reason = %q", part.Decision.Reason)
	}
}

// TestClearToolConfirmationProjectionRestoresRunning 钉住恢复语义：
// 裁决落定后部件回到运行中，待确认标记不再残留。
func TestClearToolConfirmationProjectionRestoresRunning(t *testing.T) {
	runtime, record := newConfirmationBridgeTestSystem(t)
	if err := runtime.projectToolConfirmation(context.Background(), record.runID, testBlockedAction(), testBlockedFact()); err != nil {
		t.Fatalf("projectToolConfirmation() error = %v", err)
	}

	runtime.clearToolConfirmationProjection(context.Background(), record.runID, testBlockedAction())
	part := record.session.Messages[0].Parts[0]
	if part.State != "running" {
		t.Fatalf("part state = %q", part.State)
	}
	if part.Decision != nil {
		t.Fatalf("part decision must be cleared, got %#v", part.Decision)
	}
}

// TestConfirmationOutcomeIsTakenOnce 验证结论只交给真正等它的那一次转呈。
func TestConfirmationOutcomeIsTakenOnce(t *testing.T) {
	runtime, _ := newConfirmationBridgeTestSystem(t)
	runtime.recordConfirmationOutcome("decision-1", true)

	outcome, ok := runtime.takeConfirmationOutcome("decision-1")
	if !ok || outcome.Status != types.ToolConfirmationStatusApproved {
		t.Fatalf("outcome = %#v ok=%v", outcome, ok)
	}
	if _, again := runtime.takeConfirmationOutcome("decision-1"); again {
		t.Fatal("outcome must be handed out only once")
	}
}

// TestConfirmationOutcomeRecordsRejection 验证拒绝同样被如实登记。
func TestConfirmationOutcomeRecordsRejection(t *testing.T) {
	runtime, _ := newConfirmationBridgeTestSystem(t)
	runtime.recordConfirmationOutcome("decision-2", false)

	outcome, ok := runtime.takeConfirmationOutcome("decision-2")
	if !ok || outcome.Status != types.ToolConfirmationStatusRejected {
		t.Fatalf("outcome = %#v ok=%v", outcome, ok)
	}
}

// TestConfirmationLabelAndReason 钉住卡片上的那句话：看得懂才算数。
func TestConfirmationLabelAndReason(t *testing.T) {
	if got := toolConfirmationLabel("subagent", "file_editor"); got != "subagent › file_editor" {
		t.Fatalf("label = %q", got)
	}
	if got := toolConfirmationLabel("subagent", "subagent"); got != "subagent" {
		t.Fatalf("label = %q", got)
	}
	if got := toolConfirmationReason(testBlockedFact()); got != "file_editor：路径越出工作区围栏" {
		t.Fatalf("reason = %q", got)
	}
}

// TestConfirmationOutcomeKeepsSubmittedDecisionID 钉住那个曾经出事的细节：
// 围栏确认应用后会重新裁决并换号，结论必须落在用户提交的那一个编号上，
// 否则等它的转呈方会等成「已失效」。
func TestConfirmationOutcomeKeepsSubmittedDecisionID(t *testing.T) {
	runtime, _ := newConfirmationBridgeTestSystem(t)
	runtime.recordConfirmationOutcome("permission-submitted", true)

	outcome, ok := runtime.takeConfirmationOutcome("permission-submitted")
	if !ok || outcome.Status != types.ToolConfirmationStatusApproved {
		t.Fatalf("submitted decision id must carry the outcome, got %#v ok=%v", outcome, ok)
	}
	if _, stale := runtime.takeConfirmationOutcome("permission-regenerated"); stale {
		t.Fatal("regenerated decision id must not be the one waiting parties look up")
	}
}
