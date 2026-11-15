package subagent

import (
	"errors"
	"strings"
	"testing"

	"eucli-box/tools/subagent/internal/toolcontrol"
	"eucli-box/tools/subagent/internal/types"
)

// acceptedResult 是一份受理回执：会话身份已经落定，角色还没给答复。
func acceptedResult(sessionID string, runID string) toolcontrol.CapabilityResult {
	return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolMessageResult{
		Status:    types.ToolMessageStatusAccepted,
		RunID:     runID,
		SessionID: sessionID,
		RoleID:    "role-b",
	}}
}

// repliedResult 是一份落定答复：角色停下后留下了一条消息。
func repliedResult(sessionID string, runID string) toolcontrol.CapabilityResult {
	return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolMessageResult{
		Status:    types.ToolMessageStatusReplied,
		RunID:     runID,
		SessionID: sessionID,
		MessageID: "message-2",
		RoleID:    "role-b",
		Message:   "收到",
	}}
}

// TestSendReportsSessionIdentityOnArrival 钉住「会话一建立就亮身份」：
// 受理回执到达的那一刻，实时通道上就该出现会话身份，不等角色把话说完。
func TestSendReportsSessionIdentityOnArrival(t *testing.T) {
	session := &scriptedSession{messageResults: []toolcontrol.CapabilityResult{
		acceptedResult("session-9", "run-1"),
		repliedResult("session-9", "run-1"),
	}}

	if output := runWith(session, map[string]any{"roleId": "role-b", "message": "在吗"}); output.Status != types.ToolStatusSuccess {
		t.Fatalf("send result = %#v", output)
	}
	if len(session.reports) == 0 {
		t.Fatal("established session must be reported on the live channel at once")
	}
	if session.reports[0] != "会话 session-9" {
		t.Fatalf("first live report = %q, want the session identity", session.reports[0])
	}
}

// TestSendResumesAfterReceipt 钉住两趟交付：受理之后不再重发消息，
// 只凭回执里的运行标识接着等同一条运行。
func TestSendResumesAfterReceipt(t *testing.T) {
	session := &scriptedSession{messageResults: []toolcontrol.CapabilityResult{
		acceptedResult("session-9", "run-1"),
		repliedResult("session-9", "run-1"),
	}}

	runWith(session, map[string]any{"roleId": "role-b", "message": "在吗"})
	if len(session.messageCalls) != 2 {
		t.Fatalf("send must take two trips, got %d", len(session.messageCalls))
	}
	first, second := session.messageCalls[0], session.messageCalls[1]
	if first.ResumeRunID != "" || first.Message != "在吗" {
		t.Fatalf("first trip must send the message, got %#v", first)
	}
	if second.ResumeRunID != "run-1" {
		t.Fatalf("second trip must resume the accepted run, got %#v", second)
	}
	if second.Message != "" {
		t.Fatalf("second trip must not send the message again, got %#v", second)
	}
}

// TestSendPutsSessionIdentityFirst 钉住正文排版：会话身份永远在第一行，
// 后面才是角色答复，让人一眼就能拿走身份。
func TestSendPutsSessionIdentityFirst(t *testing.T) {
	session := &scriptedSession{messageResults: []toolcontrol.CapabilityResult{
		acceptedResult("session-9", "run-1"),
		repliedResult("session-9", "run-1"),
	}}

	output := runWith(session, map[string]any{"roleId": "role-b", "message": "在吗"})
	lines := strings.Split(output.Content, "\n")
	if len(lines) == 0 || lines[0] != "会话 session-9" {
		t.Fatalf("first line must be the session identity, got %q", output.Content)
	}
	if !strings.Contains(output.Content, "收到") {
		t.Fatalf("content must carry the reply, got %q", output.Content)
	}
	if output.Metadata["sessionId"] != "session-9" {
		t.Fatalf("metadata must carry the session identity, got %#v", output.Metadata)
	}
}

// TestSendKeepsSessionIdentityWhenRunFails 钉住失败语义：会话已经建立，
// 运行后来失败也要把会话身份放在第一行，原因跟在后面。
func TestSendKeepsSessionIdentityWhenRunFails(t *testing.T) {
	session := &scriptedSession{messageResults: []toolcontrol.CapabilityResult{
		acceptedResult("session-9", "run-1"),
		{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolMessageResult{
			Status:    types.ToolMessageStatusFailed,
			RunID:     "run-1",
			SessionID: "session-9",
			RoleID:    "role-b",
			Error:     "供应商连接断开",
		}},
	}}

	output := runWith(session, map[string]any{"roleId": "role-b", "message": "在吗"})
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("failed run must surface as failure, got %#v", output)
	}
	lines := strings.Split(output.Content, "\n")
	if len(lines) == 0 || lines[0] != "会话 session-9" {
		t.Fatalf("failed content must start with the session identity, got %q", output.Content)
	}
	if !strings.Contains(output.Content, "供应商连接断开") {
		t.Fatalf("failed content must carry the reason, got %q", output.Content)
	}
	if output.Metadata["sessionId"] != "session-9" {
		t.Fatalf("metadata must carry the session identity, got %#v", output.Metadata)
	}
}

// TestSendKeepsSessionIdentityWhenResumeBreaks 钉住通道中断的语义：
// 受理之后通道断了，会话身份照样交回去，不因为「结果没取到」就把身份丢掉。
func TestSendKeepsSessionIdentityWhenResumeBreaks(t *testing.T) {
	session := &scriptedSession{
		messageResults: []toolcontrol.CapabilityResult{acceptedResult("session-9", "run-1")},
		messageErr:     []error{nil, errors.New("tool control client is closed")},
	}

	output := runWith(session, map[string]any{"roleId": "role-b", "message": "在吗"})
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("broken channel must surface as failure, got %#v", output)
	}
	if !strings.HasPrefix(output.Content, "会话 session-9") {
		t.Fatalf("failed content must start with the session identity, got %q", output.Content)
	}
}

// TestSendSurfacesReportFailure 钉住上报失败的可见性：实时上报是尽力而为，
// 它失败不打断这次发送，但原因必须留在结果里，不能被吞掉。
func TestSendSurfacesReportFailure(t *testing.T) {
	session := &fakeSession{
		reportRecorder: reportRecorder{err: errors.New("通道已关闭")},
		result: toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolMessageResult{
			Status:    types.ToolMessageStatusReplied,
			RunID:     "run-1",
			SessionID: "session-9",
			MessageID: "message-2",
			RoleID:    "role-b",
			Message:   "收到",
		}},
	}

	output := runWith(session, map[string]any{"roleId": "role-b", "message": "在吗"})
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("report failure must not break the send, got %#v", output)
	}
	if output.Metadata["reportError"] != "通道已关闭" {
		t.Fatalf("report failure must be recorded, got %#v", output.Metadata)
	}
}

// TestSendWithoutSessionIdentityFailsLocally 钉住前置条件：宿主没交回会话身份，
// 说明消息根本没进会话，此时不该伪造一个身份出来。
func TestSendWithoutSessionIdentityFailsLocally(t *testing.T) {
	session := &fakeSession{result: toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolMessageResult{
		Status: types.ToolMessageStatusAccepted,
		RunID:  "run-1",
		RoleID: "role-b",
	}}}

	output := runWith(session, map[string]any{"roleId": "role-b", "message": "在吗"})
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("missing session identity must fail loudly, got %#v", output)
	}
	if strings.Contains(output.Content, "会话 ") {
		t.Fatalf("no session identity may be fabricated, got %q", output.Content)
	}
}

// TestSessionHeaderIsTheOnlyPlaceIdentityIsWritten 钉住身份那一行的措辞：
// 正文与实时通道共用同一份写法，两处不会漂移。
func TestSessionHeaderIsTheOnlyPlaceIdentityIsWritten(t *testing.T) {
	if got := sessionHeader("  session-9  "); got != "会话 session-9" {
		t.Fatalf("sessionHeader() = %q", got)
	}
	if got := sessionHeader("   "); got != "" {
		t.Fatalf("blank session identity must produce no header, got %q", got)
	}
	if got := appendHeader("", "正文"); got != "正文" {
		t.Fatalf("appendHeader() without header = %q", got)
	}
	if got := appendHeader("会话 session-9", "正文"); got != "会话 session-9\n正文" {
		t.Fatalf("appendHeader() = %q", got)
	}
}
