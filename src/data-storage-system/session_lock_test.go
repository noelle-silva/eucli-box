package datastorage

import (
	"context"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestSessionReadsWaitForSessionWriteLock(t *testing.T) {
	system := newTestSystem(t)
	session := types.Session{ID: "session-lock", RoleID: "developer", Title: "lock"}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	system.sessionMu.Lock()
	loaded := make(chan error, 1)
	go func() {
		_, err := system.LoadSession(context.Background(), "developer", session.ID)
		loaded <- err
	}()

	select {
	case err := <-loaded:
		t.Fatalf("LoadSession() returned while session lock was held: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	system.sessionMu.Unlock()

	select {
	case err := <-loaded:
		if err != nil {
			t.Fatalf("LoadSession() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("LoadSession() did not proceed after session lock was released")
	}
}

func TestSessionListReadsWaitForSessionWriteLock(t *testing.T) {
	system := newTestSystem(t)
	session := types.Session{ID: "session-list-lock", RoleID: "developer", Title: "lock"}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	system.sessionMu.Lock()
	listed := make(chan error, 1)
	go func() {
		_, err := system.ListSessions(context.Background(), "developer")
		listed <- err
	}()

	select {
	case err := <-listed:
		t.Fatalf("ListSessions() returned while session lock was held: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	system.sessionMu.Unlock()

	select {
	case err := <-listed:
		if err != nil {
			t.Fatalf("ListSessions() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ListSessions() did not proceed after session lock was released")
	}
}
