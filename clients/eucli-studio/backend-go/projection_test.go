package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatProjectionPreservesMessageParts(t *testing.T) {
	session := map[string]any{
		"id":        "session-1",
		"roleId":    "developer",
		"title":     "Tools",
		"createdAt": "2026-06-03T10:00:00Z",
		"updatedAt": "2026-06-03T10:00:00Z",
		"messages": []any{map[string]any{
			"id":              "m1",
			"type":            "assistant",
			"content":         "checking",
			"parentMessageId": "u1",
			"branchId":        "main",
			"createdAt":       "2026-06-03T10:00:00Z",
			"updatedAt":       "2026-06-03T10:00:00Z",
			"parts": []any{map[string]any{
				"id":       "part-1",
				"type":     "tool",
				"source":   "text_protocol",
				"raw":      "<<<TOOL_REQUEST>>>\n[tool]: shell_command\n[command]: pwd\n<<<END_TOOL_REQUEST>>>",
				"callId":   "call-1",
				"toolName": "shell_command",
				"state":    "completed",
				"input":    map[string]any{"command": "pwd"},
				"result":   map[string]any{"status": "success", "content": "ok"},
			}},
		}},
	}

	ui := toUIChat(session)
	messages := objectList(ui["messages"])
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", ui["messages"])
	}
	if messages[0]["role"] != "assistant" || messages[0]["type"] != "assistant" {
		t.Fatalf("ui message role/type = %#v", messages[0])
	}
	if parts := objectList(messages[0]["parts"]); len(parts) != 1 || stringField(parts[0], "source") != "text_protocol" || stringField(parts[0], "raw") == "" || stringField(parts[0], "callId") != "call-1" {
		t.Fatalf("ui parts = %#v", messages[0]["parts"])
	}

	back := fromUIChat(ui, "developer")
	backMessages := objectList(back["messages"])
	if len(backMessages) != 1 {
		t.Fatalf("back messages = %#v", back["messages"])
	}
	if parts := objectList(backMessages[0]["parts"]); len(parts) != 1 || stringField(parts[0], "source") != "text_protocol" || stringField(parts[0], "raw") == "" || stringField(parts[0], "toolName") != "shell_command" {
		t.Fatalf("back parts = %#v", backMessages[0]["parts"])
	}
}

func TestLegacyToolMessageProjectsAsAssistantSide(t *testing.T) {
	message := map[string]any{"type": "tool", "content": "ok"}
	if role := messageRole(message); role != "assistant" {
		t.Fatalf("role = %q", role)
	}
	if typ := messageStorageType(message); typ != "tool" {
		t.Fatalf("type = %q", typ)
	}
}

func TestSessionFavoritesStorageKeyUsesRootAction(t *testing.T) {
	stored := map[string]any{"folders": []any{}, "chatRefsByFolderId": map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sessions/favorites" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": stored})
		case http.MethodPut:
			var next map[string]any
			if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
				t.Fatalf("decode favorites request: %v", err)
			}
			stored = next
			_ = json.NewEncoder(w).Encode(map[string]any{"data": stored})
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	store, err := newConfigStore(t.TempDir())
	if err != nil {
		t.Fatalf("newConfigStore() error = %v", err)
	}
	if _, err := store.save(clientConfig{EucliBoxURL: server.URL}); err != nil {
		t.Fatalf("save config error = %v", err)
	}
	projection := newProjectionService(store, newEBClient(store))
	favorites := map[string]any{
		"folders":            []any{map[string]any{"id": "favf-1", "name": "Important", "parentId": "", "createdAt": float64(1), "updatedAt": float64(1)}},
		"chatRefsByFolderId": map[string]any{"favf-1": []any{map[string]any{"targetKind": "role", "targetId": "developer", "chatId": "session-1", "addedAt": float64(2)}}},
	}

	if err := projection.set(context.Background(), sessionFavoritesKey, favorites); err != nil {
		t.Fatalf("projection.set() error = %v", err)
	}
	loaded, err := projection.get(context.Background(), sessionFavoritesKey)
	if err != nil {
		t.Fatalf("projection.get() error = %v", err)
	}
	loadedMap := objectMap(loaded)
	if refs := objectMap(loadedMap["chatRefsByFolderId"]); refs["favf-1"] == nil {
		t.Fatalf("loaded favorites = %#v", loadedMap)
	}
}

func TestRunningSessionMarksLatestAssistantPending(t *testing.T) {
	session := map[string]any{
		"id":        "session-1",
		"roleId":    "developer",
		"title":     "Streaming",
		"status":    runStatusRunning,
		"createdAt": "2026-06-03T10:00:00Z",
		"updatedAt": "2026-06-03T10:00:00Z",
		"messages": []any{
			map[string]any{"id": "u1", "type": "user", "content": "hi", "createdAt": "2026-06-03T10:00:00Z", "updatedAt": "2026-06-03T10:00:00Z"},
			map[string]any{"id": "a1", "type": "assistant", "content": "", "parentMessageId": "u1", "createdAt": "2026-06-03T10:00:01Z", "updatedAt": "2026-06-03T10:00:01Z"},
		},
	}

	ui := toUIChat(session)
	messages := objectList(ui["messages"])
	if len(messages) != 2 {
		t.Fatalf("messages = %#v", ui["messages"])
	}
	assistant := messages[1]
	if assistant["pending"] != true || assistant["streaming"] != true {
		t.Fatalf("assistant pending/streaming = %#v", assistant)
	}
}

func TestChatProjectionPreservesAssistantError(t *testing.T) {
	session := map[string]any{
		"id":        "session-1",
		"roleId":    "developer",
		"title":     "Error",
		"createdAt": "2026-06-03T10:00:00Z",
		"updatedAt": "2026-06-03T10:00:00Z",
		"messages": []any{map[string]any{
			"id":              "a1",
			"type":            "assistant",
			"content":         "",
			"parentMessageId": "u1",
			"createdAt":       "2026-06-03T10:00:00Z",
			"updatedAt":       "2026-06-03T10:00:00Z",
			"error":           map[string]any{"code": "provider.service_failed", "message": "upstream says no", "details": map[string]any{"body": "raw"}},
		}},
	}

	ui := toUIChat(session)
	messages := objectList(ui["messages"])
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", ui["messages"])
	}
	if errBox := objectMap(messages[0]["error"]); stringField(errBox, "message") != "upstream says no" || stringField(errBox, "code") != "provider.service_failed" {
		t.Fatalf("ui error = %#v", messages[0]["error"])
	}
	back := fromUIChat(ui, "developer")
	backMessages := objectList(back["messages"])
	if errBox := objectMap(backMessages[0]["error"]); stringField(errBox, "message") != "upstream says no" {
		t.Fatalf("back error = %#v", backMessages[0]["error"])
	}
}

func TestStableUpdatedAtUsesMaxPositiveValue(t *testing.T) {
	if got := stableUpdatedAt(0, -1); got != 1 {
		t.Fatalf("empty stable updatedAt = %d", got)
	}
	if got := stableUpdatedAt(10, 42, 20); got != 42 {
		t.Fatalf("stable updatedAt = %d", got)
	}
}
