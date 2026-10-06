package agentruntime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

// TestConcurrentAsyncResultFlushInjectsOnce 钉住 B6 约束 1/2：
// 同一异步任务的结果最多回灌一次，两条运行并发回灌不得产生第二条结果消息。
func TestConcurrentAsyncResultFlushInjectsOnce(t *testing.T) {
	fakes := newRuntimeFakes()
	runtime := newTestRuntime(t, fakes, Config{}).(*system)

	now := time.Now().UTC()
	task := types.AsyncToolTask{
		ID: "async-1", RoleID: "developer", SessionID: "session-1", ToolName: "file-reader",
		Status: types.AsyncToolTaskStatusSucceeded, SubmittedAt: now, FinishedAt: now,
		Result: &types.ToolResult{ID: "result-1", ToolName: "file-reader", Status: types.ToolStatusSuccess, Content: "ready", CreatedAt: now},
	}
	fakes.storage.mu.Lock()
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID: "session-1", RoleID: "developer", Status: string(types.RunStatusRunning),
		CreatedAt: now, UpdatedAt: now, LastActive: now,
		Messages:       []types.Message{{ID: "u1", Type: "user", Content: "before", BranchID: defaultRuntimeBranchID, CreatedAt: now, UpdatedAt: now}},
		AsyncToolTasks: []types.AsyncToolTask{task},
	}
	fakes.storage.mu.Unlock()
	runtime.upsertRuntimeAsyncToolTask(task)

	records := []*runRecord{newAsyncFlushRecord(runtime, "run-a"), newAsyncFlushRecord(runtime, "run-b")}
	flushed := make([]bool, len(records))
	errs := make([]error, len(records))
	var wg sync.WaitGroup
	for index := range records {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			flushed[index], errs[index] = runtime.flushReadyAsyncToolResults(context.Background(), records[index], nil, types.RunStatusRunning, false)
		}(index)
	}
	wg.Wait()

	for index := range errs {
		if errs[index] != nil {
			t.Fatalf("flush[%d] error = %v", index, errs[index])
		}
	}
	winners := 0
	for _, didFlush := range flushed {
		if didFlush {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("flush winners = %d, want 1", winners)
	}

	fakes.storage.mu.Lock()
	stored := fakes.storage.sessions["developer/session-1"]
	fakes.storage.mu.Unlock()

	resultMessages := 0
	for _, message := range stored.Messages {
		if message.Type != types.MessageTypeAsyncToolResult {
			continue
		}
		resultMessages++
		if message.AsyncToolTaskID != "async-1" {
			t.Fatalf("result message task binding = %q, want async-1", message.AsyncToolTaskID)
		}
	}
	if resultMessages != 1 {
		t.Fatalf("async result messages = %d, want 1; messages = %#v", resultMessages, stored.Messages)
	}
	if len(stored.AsyncToolTasks) != 1 || stored.AsyncToolTasks[0].Status != types.AsyncToolTaskStatusCompleted || stored.AsyncToolTasks[0].CompletedAt.IsZero() {
		t.Fatalf("stored task = %#v", stored.AsyncToolTasks)
	}
	if _, ok := runtime.asyncTasks["async-1"]; ok {
		t.Fatalf("injected task must be reclaimed from runtime registry")
	}
}

// TestStaleRuntimeTaskDoesNotOverwritePersistedCompletion 钉住 B6 约束 3：
// 运行期表不得覆盖落盘会话中已完成的标记，陈旧运行期副本不得触发二次回灌。
func TestStaleRuntimeTaskDoesNotOverwritePersistedCompletion(t *testing.T) {
	fakes := newRuntimeFakes()
	runtime := newTestRuntime(t, fakes, Config{}).(*system)

	now := time.Now().UTC()
	completed := types.AsyncToolTask{ID: "async-1", RoleID: "developer", SessionID: "session-1", ToolName: "file-reader", Status: types.AsyncToolTaskStatusCompleted, CompletedAt: now, SubmittedAt: now}
	stale := types.AsyncToolTask{ID: "async-1", RoleID: "developer", SessionID: "session-1", ToolName: "file-reader", Status: types.AsyncToolTaskStatusSucceeded, SubmittedAt: now, FinishedAt: now}
	fakes.storage.mu.Lock()
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID: "session-1", RoleID: "developer", Status: string(types.RunStatusRunning),
		CreatedAt: now, UpdatedAt: now, LastActive: now, AsyncToolTasks: []types.AsyncToolTask{completed},
	}
	fakes.storage.mu.Unlock()
	runtime.upsertRuntimeAsyncToolTask(stale)

	record := newAsyncFlushRecord(runtime, "run-x")
	flushed, err := runtime.flushReadyAsyncToolResults(context.Background(), record, nil, types.RunStatusRunning, false)
	if err != nil {
		t.Fatalf("flush error = %v", err)
	}
	if flushed {
		t.Fatalf("stale runtime task must not trigger a second injection")
	}

	fakes.storage.mu.Lock()
	stored := fakes.storage.sessions["developer/session-1"]
	fakes.storage.mu.Unlock()
	if len(stored.AsyncToolTasks) != 1 || stored.AsyncToolTasks[0].CompletedAt.IsZero() {
		t.Fatalf("persisted completion marker was overwritten: %#v", stored.AsyncToolTasks)
	}
}

// TestAsyncRecoverySkipsAlreadyInjectedMessage 钉住 B6 约束 5：
// 结果消息按任务 ID 与任务绑定，作为幂等的第二道防线：消息已在则只补完成标记，不重复回灌。
func TestAsyncRecoverySkipsAlreadyInjectedMessage(t *testing.T) {
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID: "session-1", RoleID: "developer", Status: string(types.RunStatusCompleted),
		CreatedAt: now, UpdatedAt: now, LastActive: now,
		Messages:       []types.Message{{ID: "m1", Type: types.MessageTypeAsyncToolResult, Content: "existing", AsyncToolTaskID: "async-1", BranchID: defaultRuntimeBranchID, CreatedAt: now, UpdatedAt: now}},
		AsyncToolTasks: []types.AsyncToolTask{{ID: "async-1", RoleID: "developer", SessionID: "session-1", ToolName: "file-reader", Status: types.AsyncToolTaskStatusSucceeded, SubmittedAt: now, FinishedAt: now}},
	}
	_ = newTestRuntime(t, fakes, Config{})

	fakes.storage.mu.Lock()
	stored := fakes.storage.sessions["developer/session-1"]
	fakes.storage.mu.Unlock()

	resultMessages := 0
	for _, message := range stored.Messages {
		if message.Type == types.MessageTypeAsyncToolResult {
			resultMessages++
		}
	}
	if resultMessages != 1 {
		t.Fatalf("async result messages = %d, want 1; messages = %#v", resultMessages, stored.Messages)
	}
	if len(stored.AsyncToolTasks) != 1 || stored.AsyncToolTasks[0].CompletedAt.IsZero() {
		t.Fatalf("task must be marked completed on recovery: %#v", stored.AsyncToolTasks)
	}
}

// TestFailedAsyncFlushDoesNotPersistPartialResult 钉住 B6 约束 4：
// 落盘失败必须回退认领并撤销未落定的结果消息，绝不把半成品留给后续收口落盘。
func TestFailedAsyncFlushDoesNotPersistPartialResult(t *testing.T) {
	fakes := newRuntimeFakes()
	runtime := newTestRuntime(t, fakes, Config{}).(*system)

	now := time.Now().UTC()
	task := types.AsyncToolTask{
		ID: "async-1", RoleID: "developer", SessionID: "session-1", ToolName: "file-reader",
		Status: types.AsyncToolTaskStatusSucceeded, SubmittedAt: now, FinishedAt: now,
		Result: &types.ToolResult{ID: "result-1", ToolName: "file-reader", Status: types.ToolStatusSuccess, Content: "ready", CreatedAt: now},
	}
	fakes.storage.mu.Lock()
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID: "session-1", RoleID: "developer", Status: string(types.RunStatusRunning),
		CreatedAt: now, UpdatedAt: now, LastActive: now,
		Messages:       []types.Message{{ID: "u1", Type: "user", Content: "before", BranchID: defaultRuntimeBranchID, CreatedAt: now, UpdatedAt: now}},
		AsyncToolTasks: []types.AsyncToolTask{task},
	}
	fakes.storage.saveMessagesErr = errors.New("disk full")
	fakes.storage.mu.Unlock()
	runtime.upsertRuntimeAsyncToolTask(task)

	record := newAsyncFlushRecord(runtime, "run-a")
	flushed, err := runtime.flushReadyAsyncToolResults(context.Background(), record, nil, types.RunStatusRunning, false)
	if err == nil || flushed {
		t.Fatalf("flush must fail, flushed=%v err=%v", flushed, err)
	}
	if _, ok := runtime.asyncTasks["async-1"]; !ok {
		t.Fatalf("runtime registry must be retained when persist fails")
	}

	fakes.storage.mu.Lock()
	stored := fakes.storage.sessions["developer/session-1"]
	fakes.storage.mu.Unlock()
	for _, message := range stored.Messages {
		if message.Type == types.MessageTypeAsyncToolResult {
			t.Fatalf("partial async result must not be persisted: %#v", stored.Messages)
		}
	}
	if len(stored.AsyncToolTasks) != 1 || !stored.AsyncToolTasks[0].InjectionClaimedAt.IsZero() {
		t.Fatalf("claim must be rolled back: %#v", stored.AsyncToolTasks)
	}
}

func newAsyncFlushRecord(runtime *system, runID string) *runRecord {
	now := time.Now().UTC()
	parent := types.Message{ID: "u1", Type: "user", Content: "before", BranchID: defaultRuntimeBranchID, CreatedAt: now, UpdatedAt: now}
	record := &runRecord{
		runID:         runID,
		roleID:        "developer",
		state:         types.RunState{ID: runID, RoleID: "developer", SessionID: "session-1", Status: types.RunStatusRunning},
		session:       types.Session{ID: "session-1", RoleID: "developer", Messages: []types.Message{parent}},
		messageParent: parent,
	}
	runtime.mu.Lock()
	runtime.runs[runID] = record
	runtime.mu.Unlock()
	return record
}
