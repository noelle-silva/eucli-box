package datastorage

import (
	"context"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

// TestClaimAsyncToolResultIsExclusiveAndReversible 钉住 B6 约束 2/4：
// 认领以任务 ID 为键互斥；回退仅对认领者生效，回退后可被重新认领。
func TestClaimAsyncToolResultIsExclusiveAndReversible(t *testing.T) {
	system := newTestSystem(t)
	now := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	task := types.AsyncToolTask{ID: "async-1", RoleID: "developer", SessionID: "session-claim", ToolName: "file-reader", Status: types.AsyncToolTaskStatusSucceeded, SubmittedAt: now, FinishedAt: now}
	session := types.Session{ID: "session-claim", RoleID: "developer", Title: "Claim", Status: string(types.RunStatusRunning), CreatedAt: now, UpdatedAt: now, LastActive: now, AsyncToolTasks: []types.AsyncToolTask{task}}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	claimed, err := system.ClaimAsyncToolResult(context.Background(), session, "async-1", "run-a")
	if err != nil || !claimed {
		t.Fatalf("claim by run-a = %v, %v", claimed, err)
	}
	claimed, err = system.ClaimAsyncToolResult(context.Background(), session, "async-1", "run-b")
	if err != nil || claimed {
		t.Fatalf("second claim must lose: %v, %v", claimed, err)
	}

	if err := system.ReleaseAsyncToolResultClaim(context.Background(), session, "async-1", "run-b"); err != nil {
		t.Fatalf("release by non-owner error = %v", err)
	}
	claimed, err = system.ClaimAsyncToolResult(context.Background(), session, "async-1", "run-b")
	if err != nil || claimed {
		t.Fatalf("claim after non-owner release must still lose: %v, %v", claimed, err)
	}

	if err := system.ReleaseAsyncToolResultClaim(context.Background(), session, "async-1", "run-a"); err != nil {
		t.Fatalf("release by owner error = %v", err)
	}
	claimed, err = system.ClaimAsyncToolResult(context.Background(), session, "async-1", "run-b")
	if err != nil || !claimed {
		t.Fatalf("claim after owner release = %v, %v", claimed, err)
	}

	loaded, err := system.LoadSession(context.Background(), "developer", "session-claim")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if len(loaded.AsyncToolTasks) != 1 || loaded.AsyncToolTasks[0].InjectionClaimedAt.IsZero() || loaded.AsyncToolTasks[0].InjectionClaimRunID != "run-b" {
		t.Fatalf("persisted claim = %#v", loaded.AsyncToolTasks)
	}
}

// TestClaimAsyncToolResultRejectsCompletedTask 钉住 B6 约束 3：
// 落盘会话中已完成的标记是权威判据，已完成任务不可再被认领。
func TestClaimAsyncToolResultRejectsCompletedTask(t *testing.T) {
	system := newTestSystem(t)
	now := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	task := types.AsyncToolTask{ID: "async-1", RoleID: "developer", SessionID: "session-done", ToolName: "file-reader", Status: types.AsyncToolTaskStatusCompleted, CompletedAt: now, SubmittedAt: now, FinishedAt: now}
	session := types.Session{ID: "session-done", RoleID: "developer", Title: "Done", Status: string(types.RunStatusRunning), CreatedAt: now, UpdatedAt: now, LastActive: now, AsyncToolTasks: []types.AsyncToolTask{task}}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	claimed, err := system.ClaimAsyncToolResult(context.Background(), session, "async-1", "run-a")
	if err != nil || claimed {
		t.Fatalf("completed task must not be claimable: %v, %v", claimed, err)
	}
}
