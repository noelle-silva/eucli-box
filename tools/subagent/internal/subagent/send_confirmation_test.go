package subagent

import (
	"context"
	"strings"
	"testing"

	"eucli-box/tools/subagent/internal/toolcontrol"
	"eucli-box/tools/subagent/internal/types"
)

// scriptedSession 按脚本回应两类能力：发送消息按序取结果，转呈确认固定一个结论。
type scriptedSession struct {
	messageResults     []toolcontrol.CapabilityResult
	messageIndex       int
	messageCalls       []types.ToolMessageRequest
	confirmationResult toolcontrol.CapabilityResult
	confirmationCalls  []types.ToolConfirmationFact
}

func (s *scriptedSession) Request(_ context.Context, capability string, access string, payload any) (toolcontrol.CapabilityResult, error) {
	switch capability {
	case types.ToolCapabilitySessionMessage:
		s.messageCalls = append(s.messageCalls, payload.(types.ToolMessageRequest))
		if s.messageIndex >= len(s.messageResults) {
			return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusFailed, Error: "脚本里没有更多发送结果"}, nil
		}
		result := s.messageResults[s.messageIndex]
		s.messageIndex++
		return result, nil
	case types.ToolCapabilityToolConfirmation:
		s.confirmationCalls = append(s.confirmationCalls, payload.(types.ToolConfirmationFact))
		return s.confirmationResult, nil
	default:
		return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusFailed, Error: "unexpected capability"}, nil
	}
}

func pendingMessageResult() toolcontrol.CapabilityResult {
	return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolMessageResult{
		Status:    types.ToolMessageStatusConfirmationRequired,
		RunID:     "run-2",
		SessionID: "session-2",
		RoleID:    "role-b",
		Confirmation: &types.ToolConfirmationFact{
			DecisionID: "decision-1",
			ToolName:   "file_editor",
			Reason:     "路径越出工作区围栏",
			Arguments:  map[string]any{"path": "E:/outside/a.txt"},
			SessionID:  "session-2",
			RoleID:     "role-b",
		},
	}}
}

func repliedMessageResult(content string) toolcontrol.CapabilityResult {
	return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolMessageResult{
		Status:    types.ToolMessageStatusReplied,
		RunID:     "run-2",
		SessionID: "session-2",
		MessageID: "message-9",
		RoleID:    "role-b",
		Message:   content,
	}}
}

func approvedConfirmation() toolcontrol.CapabilityResult {
	return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolConfirmationOutcome{Status: types.ToolConfirmationStatusApproved, DecisionID: "decision-1"}}
}

// TestSendRelaysConfirmationThenResumes 钉住极简适配的完整闭环：
// 拿到待确认就抛给宿主，裁决后接着等同一条运行，最后交出答复。
func TestSendRelaysConfirmationThenResumes(t *testing.T) {
	session := &scriptedSession{
		messageResults:     []toolcontrol.CapabilityResult{pendingMessageResult(), repliedMessageResult("文件写完了")},
		confirmationResult: approvedConfirmation(),
	}
	output := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"roleId": "role-b", "message": "去写文件"}}, session)

	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("send result = %#v", output)
	}
	if !strings.Contains(output.Content, "文件写完了") {
		t.Fatalf("content must carry the reply, got %q", output.Content)
	}
	if len(session.confirmationCalls) != 1 || session.confirmationCalls[0].DecisionID != "decision-1" {
		t.Fatalf("confirmation relayed = %#v", session.confirmationCalls)
	}
	if len(session.messageCalls) != 2 {
		t.Fatalf("message calls = %#v", session.messageCalls)
	}
	resume := session.messageCalls[1]
	if resume.ResumeRunID != "run-2" {
		t.Fatalf("resume must wait for the same run, got %#v", resume)
	}
	if strings.TrimSpace(resume.Message) != "" {
		t.Fatalf("resume must not send a new message, got %q", resume.Message)
	}
}

// TestSendResumesAfterRejection 验证拒绝同样续等：目标角色会收尾并给出说明。
func TestSendResumesAfterRejection(t *testing.T) {
	session := &scriptedSession{
		messageResults: []toolcontrol.CapabilityResult{pendingMessageResult(), repliedMessageResult("我停手了，没有写那个文件")},
		confirmationResult: toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolConfirmationOutcome{
			Status: types.ToolConfirmationStatusRejected, DecisionID: "decision-1",
		}},
	}
	output := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"roleId": "role-b", "message": "去写文件"}}, session)
	if output.Status != types.ToolStatusSuccess || !strings.Contains(output.Content, "停手") {
		t.Fatalf("rejected branch result = %#v", output)
	}
}

// TestSendFailsWhenConfirmationExpired 验证审批单失效时明确失败，不空等。
func TestSendFailsWhenConfirmationExpired(t *testing.T) {
	session := &scriptedSession{
		messageResults: []toolcontrol.CapabilityResult{pendingMessageResult()},
		confirmationResult: toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolConfirmationOutcome{
			Status: types.ToolConfirmationStatusExpired, DecisionID: "decision-1",
		}},
	}
	output := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"roleId": "role-b", "message": "去写文件"}}, session)
	if output.Status != types.ToolStatusFailed || !strings.Contains(output.Error, "已失效") {
		t.Fatalf("expired branch result = %#v", output)
	}
}

// TestSendFailsWhenPendingFactsMissing 验证宿主报了待确认却没给审批单时当场失败。
func TestSendFailsWhenPendingFactsMissing(t *testing.T) {
	session := &scriptedSession{messageResults: []toolcontrol.CapabilityResult{
		{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolMessageResult{Status: types.ToolMessageStatusConfirmationRequired, RunID: "run-2", SessionID: "session-2"}},
	}}
	output := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"roleId": "role-b", "message": "去写文件"}}, session)
	if output.Status != types.ToolStatusFailed || !strings.Contains(output.Error, "审批单") {
		t.Fatalf("missing facts result = %#v", output)
	}
}

// TestSendReportsApplyFailureAsIs 验证裁决应用失败时把真实原因带回，
// 不与「审批单失效」混为一谈。
func TestSendReportsApplyFailureAsIs(t *testing.T) {
	session := &scriptedSession{
		messageResults: []toolcontrol.CapabilityResult{pendingMessageResult()},
		confirmationResult: toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.ToolConfirmationOutcome{
			Status: types.ToolConfirmationStatusFailed, DecisionID: "decision-1", Reason: "工作区围栏注册目录不可用",
		}},
	}
	output := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"roleId": "role-b", "message": "去写文件"}}, session)
	if output.Status != types.ToolStatusFailed || !strings.Contains(output.Error, "工作区围栏注册目录不可用") {
		t.Fatalf("apply failure result = %#v", output)
	}
}
