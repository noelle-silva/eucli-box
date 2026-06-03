package main

import "testing"

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
	if parts := objectList(messages[0]["parts"]); len(parts) != 1 || stringField(parts[0], "callId") != "call-1" {
		t.Fatalf("ui parts = %#v", messages[0]["parts"])
	}

	back := fromUIChat(ui, "developer")
	backMessages := objectList(back["messages"])
	if len(backMessages) != 1 {
		t.Fatalf("back messages = %#v", back["messages"])
	}
	if parts := objectList(backMessages[0]["parts"]); len(parts) != 1 || stringField(parts[0], "toolName") != "shell_command" {
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
