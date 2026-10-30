package datastorage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func readSessionIndexDocument(t *testing.T, system *system, scope sessionScope) sessionIndexDocument {
	t.Helper()
	target, err := system.sessionIndexFile(scope)
	if err != nil {
		t.Fatalf("sessionIndexFile() error = %v", err)
	}
	document, err := readJSON[sessionIndexDocument](context.Background(), target)
	if err != nil {
		t.Fatalf("read session index error = %v", err)
	}
	return document
}

func TestListSessionsReadsScopeIndexWithoutScanningSessionData(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	for _, id := range []string{"session-a", "session-b"} {
		session := types.Session{ID: id, RoleID: "developer", Title: id, CreatedAt: now, UpdatedAt: now, LastActive: now}
		if err := system.SaveSession(ctx, session); err != nil {
			t.Fatalf("SaveSession(%s) error = %v", id, err)
		}
		dataFile := filepath.Join(system.paths.root, "sessions", "roles", "developer", id, "data.json")
		if err := os.WriteFile(dataFile, []byte("not-json"), 0o644); err != nil {
			t.Fatalf("corrupt data.json error = %v", err)
		}
	}

	sessions, err := system.ListSessions(ctx, "developer")
	if err != nil {
		t.Fatalf("ListSessions() must read the index, got error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %#v", sessions)
	}
	metrics := system.SessionStorageMetrics()
	if metrics.IndexBytes == 0 {
		t.Fatal("list must be served from the index")
	}
	if metrics.DataReads != 0 {
		t.Fatalf("list must not read session data.json, data reads = %d", metrics.DataReads)
	}
}

func TestListSessionsRebuildsMissingIndex(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	session := types.Session{ID: "session-missing-index", RoleID: "developer", Title: "missing", LastActive: time.Now().UTC()}
	if err := system.SaveSession(ctx, session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	indexFile := filepath.Join(system.paths.root, "sessions", "roles", "developer", "index.json")
	if err := os.Remove(indexFile); err != nil {
		t.Fatalf("Remove(index) error = %v", err)
	}

	sessions, err := system.ListSessions(ctx, "developer")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != session.ID {
		t.Fatalf("sessions = %#v", sessions)
	}
	document := readSessionIndexDocument(t, system, roleSessionScope("developer"))
	if document.Dirty || document.SchemaVersion != sessionIndexSchemaVersion {
		t.Fatalf("rebuilt index = %#v", document)
	}
}

func TestListSessionsRebuildsDirtyIndex(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	session := types.Session{ID: "session-dirty", RoleID: "developer", Title: "dirty", LastActive: time.Now().UTC()}
	if err := system.SaveSession(ctx, session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	scope := roleSessionScope("developer")
	document := readSessionIndexDocument(t, system, scope)
	document.Dirty = true
	indexFile, err := system.sessionIndexFile(scope)
	if err != nil {
		t.Fatalf("sessionIndexFile() error = %v", err)
	}
	if err := writeJSON(ctx, indexFile, document); err != nil {
		t.Fatalf("write dirty index error = %v", err)
	}

	sessions, err := system.ListSessions(ctx, "developer")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != session.ID {
		t.Fatalf("sessions = %#v", sessions)
	}
	if readSessionIndexDocument(t, system, scope).Dirty {
		t.Fatal("dirty flag must be cleared by self-heal")
	}
}

func TestListSessionsFiltersDanglingEntries(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, id := range []string{"session-keep", "session-removed"} {
		if err := system.SaveSession(ctx, types.Session{ID: id, RoleID: "developer", Title: id, LastActive: now}); err != nil {
			t.Fatalf("SaveSession(%s) error = %v", id, err)
		}
	}
	removedDir := filepath.Join(system.paths.root, "sessions", "roles", "developer", "session-removed")
	if err := os.RemoveAll(removedDir); err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}

	sessions, err := system.ListSessions(ctx, "developer")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "session-keep" {
		t.Fatalf("sessions = %#v", sessions)
	}
	document := readSessionIndexDocument(t, system, roleSessionScope("developer"))
	if len(document.Sessions) != 1 || document.Sessions[0].ID != "session-keep" {
		t.Fatalf("index after repair = %#v", document.Sessions)
	}
}

func TestSessionSummaryCarriesProjectionFactsAndDeterministicSort(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	sameMoment := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	older := types.Session{ID: "session-older", RoleID: "developer", Title: "older", CreatedAt: sameMoment.Add(-time.Hour), UpdatedAt: sameMoment.Add(-time.Hour), LastActive: sameMoment.Add(-time.Hour)}
	newerB := types.Session{ID: "session-b", RoleID: "developer", Title: "newer-b", CreatedAt: sameMoment, UpdatedAt: sameMoment, LastActive: sameMoment}
	newerA := types.Session{ID: "session-a", RoleID: "developer", Title: "newer-a", CreatedAt: sameMoment, UpdatedAt: sameMoment, LastActive: sameMoment, Messages: []types.Message{
		{ID: "m1", Type: "user", Content: "你好", BranchID: "main", CreatedAt: sameMoment, UpdatedAt: sameMoment},
	}}
	for _, session := range []types.Session{older, newerB, newerA} {
		if err := system.SaveSession(ctx, session); err != nil {
			t.Fatalf("SaveSession(%s) error = %v", session.ID, err)
		}
	}

	sessions, err := system.ListSessions(ctx, "developer")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 3 || sessions[0].ID != "session-a" || sessions[1].ID != "session-b" || sessions[2].ID != "session-older" {
		t.Fatalf("sort contract violated: %#v", sessions)
	}
	first := sessions[0]
	if !first.CreatedAt.Equal(sameMoment) || first.MessageCount != 1 || first.Source != roleSessionScope("developer").fingerprint() {
		t.Fatalf("summary projection facts = %#v", first)
	}
}

func TestSessionIndexUpdatedIncrementallyPerScope(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := system.SaveSession(ctx, types.Session{ID: "session-role", RoleID: "developer", Title: "role", LastActive: now}); err != nil {
		t.Fatalf("SaveSession(role) error = %v", err)
	}
	roleScope := roleSessionScope("developer")
	roleIndexBefore := readSessionIndexDocument(t, system, roleScope)
	if err := system.SaveSession(ctx, types.Session{ID: "session-group", GroupID: "group-1", Title: "group", LastActive: now}); err != nil {
		t.Fatalf("SaveSession(group) error = %v", err)
	}

	roleIndexAfter := readSessionIndexDocument(t, system, roleScope)
	if roleIndexAfter.Rev != roleIndexBefore.Rev {
		t.Fatalf("role index rev changed by a group write: before=%d after=%d", roleIndexBefore.Rev, roleIndexAfter.Rev)
	}
	groupIndex := readSessionIndexDocument(t, system, groupSessionScope("group-1"))
	if len(groupIndex.Sessions) != 1 || groupIndex.Sessions[0].ID != "session-group" {
		t.Fatalf("group index = %#v", groupIndex.Sessions)
	}
	metrics := system.SessionStorageMetrics()
	if metrics.IndexRebuilds != 0 {
		t.Fatalf("writes must not rebuild indexes, rebuilds = %d", metrics.IndexRebuilds)
	}
	if metrics.IndexWrites != 2 {
		t.Fatalf("index writes = %d, want 2 (one per saved session)", metrics.IndexWrites)
	}
}

func TestSessionSaveFailsWhenIndexWriteFails(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	session := types.Session{ID: "session-index-failure", RoleID: "developer", Title: "before", LastActive: time.Now().UTC()}
	if err := system.SaveSession(ctx, session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	indexFile := filepath.Join(system.paths.root, "sessions", "roles", "developer", "index.json")
	if err := os.Remove(indexFile); err != nil {
		t.Fatalf("Remove(index) error = %v", err)
	}
	if err := os.Mkdir(indexFile, 0o755); err != nil {
		t.Fatalf("Mkdir(index) error = %v", err)
	}

	session.Title = "after"
	err := system.SaveSession(ctx, session)
	assertAppErrorCode(t, err, "storage.index_failed")
	if system.SessionStorageMetrics().IndexFailures == 0 {
		t.Fatal("index failure must be counted")
	}
	loaded, loadErr := system.LoadSession(ctx, "developer", session.ID)
	if loadErr != nil {
		t.Fatalf("LoadSession() error = %v", loadErr)
	}
	if loaded.Title != "before" {
		t.Fatalf("data must not be written when the index dirty marker fails: title=%q", loaded.Title)
	}
}

func TestLoadSessionIsPureAcrossReads(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	dir := filepath.Join(system.paths.root, "sessions", "roles", "developer", "session-legacy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacy := `{"id":"session-legacy","roleId":"developer","title":"legacy","status":"created","messages":[{"id":"m1","type":"user","content":"hi"}]}`
	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	first, err := system.LoadSession(ctx, "developer", "session-legacy")
	if err != nil {
		t.Fatalf("LoadSession(first) error = %v", err)
	}
	second, err := system.LoadSession(ctx, "developer", "session-legacy")
	if err != nil {
		t.Fatalf("LoadSession(second) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("read path is not pure: first=%#v second=%#v", first, second)
	}
	if !first.CreatedAt.IsZero() || !first.LastActive.IsZero() || !first.Messages[0].CreatedAt.IsZero() {
		t.Fatalf("read path invented timestamps: %#v", first)
	}
}

func TestDeleteSessionRemovesIndexEntryAndRecyclesDirectory(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	session := types.Session{ID: "session-delete", RoleID: "developer", Title: "delete", LastActive: time.Now().UTC()}
	if err := system.SaveSession(ctx, session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if err := system.DeleteSession(ctx, "developer", session.ID); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}

	sessions, err := system.ListSessions(ctx, "developer")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions = %#v", sessions)
	}
	document := readSessionIndexDocument(t, system, roleSessionScope("developer"))
	if document.Dirty || len(document.Sessions) != 0 {
		t.Fatalf("index after delete = %#v", document)
	}
	records, err := readRecycleRecords(ctx, system.paths.recycleRoot())
	if err != nil {
		t.Fatalf("readRecycleRecords() error = %v", err)
	}
	if len(records) != 1 || records[0].OriginalID != session.ID || records[0].OriginalType != types.StorageItemSession {
		t.Fatalf("recycle records = %#v", records)
	}
}

func TestConcurrentSessionSavesKeepEveryIndexEntry(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	const total = 24
	var wait sync.WaitGroup
	for index := 0; index < total; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			now := time.Now().UTC()
			session := types.Session{ID: fmt.Sprintf("session-concurrent-%02d", index), RoleID: "developer", Title: "concurrent", LastActive: now}
			if err := system.SaveSession(ctx, session); err != nil {
				t.Errorf("SaveSession(%d) error = %v", index, err)
			}
		}(index)
	}
	wait.Wait()

	sessions, err := system.ListSessions(ctx, "developer")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != total {
		t.Fatalf("sessions = %d, want %d", len(sessions), total)
	}
	if document := readSessionIndexDocument(t, system, roleSessionScope("developer")); len(document.Sessions) != total {
		t.Fatalf("index entries = %d, want %d", len(document.Sessions), total)
	}
}

func TestConcurrentSessionSavesAcrossScopesStayIsolated(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	scopes := []string{"role-a", "role-b", "role-c"}
	const perScope = 8
	var wait sync.WaitGroup
	for _, roleID := range scopes {
		for index := 0; index < perScope; index++ {
			wait.Add(1)
			go func(roleID string, index int) {
				defer wait.Done()
				now := time.Now().UTC()
				session := types.Session{ID: fmt.Sprintf("%s-session-%02d", roleID, index), RoleID: roleID, Title: "isolated", LastActive: now}
				if err := system.SaveSession(ctx, session); err != nil {
					t.Errorf("SaveSession(%s, %d) error = %v", roleID, index, err)
				}
			}(roleID, index)
		}
	}
	wait.Wait()

	for _, roleID := range scopes {
		sessions, err := system.ListSessions(ctx, roleID)
		if err != nil {
			t.Fatalf("ListSessions(%s) error = %v", roleID, err)
		}
		if len(sessions) != perScope {
			t.Fatalf("ListSessions(%s) = %d, want %d", roleID, len(sessions), perScope)
		}
		for _, summary := range sessions {
			if summary.Source != roleSessionScope(roleID).fingerprint() {
				t.Fatalf("summary source = %q, want %q", summary.Source, roleSessionScope(roleID).fingerprint())
			}
		}
	}
}
