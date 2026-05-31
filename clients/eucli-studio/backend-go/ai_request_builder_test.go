package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildOpenAIChatReqFromStorageRejectsLocalModelPath(t *testing.T) {
	dataDir := t.TempDir()
	svc := newService(dataDir)
	_, err := svc.buildOpenAIChatReqFromStorage(map[string]any{
		"roleId": "r1",
		"chatId": "c1",
		"stream": false,
	})
	if err == nil || !strings.Contains(err.Error(), "eucli-box") {
		t.Fatalf("expected eucli-box-only error, got %v", err)
	}
}

func TestLoadSplitMetaKeepsFoldersCompatibleWithCallers(t *testing.T) {
	dataDir := t.TempDir()
	writeJSONForRunnerTest(t, filepath.Join(dataDir, "meta", "index.json"), map[string]any{
		"schemaVersion": 1,
		"dataVersion":   7,
		"settings":      map[string]any{},
	})
	writeJSONForRunnerTest(t, filepath.Join(dataDir, "chats", "index.json"), map[string]any{
		"schemaVersion": 1,
		"roleOrder":     []any{"r1"},
		"roleFolders":   map[string]any{"r1": "Alice"},
	})
	writeJSONForRunnerTest(t, filepath.Join(dataDir, "chats", "Alice", "index.json"), map[string]any{
		"schemaVersion": 1,
		"activeChatId":  "c1",
		"chatIds":       []any{"c1"},
		"chatUpdatedAt": map[string]any{"c1": int64(2)},
		"chatMetas":     []any{map[string]any{"id": "c1", "title": "Hello", "updatedAt": int64(2)}},
	})
	writeJSONForRunnerTest(t, filepath.Join(dataDir, "groups", "index.json"), map[string]any{
		"schemaVersion": 1,
		"groupOrder":    []any{"g1"},
		"groupFolders":  map[string]any{"g1": "Team"},
	})
	writeJSONForRunnerTest(t, filepath.Join(dataDir, "groups", "Team", "chats", "index.json"), map[string]any{
		"schemaVersion": 1,
		"activeChatId":  "gc1",
		"chatIds":       []any{"gc1"},
		"chatUpdatedAt": map[string]any{"gc1": int64(3)},
		"chatMetas":     []any{map[string]any{"id": "gc1", "title": "Team", "updatedAt": int64(3)}},
	})

	svc := newService(dataDir)
	meta, err := svc.loadSplitMeta()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(asString(asMap(meta["roleFolders"])["r1"])); got != "Alice" {
		t.Fatalf("role folder = %q", got)
	}
	if got := strings.TrimSpace(asString(asMap(meta["groupFolders"])["g1"])); got != "Team" {
		t.Fatalf("group folder = %q", got)
	}
	if got := asMap(asMap(meta["chatIndexByRole"])["r1"]); len(asSlice(got["chatMetas"])) != 1 {
		t.Fatalf("role chat index = %#v", got)
	}
	if got := asMap(asMap(meta["chatIndexByGroup"])["g1"]); len(asSlice(got["chatMetas"])) != 1 {
		t.Fatalf("group chat index = %#v", got)
	}
}

func TestRunJobStubWithTargetUsesTargetIdentity(t *testing.T) {
	job := runJobStubWithTarget(map[string]any{
		"roleId":       "stale-role",
		"groupId":      "stale-group",
		"chatId":       "stale-chat",
		"branchId":     "stale-branch",
		"assistantMid": "stale-mid",
		"generationId": "stale-gen",
		"stream":       true,
	}, aiRunTarget{
		Kind:         "group",
		RoleID:       "r1",
		GroupID:      "g1",
		ChatID:       "c1",
		BranchID:     "main",
		AssistantMid: "m1",
		GenerationID: "gen-1",
	})

	if job["roleId"] != "r1" || job["groupId"] != "g1" || job["chatId"] != "c1" || job["branchId"] != "main" || job["assistantMid"] != "m1" || job["generationId"] != "gen-1" {
		t.Fatalf("job identity was not normalized from target: %#v", job)
	}
	if job["stream"] != true {
		t.Fatalf("non-identity fields should be preserved: %#v", job)
	}
}
