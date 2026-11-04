package datastorage

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestSessionVersionStartsAtOneAndIncreasesOnEveryWrite(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	roleID := "role-version"
	session, err := system.CreateSession(ctx, roleID, "version")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if session.Version != 1 {
		t.Fatalf("created session version = %d, want 1", session.Version)
	}
	loadVersion := func() uint64 {
		t.Helper()
		loaded, err := system.LoadSession(ctx, roleID, session.ID)
		if err != nil {
			t.Fatalf("LoadSession() error = %v", err)
		}
		return loaded.Version
	}
	assertVersion := func(want uint64) {
		t.Helper()
		if got := loadVersion(); got != want {
			t.Fatalf("session version = %d, want %d", got, want)
		}
	}
	assertVersion(1)

	session.Title = "renamed"
	if err := system.SaveSession(ctx, session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	assertVersion(2)

	updated, err := system.UpdateSessionTitle(ctx, roleID, session.ID, "title")
	if err != nil {
		t.Fatalf("UpdateSessionTitle() error = %v", err)
	}
	if updated.Version != 3 {
		t.Fatalf("updated session version = %d, want 3", updated.Version)
	}

	addMessage := func(id string, parentID string, wantVersion uint64) {
		t.Helper()
		now := time.Now().UTC()
		message := types.Message{ID: id, Type: "user", Content: id, ParentMessageID: parentID, BranchID: "main", CreatedAt: now, UpdatedAt: now}
		save := types.SessionMessageSave{Session: types.Session{ID: session.ID, RoleID: roleID}, Status: types.RunStatusRunning, Writes: []types.SessionMessageWrite{{Message: message}}}
		if err := system.SaveSessionMessages(ctx, save); err != nil {
			t.Fatalf("SaveSessionMessages(%s) error = %v", id, err)
		}
		assertVersion(wantVersion)
	}
	addMessage("m1", "", 4)
	addMessage("m2", "m1", 5)
	addMessage("m3", "m2", 6)

	content := "edited"
	if _, err := system.UpdateSessionMessage(ctx, roleID, session.ID, "m3", types.SessionMessagePatch{Content: &content}); err != nil {
		t.Fatalf("UpdateSessionMessage() error = %v", err)
	}
	assertVersion(7)

	deleted, err := system.DeleteSessionMessage(ctx, roleID, session.ID, "m2")
	if err != nil {
		t.Fatalf("DeleteSessionMessage() error = %v", err)
	}
	if deleted.Version != 8 {
		t.Fatalf("delete single message version = %d, want 8", deleted.Version)
	}

	subtreeDeleted, err := system.DeleteSessionMessageSubtree(ctx, roleID, session.ID, "m3")
	if err != nil {
		t.Fatalf("DeleteSessionMessageSubtree() error = %v", err)
	}
	if subtreeDeleted.Version != 9 {
		t.Fatalf("delete subtree version = %d, want 9", subtreeDeleted.Version)
	}

	// 调用方携带的版本号不参与判定：即使反复提交版本 0，存储仍严格加一。
	stale := types.Session{ID: session.ID, RoleID: roleID, Title: "stale"}
	if err := system.SaveSession(ctx, stale); err != nil {
		t.Fatalf("SaveSession(stale) error = %v", err)
	}
	assertVersion(10)
	if err := system.SaveSession(ctx, stale); err != nil {
		t.Fatalf("SaveSession(stale again) error = %v", err)
	}
	assertVersion(11)

	summaries, err := system.ListSessions(ctx, roleID)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0].Version != 11 {
		t.Fatalf("session summaries = %#v, want version 11", summaries)
	}
}

func TestSessionVersionStartsAtOneForEverySessionForm(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()

	roleSession, err := system.CreateSession(ctx, "role-version", "role")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	groupSession, err := system.CreateGroupSession(ctx, "group-version", "group")
	if err != nil {
		t.Fatalf("CreateGroupSession() error = %v", err)
	}
	workspaceSession, err := system.CreateWorkspaceSession(ctx, "workspace-version", "role-version", "workspace")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}
	if roleSession.Version != 1 || groupSession.Version != 1 || workspaceSession.Version != 1 {
		t.Fatalf("created versions = role:%d group:%d workspace:%d, want 1", roleSession.Version, groupSession.Version, workspaceSession.Version)
	}

	roleUpdated, err := system.UpdateSessionTitle(ctx, "role-version", roleSession.ID, "role-updated")
	if err != nil {
		t.Fatalf("UpdateSessionTitle(role) error = %v", err)
	}
	groupUpdated, err := system.UpdateGroupSessionTitle(ctx, "group-version", groupSession.ID, "group-updated")
	if err != nil {
		t.Fatalf("UpdateGroupSessionTitle() error = %v", err)
	}
	workspaceUpdated, err := system.UpdateWorkspaceSessionTitle(ctx, "workspace-version", "role-version", workspaceSession.ID, "workspace-updated")
	if err != nil {
		t.Fatalf("UpdateWorkspaceSessionTitle() error = %v", err)
	}
	if roleUpdated.Version != 2 || groupUpdated.Version != 2 || workspaceUpdated.Version != 2 {
		t.Fatalf("updated versions = role:%d group:%d workspace:%d, want 2", roleUpdated.Version, groupUpdated.Version, workspaceUpdated.Version)
	}

	loadedGroup, err := system.LoadGroupSession(ctx, "group-version", groupSession.ID)
	if err != nil {
		t.Fatalf("LoadGroupSession() error = %v", err)
	}
	loadedWorkspace, err := system.LoadWorkspaceSession(ctx, "workspace-version", "role-version", workspaceSession.ID)
	if err != nil {
		t.Fatalf("LoadWorkspaceSession() error = %v", err)
	}
	if loadedGroup.Version != 2 || loadedWorkspace.Version != 2 {
		t.Fatalf("loaded versions = group:%d workspace:%d, want 2", loadedGroup.Version, loadedWorkspace.Version)
	}
}

func TestConcurrentSessionWritesKeepVersionStrictlyIncreasing(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	roleID := "role-version"
	session, err := system.CreateSession(ctx, roleID, "version")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	const total = 16
	versions := make(chan uint64, total)
	var wait sync.WaitGroup
	for index := 0; index < total; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			updated, err := system.UpdateSessionTitle(ctx, roleID, session.ID, "concurrent")
			if err != nil {
				t.Errorf("UpdateSessionTitle(%d) error = %v", index, err)
				return
			}
			versions <- updated.Version
		}(index)
	}
	wait.Wait()
	close(versions)

	seen := map[uint64]struct{}{}
	for version := range versions {
		if version < 2 {
			t.Fatalf("concurrent write version = %d, want at least 2", version)
		}
		seen[version] = struct{}{}
	}
	if len(seen) != total {
		t.Fatalf("concurrent write versions = %#v, want %d distinct values", seen, total)
	}
	loaded, err := system.LoadSession(ctx, roleID, session.ID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if loaded.Version != total+1 {
		t.Fatalf("session version = %d, want %d", loaded.Version, total+1)
	}
}

func TestSessionVersionSurvivesIndexRebuild(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	roleID := "role-version"
	session, err := system.CreateSession(ctx, roleID, "version")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := system.UpdateSessionTitle(ctx, roleID, session.ID, "second"); err != nil {
		t.Fatalf("UpdateSessionTitle() error = %v", err)
	}

	indexFile, err := system.sessionIndexFile(roleSessionScope(roleID))
	if err != nil {
		t.Fatalf("sessionIndexFile() error = %v", err)
	}
	if err := os.Remove(indexFile); err != nil {
		t.Fatalf("Remove(index) error = %v", err)
	}
	summaries, err := system.ListSessions(ctx, roleID)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0].Version != 2 {
		t.Fatalf("rebuilt summaries = %#v, want version 2", summaries)
	}

	updated, err := system.UpdateSessionTitle(ctx, roleID, session.ID, "third")
	if err != nil {
		t.Fatalf("UpdateSessionTitle() after rebuild error = %v", err)
	}
	if updated.Version != 3 {
		t.Fatalf("session version after rebuild = %d, want 3", updated.Version)
	}
}
