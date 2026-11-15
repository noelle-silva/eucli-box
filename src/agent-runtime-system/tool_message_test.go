package agentruntime

import (
	"context"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// TestWaitToolMessageRunReturnsPendingConfirmation 钉住第一步的结果语义：
// 目标运行抛出待确认时，等待方立刻拿到决策事实，不空等也不会误判成答复。
func TestWaitToolMessageRunReturnsPendingConfirmation(t *testing.T) {
	system := &system{}
	events := make(chan types.RunEvent, 1)
	plan := types.ToolRunPlan{
		Decision: types.PermissionDecision{ID: "decision-1", ToolName: "file_editor", Status: "needs_confirmation", Reason: "路径越出工作区围栏"},
		Tool:     types.ToolDefinition{ID: "file_editor", Name: "file_editor"},
		Action:   types.ToolAction{ID: "action-1", ToolName: "file_editor", Arguments: map[string]any{"path": "E:/outside/a.txt"}},
	}
	events <- types.RunEvent{RunID: "run-1", Type: toolConfirmationRequiredEventType, Payload: plan}
	state := types.RunState{ID: "run-1", SessionID: "session-1", RoleID: "role-1", Status: types.RunStatusRunning}

	terminal, stopped, confirmation, err := system.waitToolMessageRun(context.Background(), state, events)
	if err != nil {
		t.Fatalf("waitToolMessageRun() error = %v", err)
	}
	if stopped {
		t.Fatal("pending confirmation must not be reported as a stop")
	}
	if terminal.Status != types.RunStatusRunning {
		t.Fatalf("run must still be active, got %q", terminal.Status)
	}
	if confirmation == nil {
		t.Fatal("pending confirmation must be returned")
	}
	if confirmation.DecisionID != "decision-1" || confirmation.ToolName != "file_editor" {
		t.Fatalf("confirmation = %#v", confirmation)
	}
	if confirmation.Reason != "路径越出工作区围栏" {
		t.Fatalf("confirmation reason = %q", confirmation.Reason)
	}
	if confirmation.SessionID != "session-1" || confirmation.RoleID != "role-1" {
		t.Fatalf("confirmation must carry the session identity, got %#v", confirmation)
	}
	if got := confirmation.Arguments["path"]; got != "E:/outside/a.txt" {
		t.Fatalf("confirmation arguments = %#v", confirmation.Arguments)
	}
}

// TestWaitToolMessageRunIgnoresForeignRunEvents 验证其它运行的事件不会串到本次等待里。
func TestWaitToolMessageRunIgnoresForeignRunEvents(t *testing.T) {
	system := &system{}
	events := make(chan types.RunEvent, 2)
	events <- types.RunEvent{RunID: "run-other", Type: toolConfirmationRequiredEventType, Payload: types.ToolRunPlan{Decision: types.PermissionDecision{ID: "decision-x"}}}
	events <- types.RunEvent{RunID: "run-1", Payload: types.RunState{ID: "run-1", Status: types.RunStatusCompleted, SessionID: "session-1", RoleID: "role-1"}}
	state := types.RunState{ID: "run-1", SessionID: "session-1", RoleID: "role-1", Status: types.RunStatusRunning}

	terminal, _, confirmation, err := system.waitToolMessageRun(context.Background(), state, events)
	if err != nil {
		t.Fatalf("waitToolMessageRun() error = %v", err)
	}
	if confirmation != nil {
		t.Fatalf("foreign confirmation must be ignored, got %#v", confirmation)
	}
	if terminal.Status != types.RunStatusCompleted {
		t.Fatalf("terminal = %#v", terminal)
	}
}

// TestResumeRunMustBelongToSameRole 验证续等只认同一角色的运行：
// 跨角色续等会被拒绝，不让人借续等偷看别人的会话。
func TestResumeRunMustBelongToSameRole(t *testing.T) {
	system := &system{runs: map[string]*runRecord{
		"run-2": {runID: "run-2", state: types.RunState{ID: "run-2", RoleID: "role-b", SessionID: "session-2", Status: types.RunStatusRunning}},
	}}
	if _, err := system.resumeToolMessageRun("run-2", "role-x"); err == nil {
		t.Fatal("resume across roles must be rejected")
	}
	state, err := system.resumeToolMessageRun("run-2", "role-b")
	if err != nil {
		t.Fatalf("resume same role error = %v", err)
	}
	if state.ID != "run-2" {
		t.Fatalf("resumed run = %#v", state)
	}
}

// TestResumeMissingRunFailsLoudly 验证续等找不到运行时不静默返回空结果。
func TestResumeMissingRunFailsLoudly(t *testing.T) {
	system := &system{runs: map[string]*runRecord{}}
	if _, err := system.resumeToolMessageRun("run-gone", "role-b"); err == nil {
		t.Fatal("missing run must fail loudly")
	}
}

// TestFailedToolMessageKeepsSessionIdentity 钉住「会话已建立就不许丢身份」：
// 运行失败时必须把会话标识与运行标识原样带回，原因另放一处，
// 让调用方在任何结局下都能回到那个会话。
func TestFailedToolMessageKeepsSessionIdentity(t *testing.T) {
	result := failedToolMessage(types.RunState{ID: "run-1", SessionID: "session-1"}, "role-b", "供应商连接断开")
	if result.Status != types.ToolMessageStatusFailed {
		t.Fatalf("status = %q, want %q", result.Status, types.ToolMessageStatusFailed)
	}
	if result.SessionID != "session-1" {
		t.Fatalf("failed result must keep the session identity, got %#v", result)
	}
	if result.RunID != "run-1" {
		t.Fatalf("failed result must keep the run identity, got %#v", result)
	}
	if result.RoleID != "role-b" {
		t.Fatalf("failed result must keep the role identity, got %#v", result)
	}
	if result.Error != "供应商连接断开" {
		t.Fatalf("failure reason = %q", result.Error)
	}
}

// TestFailedToolMessageAlwaysHasReason 验证失败结果不会交出空白原因：
// 没有原因时给一句可读的落定说明，界面与模型都不必猜。
func TestFailedToolMessageAlwaysHasReason(t *testing.T) {
	result := failedToolMessage(types.RunState{ID: "run-1", SessionID: "session-1"}, "role-b", "   ")
	if strings.TrimSpace(result.Error) == "" {
		t.Fatalf("failed result must carry a reason, got %#v", result)
	}
}
