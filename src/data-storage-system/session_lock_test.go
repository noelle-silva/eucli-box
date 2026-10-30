package datastorage

import (
	"context"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestSessionReadsWaitForScopeFileReplacement(t *testing.T) {
	system := newTestSystem(t)
	session := types.Session{ID: "session-lock", RoleID: "developer", Title: "lock"}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	lock, err := system.scopeLock(roleSessionScope("developer"))
	if err != nil {
		t.Fatalf("scopeLock() error = %v", err)
	}
	lock.mu.Lock()
	loaded := make(chan error, 1)
	go func() {
		_, err := system.LoadSession(context.Background(), "developer", session.ID)
		loaded <- err
	}()

	select {
	case err := <-loaded:
		t.Fatalf("LoadSession() returned while scope file replacement was in progress: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	lock.mu.Unlock()

	select {
	case err := <-loaded:
		if err != nil {
			t.Fatalf("LoadSession() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("LoadSession() did not proceed after scope lock was released")
	}
}

func TestSessionListReadsWaitForScopeFileReplacement(t *testing.T) {
	system := newTestSystem(t)
	session := types.Session{ID: "session-list-lock", RoleID: "developer", Title: "lock"}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	lock, err := system.scopeLock(roleSessionScope("developer"))
	if err != nil {
		t.Fatalf("scopeLock() error = %v", err)
	}
	lock.mu.Lock()
	listed := make(chan error, 1)
	go func() {
		_, err := system.ListSessions(context.Background(), "developer")
		listed <- err
	}()

	select {
	case err := <-listed:
		t.Fatalf("ListSessions() returned while scope file replacement was in progress: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	lock.mu.Unlock()

	select {
	case err := <-listed:
		if err != nil {
			t.Fatalf("ListSessions() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ListSessions() did not proceed after scope lock was released")
	}
}
