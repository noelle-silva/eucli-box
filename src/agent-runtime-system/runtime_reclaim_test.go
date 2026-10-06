package agentruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

// TestTerminalRunRetainedThenReclaimed 钉住约束 5：终态运行记录进入有界保留，
// 窗口内仍可短时查询，超界由既有锁内的惰性回收删除。
func TestTerminalRunRetainedThenReclaimed(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "done"}}
	runtime := newTestRuntime(t, fakes, Config{}).(*system)
	state, err := runtime.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Stream: types.BoolPtr(false), Message: "hello"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitRun(t, runtime, state.ID)

	if _, err := runtime.GetRun(context.Background(), state.ID); err != nil {
		t.Fatalf("terminal run should stay queryable during retention window: %v", err)
	}

	runtime.mu.Lock()
	record := runtime.runs[state.ID]
	if record == nil || record.terminalAt.IsZero() {
		runtime.mu.Unlock()
		t.Fatalf("terminal run record missing terminalAt: %#v", record)
	}
	record.terminalAt = time.Now().Add(-terminalRunRetention - time.Second)
	runtime.reclaimTerminalRunsLocked()
	_, stillPresent := runtime.runs[state.ID]
	runtime.mu.Unlock()
	if stillPresent {
		t.Fatalf("expired terminal run was not reclaimed")
	}
	if _, err := runtime.GetRun(context.Background(), state.ID); err == nil {
		t.Fatalf("reclaimed run should not be queryable")
	}
}

// TestPublishedBaselineFollowsRunLifetime 钉住约束 4：消息推送基线归属产生它的运行，
// 运行未收口期间（含等待确认）不得删，收口后才随运行回收。
func TestPublishedBaselineFollowsRunLifetime(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "need tool", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}}}},
		{ID: "m2", Content: "final"},
	}
	fakes.tool.prepareDecision = types.PermissionDecision{ID: "decision-1", ActionID: "intent-1", ToolName: "file-reader", Status: types.PermissionStatusNeedsConfirmation}
	fakes.tool.confirmedDecision = types.PermissionDecision{ID: "decision-1", ActionID: "intent-1", ToolName: "file-reader", Status: types.PermissionStatusAllowed}
	runtime := newTestRuntime(t, fakes, Config{}).(*system)
	state, err := runtime.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Stream: types.BoolPtr(false), Message: "use tool"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitStatus(t, runtime, state.ID, types.RunStatusWaitingConfirmation)

	if !hasPublishedBaseline(runtime, state.ID) {
		t.Fatalf("baseline must be retained while run is not finalized")
	}
	if err := runtime.SubmitToolConfirmation(context.Background(), types.ToolConfirmation{DecisionID: "decision-1", Approved: true}); err != nil {
		t.Fatalf("SubmitToolConfirmation() error = %v", err)
	}
	waitRun(t, runtime, state.ID)
	if hasPublishedBaseline(runtime, state.ID) {
		t.Fatalf("baseline must be released once run is finalized")
	}
}

func hasPublishedBaseline(runtime *system, runID string) bool {
	runtime.publishedMu.Lock()
	defer runtime.publishedMu.Unlock()
	return len(runtime.publishedMessages[runID]) > 0
}

// TestAsyncTaskReclaimedAfterFlushAndStillQueryable 钉住约束 3：
// 结果回灌并入会话且落盘成功后回收运行期登记，权威副本仍可查询。
func TestAsyncTaskReclaimedAfterFlushAndStillQueryable(t *testing.T) {
	runtime, record, task := newFlushFixture(t, false)
	flushed, err := runtime.flushReadyAsyncToolResults(context.Background(), record, nil, types.RunStatusRunning, false)
	if err != nil || !flushed {
		t.Fatalf("flush flushed=%v err=%v", flushed, err)
	}
	if _, ok := runtime.asyncTasks[task.ID]; ok {
		t.Fatalf("completed task must be reclaimed from runtime registry")
	}
	tasks, err := runtime.ListAsyncToolTasks(context.Background(), types.AsyncToolTaskQuery{RoleID: "developer", SessionID: "session-1"})
	if err != nil {
		t.Fatalf("ListAsyncToolTasks() error = %v", err)
	}
	for _, got := range tasks {
		if got.ID == task.ID {
			if got.Status != types.AsyncToolTaskStatusCompleted || got.CompletedAt.IsZero() {
				t.Fatalf("authoritative task = %#v", got)
			}
			return
		}
	}
	t.Fatalf("reclaimed task not queryable from session storage: %#v", tasks)
}

// TestAsyncTaskRetainedWhenFlushPersistFails 钉住约束 3：
// 回灌落盘失败必须保留运行期登记，绝不丢结果。
func TestAsyncTaskRetainedWhenFlushPersistFails(t *testing.T) {
	runtime, record, task := newFlushFixture(t, true)
	flushed, err := runtime.flushReadyAsyncToolResults(context.Background(), record, nil, types.RunStatusRunning, false)
	if err == nil || flushed {
		t.Fatalf("flush must fail when persist fails, flushed=%v err=%v", flushed, err)
	}
	if _, ok := runtime.asyncTasks[task.ID]; !ok {
		t.Fatalf("runtime registry must be retained when persist fails")
	}
}

func newFlushFixture(t *testing.T, failPersist bool) (*system, *runRecord, types.AsyncToolTask) {
	t.Helper()
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	fakes.storage.sessions["developer/session-1"] = types.Session{ID: "session-1", RoleID: "developer", Status: string(types.RunStatusRunning), CreatedAt: now, UpdatedAt: now, LastActive: now, Messages: []types.Message{}}
	if failPersist {
		fakes.storage.saveMessagesErr = errors.New("disk full")
	}
	runtime := newTestRuntime(t, fakes, Config{}).(*system)
	task := types.AsyncToolTask{ID: "async-1", RunID: "run-1", RoleID: "developer", SessionID: "session-1", ToolName: "file-reader", Status: types.AsyncToolTaskStatusSucceeded, Result: &types.ToolResult{ID: "result-1", ToolName: "file-reader", Status: types.ToolStatusSuccess, Content: "ready", CreatedAt: now}}
	runtime.upsertRuntimeAsyncToolTask(task)
	record := &runRecord{runID: "run-1", roleID: "developer", state: types.RunState{ID: "run-1", Status: types.RunStatusRunning}, session: types.Session{ID: "session-1", RoleID: "developer"}}
	runtime.mu.Lock()
	runtime.runs["run-1"] = record
	runtime.mu.Unlock()
	return runtime, record, task
}

// TestAsyncTaskUpdateCarriesOwnIdentity 钉住约束 6：晚到异步事件的身份由事件源自带，
// 不依赖运行记录是否仍在内存。
func TestAsyncTaskUpdateCarriesOwnIdentity(t *testing.T) {
	runtime := newTestRuntime(t, newRuntimeFakes(), Config{}).(*system)
	events, unsubscribe, err := runtime.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	task := types.AsyncToolTask{ID: "async-1", RunID: "run-gone", GroupID: "group-1", WorkspaceID: "ws-1", Status: types.AsyncToolTaskStatusSucceeded}
	runtime.publishAsyncToolTaskUpdate(task)

	select {
	case event := <-events:
		if event.RunID != "run-gone" || event.GroupID != "group-1" || event.WorkspaceID != "ws-1" {
			t.Fatalf("event identity = %#v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("async task update event was not published")
	}
}
