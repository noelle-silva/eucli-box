package agentruntime

import (
	"context"
	"strings"
	"testing"
	"time"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

// TestRunOriginDerivation 钉住 B7 约束 2：四种输入形态收敛为同一分类来源，
// 显式声明的起点种类优先，缺省时按既有输入字段回推。
func TestRunOriginDerivation(t *testing.T) {
	cases := []struct {
		name    string
		request types.RunRequest
		want    types.RunOrigin
	}{
		{"append message", types.RunRequest{Message: "hi"}, types.RunOriginMessage},
		{"user message", types.RunRequest{UserMessageID: "u1"}, types.RunOriginUserMessage},
		{"context message", types.RunRequest{ContextMessageID: "a1"}, types.RunOriginContextMessage},
		{"explicit session tail", types.RunRequest{Origin: types.RunOriginSessionTail}, types.RunOriginSessionTail},
	}
	for _, tc := range cases {
		if got := runOriginFromRequest(tc.request); got != tc.want {
			t.Fatalf("%s: origin = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestAsyncContinuationDeclaresSessionTailOrigin 钉住 B7 约束 3：
// 异步续跑只声明「从会话末条」起点，不自行读会话、不预计算锚点消息 ID。
func TestAsyncContinuationDeclaresSessionTailOrigin(t *testing.T) {
	task := types.AsyncToolTask{RoleID: "developer", SessionID: "session-1", Continuation: types.RunContinuation{}}
	key := asyncContinuationKey{roleID: "developer", sessionID: "session-1"}
	request := runRequestFromAsyncToolContinuation(task, key)
	if request.Origin != types.RunOriginSessionTail {
		t.Fatalf("continuation origin = %q, want %q", request.Origin, types.RunOriginSessionTail)
	}
	if strings.TrimSpace(request.ContextMessageID) != "" {
		t.Fatalf("continuation must not precompute anchor, contextMessageId = %q", request.ContextMessageID)
	}
	if strings.TrimSpace(request.UserMessageID) != "" {
		t.Fatalf("continuation must not precompute user message, userMessageId = %q", request.UserMessageID)
	}
}

// TestSessionTailResetReloadsAndResolvesCurrentTail 钉住 B7 约束 1/4：
// 遭遇并发追加后重新载入会话、重解析起点，锚点取当前活动分支最新且不分叉。
func TestSessionTailResetReloadsAndResolvesCurrentTail(t *testing.T) {
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID: "session-1", RoleID: "developer", Status: string(types.RunStatusRunning),
		CreatedAt: now, UpdatedAt: now, LastActive: now,
		Messages: []types.Message{
			{ID: "u1", Type: "user", Content: "q", BranchID: "main", CreatedAt: now, UpdatedAt: now},
			{ID: "a1", Type: "assistant", Content: "a", ParentMessageID: "u1", BranchID: "main", CreatedAt: now, UpdatedAt: now},
		},
	}
	runtime := newTestRuntime(t, fakes, Config{}).(*system)
	record := &runRecord{
		runID: "run-1", roleID: "developer", origin: types.RunOriginSessionTail,
		state:         types.RunState{ID: "run-1", RoleID: "developer", SessionID: "session-1", Status: types.RunStatusRunning},
		session:       types.Session{ID: "session-1", RoleID: "developer"},
		messageParent: types.Message{ID: "a1"},
	}
	runtime.mu.Lock()
	runtime.runs[record.runID] = record
	runtime.mu.Unlock()

	// 并发追加一条新消息，模拟另一条运行抢先落定。
	fakes.storage.mu.Lock()
	session := fakes.storage.sessions["developer/session-1"]
	session.Messages = append(session.Messages, types.Message{ID: "u2", Type: "user", Content: "follow up", ParentMessageID: "a1", BranchID: "main", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)})
	fakes.storage.sessions["developer/session-1"] = session
	fakes.storage.mu.Unlock()

	if _, err := runtime.resetSessionTailRun(context.Background(), record); err != nil {
		t.Fatalf("resetSessionTailRun() error = %v", err)
	}
	if record.messageParent.ID != "u2" {
		t.Fatalf("re-resolved anchor = %q, want u2", record.messageParent.ID)
	}
	if record.anchorMessageID != "u2" {
		t.Fatalf("anchorMessageID = %q, want u2", record.anchorMessageID)
	}
	if record.forceBranchReply {
		t.Fatalf("session tail run must not fork")
	}
	if !record.forceNewAssistantReply {
		t.Fatalf("session tail run must start a new assistant reply")
	}
}

// TestSessionTailRunRetriesAfterConcurrentAppend 钉住 B7 约束 4：
// 从会话末条的回复写在与锚点同分支的槽位，被并发追加占用即触发冲突，
// 捕获后重新加载、重解析起点并重试，最终接在最新事实上、不产生意外分支。
func TestSessionTailRunRetriesAfterConcurrentAppend(t *testing.T) {
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	task := types.AsyncToolTask{
		ID: "async-1", RoleID: "developer", SessionID: "session-1", ToolName: "file-reader",
		Status: types.AsyncToolTaskStatusSucceeded, SubmittedAt: now, FinishedAt: now,
		Result: &types.ToolResult{ID: "result-1", ToolName: "file-reader", Status: types.ToolStatusSuccess, Content: "tool ok", CreatedAt: now},
	}
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "final after async"},
		{ID: "m2", Content: "final after async"},
	}
	runtime := newTestRuntime(t, fakes, Config{}).(*system)
	// 会话在启动恢复之后登记：避免启动恢复把就绪任务先行回灌，确保回灌发生在运行主干上。
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID: "session-1", RoleID: "developer", Status: string(types.RunStatusRunning),
		CreatedAt: now, UpdatedAt: now, LastActive: now,
		Messages: []types.Message{
			{ID: "u1", Type: "user", Content: "use async tool", BranchID: "main", CreatedAt: now, UpdatedAt: now},
			{ID: "a1", Type: "assistant", Content: "", ParentMessageID: "u1", BranchID: "main", CreatedAt: now, UpdatedAt: now},
		},
		AsyncToolTasks: []types.AsyncToolTask{task},
	}
	runtime.upsertRuntimeAsyncToolTask(task)

	// 首次带写落盘时注入一条并发追加并制造分支槽冲突；后续落盘恢复正常。
	injected := false
	fakes.storage.onSaveMessages = func(save types.SessionMessageSave) {
		if injected {
			fakes.storage.saveMessagesErr = nil
			return
		}
		if len(save.Writes) == 0 {
			return
		}
		injected = true
		key := fakes.storage.sessionKey(save.Session)
		session := fakes.storage.sessions[key]
		session.Messages = append(session.Messages, types.Message{ID: "u2", Type: "user", Content: "concurrent follow up", ParentMessageID: "a1", BranchID: "main", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)})
		fakes.storage.sessions[key] = session
		fakes.storage.saveMessagesErr = apperrors.New("fake-storage", "storage.conflict", "message branch slot is already occupied")
	}

	state, err := runtime.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", Origin: types.RunOriginSessionTail, Stream: types.BoolPtr(false)})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, runtime, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}

	fakes.storage.mu.Lock()
	session := fakes.storage.sessions["developer/session-1"]
	fakes.storage.mu.Unlock()

	var asyncResult, finalReply types.Message
	for _, message := range session.Messages {
		if message.Type == types.MessageTypeAsyncToolResult && message.AsyncToolTaskID == "async-1" {
			asyncResult = message
		}
		if message.Type == "assistant" && message.Content == "final after async" {
			finalReply = message
		}
	}
	if asyncResult.ID == "" {
		t.Fatalf("async result was not injected: %#v", session.Messages)
	}
	if asyncResult.ParentMessageID != "u2" {
		t.Fatalf("async result parent = %q, want re-resolved tail u2; messages = %#v", asyncResult.ParentMessageID, session.Messages)
	}
	if finalReply.ID == "" {
		t.Fatalf("final reply was not persisted: %#v", session.Messages)
	}
	if finalReply.ParentMessageID != asyncResult.ID {
		t.Fatalf("final reply parent = %q, want async result %q", finalReply.ParentMessageID, asyncResult.ID)
	}
}

// TestSessionTailReplyConflictReloadsAndRetries 钉住 B7 约束 4：
// 异步结果已落盘后，从会话末条的助手回复若被并发追加占用分支槽，
// 同样重新加载、重解析起点并重试，最终接在最新事实上。
func TestSessionTailReplyConflictReloadsAndRetries(t *testing.T) {
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	task := types.AsyncToolTask{
		ID: "async-1", RoleID: "developer", SessionID: "session-1", ToolName: "file-reader",
		Status: types.AsyncToolTaskStatusSucceeded, SubmittedAt: now, FinishedAt: now,
		Result: &types.ToolResult{ID: "result-1", ToolName: "file-reader", Status: types.ToolStatusSuccess, Content: "tool ok", CreatedAt: now},
	}
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "first reply"},
		{ID: "m2", Content: "second reply"},
	}
	runtime := newTestRuntime(t, fakes, Config{}).(*system)
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID: "session-1", RoleID: "developer", Status: string(types.RunStatusRunning),
		CreatedAt: now, UpdatedAt: now, LastActive: now,
		Messages: []types.Message{
			{ID: "u1", Type: "user", Content: "use async tool", BranchID: "main", CreatedAt: now, UpdatedAt: now},
			{ID: "a1", Type: "assistant", Content: "", ParentMessageID: "u1", BranchID: "main", CreatedAt: now, UpdatedAt: now},
		},
		AsyncToolTasks: []types.AsyncToolTask{task},
	}
	runtime.upsertRuntimeAsyncToolTask(task)

	// 助手回复落盘时注入一条并发追加并制造分支槽冲突；后续落盘恢复正常。
	injected := false
	fakes.storage.onSaveMessages = func(save types.SessionMessageSave) {
		if injected {
			fakes.storage.saveMessagesErr = nil
			return
		}
		hasAssistant := false
		for _, write := range save.Writes {
			if write.Message.Type == "assistant" {
				hasAssistant = true
			}
		}
		if !hasAssistant {
			return
		}
		injected = true
		key := fakes.storage.sessionKey(save.Session)
		session := fakes.storage.sessions[key]
		parent := session.Messages[len(session.Messages)-1]
		session.Messages = append(session.Messages, types.Message{ID: "u2", Type: "user", Content: "concurrent follow up", ParentMessageID: parent.ID, BranchID: parent.BranchID, CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)})
		fakes.storage.sessions[key] = session
		fakes.storage.saveMessagesErr = apperrors.New("fake-storage", "storage.conflict", "message branch slot is already occupied")
	}

	state, err := runtime.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", Origin: types.RunOriginSessionTail, Stream: types.BoolPtr(false)})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, runtime, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}

	fakes.storage.mu.Lock()
	session := fakes.storage.sessions["developer/session-1"]
	fakes.storage.mu.Unlock()

	var finalReply types.Message
	for _, message := range session.Messages {
		if message.Type == "assistant" && message.Content == "second reply" {
			finalReply = message
		}
	}
	if finalReply.ID == "" {
		t.Fatalf("retried reply was not persisted: %#v", session.Messages)
	}
	if finalReply.ParentMessageID != "u2" {
		t.Fatalf("final reply parent = %q, want re-resolved tail u2; messages = %#v", finalReply.ParentMessageID, session.Messages)
	}
}

// TestSessionTailRunRejectsMessageInput 钉住 B7 约束 2：
// 「从会话末条」起点只声明会话，不携带任何消息输入。
func TestSessionTailRunRejectsMessageInput(t *testing.T) {
	runtime := newTestRuntime(t, newRuntimeFakes(), Config{}).(*system)
	_, err := runtime.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", Message: "hi", Origin: types.RunOriginSessionTail})
	if err == nil {
		t.Fatalf("session tail run with message input must be rejected")
	}
}
