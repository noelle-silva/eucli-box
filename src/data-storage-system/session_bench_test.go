package datastorage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

// BenchmarkListSessionsIndex 量化列表读路径：只读 scope 索引，不读 data.json。
func BenchmarkListSessionsIndex(b *testing.B) {
	system := newBenchmarkSystem(b, 100)
	ctx := context.Background()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := system.ListSessions(ctx, "developer"); err != nil {
			b.Fatalf("ListSessions() error = %v", err)
		}
	}
}

// BenchmarkSaveSessionMessages 量化热路径保存：单会话增量写，不触发全库重建。
func BenchmarkSaveSessionMessages(b *testing.B) {
	system := newBenchmarkSystem(b, 100)
	ctx := context.Background()
	now := time.Now().UTC()
	session := types.Session{ID: "session-benchmark", RoleID: "developer", Title: "benchmark", CreatedAt: now, UpdatedAt: now, LastActive: now, Messages: []types.Message{}}
	if err := system.SaveSession(ctx, session); err != nil {
		b.Fatalf("SaveSession() error = %v", err)
	}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		message := types.Message{ID: fmt.Sprintf("message-%d", index), Type: "user", Content: "增量保存", BranchID: "main", ParentMessageID: lastBenchmarkMessageID(index), CreatedAt: now, UpdatedAt: now}
		save := types.SessionMessageSave{Session: session, Status: types.RunStatusRunning, Writes: []types.SessionMessageWrite{{Message: message}}}
		if err := system.SaveSessionMessages(ctx, save); err != nil {
			b.Fatalf("SaveSessionMessages() error = %v", err)
		}
	}
}

func lastBenchmarkMessageID(index int) string {
	if index == 0 {
		return ""
	}
	return fmt.Sprintf("message-%d", index-1)
}

func newBenchmarkSystem(b *testing.B, sessions int) *system {
	b.Helper()
	system := newTestSystem(b)
	ctx := context.Background()
	base := time.Now().UTC()
	for index := 0; index < sessions; index++ {
		session := types.Session{
			ID:         fmt.Sprintf("session-%03d", index),
			RoleID:     "developer",
			Title:      "benchmark",
			CreatedAt:  base,
			UpdatedAt:  base,
			LastActive: base.Add(time.Duration(index) * time.Second),
			Messages: []types.Message{
				{ID: fmt.Sprintf("message-%03d", index), Type: "user", Content: "基准会话", BranchID: "main", CreatedAt: base, UpdatedAt: base},
			},
		}
		if err := system.SaveSession(ctx, session); err != nil {
			b.Fatalf("SaveSession(%d) error = %v", index, err)
		}
	}
	return system
}
