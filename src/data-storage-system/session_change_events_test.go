package datastorage

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func subscribeSessionChanges(t *testing.T, system *system) (<-chan types.SessionChangeEvent, func()) {
	t.Helper()
	events, unsubscribe, err := system.SubscribeSessionChanges(context.Background())
	if err != nil {
		t.Fatalf("SubscribeSessionChanges() error = %v", err)
	}
	return events, unsubscribe
}

func nextSessionChange(t *testing.T, events <-chan types.SessionChangeEvent) types.SessionChangeEvent {
	t.Helper()
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("session change channel closed unexpectedly")
		}
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session change event")
		return types.SessionChangeEvent{}
	}
}

func assertNoSessionChange(t *testing.T, events <-chan types.SessionChangeEvent) {
	t.Helper()
	select {
	case event, ok := <-events:
		if !ok {
			return
		}
		t.Fatalf("unexpected extra session change event: %#v", event)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSessionWritePipelineEmitsExactlyOneEventPerWrite(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	events, unsubscribe := subscribeSessionChanges(t, system)
	defer unsubscribe()
	roleID := "role-events"

	assertEvent := func(changeType types.SessionChangeType, version uint64, sessionID string) {
		t.Helper()
		event := nextSessionChange(t, events)
		if err := event.Validate(); err != nil {
			t.Fatalf("event is not valid: %v (%#v)", err, event)
		}
		if event.ChangeType != changeType {
			t.Fatalf("change type = %s, want %s", event.ChangeType, changeType)
		}
		if event.TargetKind != changeType.TargetKind() {
			t.Fatalf("target kind = %s, want %s", event.TargetKind, changeType.TargetKind())
		}
		if event.Version != version {
			t.Fatalf("event version = %d, want %d", event.Version, version)
		}
		if event.Location != types.RoleSessionLocation(roleID, sessionID) {
			t.Fatalf("event location = %#v", event.Location)
		}
		if event.OccurredAt.IsZero() {
			t.Fatal("event occurrence time is zero")
		}
		assertNoSessionChange(t, events)
	}

	session, err := system.CreateSession(ctx, roleID, "events")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	assertEvent(types.SessionChangeSessionPersisted, 1, session.ID)

	session.Title = "renamed"
	if err := system.SaveSession(ctx, session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	assertEvent(types.SessionChangeSessionPersisted, 2, session.ID)

	now := time.Now().UTC()
	m1 := types.Message{ID: "m1", Type: "user", Content: "one", CreatedAt: now, UpdatedAt: now}
	saveMessages := func(writes ...types.Message) {
		t.Helper()
		items := make([]types.SessionMessageWrite, 0, len(writes))
		for _, message := range writes {
			items = append(items, types.SessionMessageWrite{Message: message})
		}
		save := types.SessionMessageSave{Session: types.Session{ID: session.ID, RoleID: roleID}, Status: types.RunStatusRunning, Writes: items}
		if err := system.SaveSessionMessages(ctx, save); err != nil {
			t.Fatalf("SaveSessionMessages() error = %v", err)
		}
	}
	saveMessages(m1)
	assertEvent(types.SessionChangeMessageAdded, 3, session.ID)

	content := "edited"
	if _, err := system.UpdateSessionMessage(ctx, roleID, session.ID, "m1", types.SessionMessagePatch{Content: &content}); err != nil {
		t.Fatalf("UpdateSessionMessage() error = %v", err)
	}
	assertEvent(types.SessionChangeMessageEdited, 4, session.ID)

	if _, err := system.DeleteSessionMessage(ctx, roleID, session.ID, "m1"); err != nil {
		t.Fatalf("DeleteSessionMessage() error = %v", err)
	}
	assertEvent(types.SessionChangeMessageDeleted, 5, session.ID)

	m2 := types.Message{ID: "m2", Type: "user", Content: "two", ParentMessageID: "m1", CreatedAt: now, UpdatedAt: now}
	m3 := types.Message{ID: "m3", Type: "assistant", Content: "three", ParentMessageID: "m2", CreatedAt: now, UpdatedAt: now}
	saveMessages(m2, m3)
	assertEvent(types.SessionChangeMessageAdded, 6, session.ID)

	if _, err := system.DeleteSessionMessageSubtree(ctx, roleID, session.ID, "m2"); err != nil {
		t.Fatalf("DeleteSessionMessageSubtree() error = %v", err)
	}
	assertEvent(types.SessionChangeMessageSubtreeDeleted, 7, session.ID)

	// 无消息写入的落盘收尾同样是一次写盘：版本加一，事件是整份会话持久化。
	if err := system.SaveSessionMessages(ctx, types.SessionMessageSave{Session: types.Session{ID: session.ID, RoleID: roleID}, Status: types.RunStatusCompleted}); err != nil {
		t.Fatalf("SaveSessionMessages(status) error = %v", err)
	}
	assertEvent(types.SessionChangeSessionPersisted, 8, session.ID)

	loaded, err := system.LoadSession(ctx, roleID, session.ID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if loaded.Version != 8 {
		t.Fatalf("session version = %d, want 8", loaded.Version)
	}
}

func TestSessionChangeEventCarriesEverySessionFormLocation(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	events, unsubscribe := subscribeSessionChanges(t, system)
	defer unsubscribe()

	groupSession, err := system.CreateGroupSession(ctx, "group-events", "group")
	if err != nil {
		t.Fatalf("CreateGroupSession() error = %v", err)
	}
	groupEvent := nextSessionChange(t, events)
	if groupEvent.Location != types.GroupSessionLocation("group-events", groupSession.ID) {
		t.Fatalf("group event location = %#v", groupEvent.Location)
	}
	if groupEvent.Version != 1 {
		t.Fatalf("group event version = %d, want 1", groupEvent.Version)
	}

	workspaceSession, err := system.CreateWorkspaceSession(ctx, "workspace-events", "role-events", "workspace")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}
	workspaceEvent := nextSessionChange(t, events)
	if workspaceEvent.Location != types.WorkspaceSessionLocation("workspace-events", "role-events", workspaceSession.ID) {
		t.Fatalf("workspace event location = %#v", workspaceEvent.Location)
	}
	if workspaceEvent.Version != 1 {
		t.Fatalf("workspace event version = %d, want 1", workspaceEvent.Version)
	}

	roleSession, err := system.CreateSession(ctx, "role-events", "role")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	roleEvent := nextSessionChange(t, events)
	if roleEvent.Location != types.RoleSessionLocation("role-events", roleSession.ID) {
		t.Fatalf("role event location = %#v", roleEvent.Location)
	}
	if roleEvent.Version != 1 {
		t.Fatalf("role event version = %d, want 1", roleEvent.Version)
	}
	assertNoSessionChange(t, events)
}

func TestSessionWriteFailureEmitsNoEventAndKeepsVersion(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	events, unsubscribe := subscribeSessionChanges(t, system)
	defer unsubscribe()
	roleID := "role-events"

	session, err := system.CreateSession(ctx, roleID, "events")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if event := nextSessionChange(t, events); event.Version != 1 {
		t.Fatalf("create event version = %d, want 1", event.Version)
	}

	indexFile, err := system.sessionIndexFile(roleSessionScope(roleID))
	if err != nil {
		t.Fatalf("sessionIndexFile() error = %v", err)
	}
	if err := os.Remove(indexFile); err != nil {
		t.Fatalf("Remove(index) error = %v", err)
	}
	if err := os.Mkdir(indexFile, 0o755); err != nil {
		t.Fatalf("Mkdir(index) error = %v", err)
	}

	session.Title = "failed"
	err = system.SaveSession(ctx, session)
	assertAppErrorCode(t, err, "storage.index_failed")
	assertNoSessionChange(t, events)
	loaded, err := system.LoadSession(ctx, roleID, session.ID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if loaded.Version != 1 || loaded.Title == "failed" {
		t.Fatalf("failed write changed session facts: %#v", loaded)
	}

	if err := os.Remove(indexFile); err != nil {
		t.Fatalf("Remove(index directory) error = %v", err)
	}
	session.Title = "succeeded"
	if err := system.SaveSession(ctx, session); err != nil {
		t.Fatalf("SaveSession() after failure error = %v", err)
	}
	event := nextSessionChange(t, events)
	if event.Version != 2 || event.ChangeType != types.SessionChangeSessionPersisted {
		t.Fatalf("event after retry = %#v", event)
	}
	loaded, err = system.LoadSession(ctx, roleID, session.ID)
	if err != nil {
		t.Fatalf("LoadSession() after retry error = %v", err)
	}
	if loaded.Version != 2 || loaded.Title != "succeeded" {
		t.Fatalf("session after retry = %#v", loaded)
	}
	assertNoSessionChange(t, events)
}

func TestNoopSessionMutationDoesNotWriteOrEmit(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	events, unsubscribe := subscribeSessionChanges(t, system)
	defer unsubscribe()
	roleID := "role-events"

	session, err := system.CreateSession(ctx, roleID, "events")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if event := nextSessionChange(t, events); event.Version != 1 {
		t.Fatalf("create event version = %d, want 1", event.Version)
	}

	wrote, err := system.ClaimAsyncToolResult(ctx, session, "missing-task", "run-1")
	if err != nil {
		t.Fatalf("ClaimAsyncToolResult() error = %v", err)
	}
	if wrote {
		t.Fatal("ClaimAsyncToolResult() wrote for a missing task")
	}
	assertNoSessionChange(t, events)
	loaded, err := system.LoadSession(ctx, roleID, session.ID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if loaded.Version != 1 {
		t.Fatalf("no-op mutation changed version to %d", loaded.Version)
	}
}

func TestConcurrentSessionWritesEmitOneEventPerVersion(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	events, unsubscribe := subscribeSessionChanges(t, system)
	defer unsubscribe()
	roleID := "role-events"

	session, err := system.CreateSession(ctx, roleID, "events")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if event := nextSessionChange(t, events); event.Version != 1 {
		t.Fatalf("create event version = %d, want 1", event.Version)
	}

	const total = 12
	var wait sync.WaitGroup
	for index := 0; index < total; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := system.UpdateSessionTitle(ctx, roleID, session.ID, "concurrent"); err != nil {
				t.Errorf("UpdateSessionTitle() error = %v", err)
			}
		}()
	}
	wait.Wait()

	versions := map[uint64]struct{}{}
	for index := 0; index < total; index++ {
		event := nextSessionChange(t, events)
		if event.ChangeType != types.SessionChangeSessionPersisted {
			t.Fatalf("event change type = %s", event.ChangeType)
		}
		if event.Version < 2 || event.Version > total+1 {
			t.Fatalf("event version = %d, want in [2, %d]", event.Version, total+1)
		}
		if _, exists := versions[event.Version]; exists {
			t.Fatalf("duplicate event version = %d", event.Version)
		}
		versions[event.Version] = struct{}{}
	}
	assertNoSessionChange(t, events)
	loaded, err := system.LoadSession(ctx, roleID, session.ID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if loaded.Version != total+1 {
		t.Fatalf("session version = %d, want %d", loaded.Version, total+1)
	}
}

func TestSubscribeSessionChangesClosesOnCancelAndUnsubscribe(t *testing.T) {
	system := newTestSystem(t)
	ctx, cancel := context.WithCancel(context.Background())
	events, unsubscribe, err := system.SubscribeSessionChanges(ctx)
	if err != nil {
		t.Fatalf("SubscribeSessionChanges() error = %v", err)
	}
	cancel()
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("session change channel must not deliver after cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session change channel did not close after cancellation")
	}
	unsubscribe()
	unsubscribe()

	cancelled, cancelSubscription, err := system.SubscribeSessionChanges(context.Background())
	if err != nil {
		t.Fatalf("SubscribeSessionChanges() error = %v", err)
	}
	cancelSubscription()
	select {
	case _, ok := <-cancelled:
		if ok {
			t.Fatal("session change channel must not deliver after unsubscribe")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session change channel did not close after unsubscribe")
	}

	// 取消订阅后写盘照常成功，不得因无订阅者而失败或阻塞。
	if _, err := system.CreateSession(context.Background(), "role-events", "after-unsubscribe"); err != nil {
		t.Fatalf("CreateSession() after unsubscribe error = %v", err)
	}
}
