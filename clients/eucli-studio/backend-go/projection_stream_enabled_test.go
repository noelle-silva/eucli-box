package main

import (
	"encoding/json"
	"testing"

	"eucli-box/pkg/types"
)

func TestStreamEnabledMirrorRoundTrip(t *testing.T) {
	uiChat := map[string]any{
		"id":            "c-1",
		"title":         "会话",
		"createdAt":     float64(1787825609380),
		"updatedAt":     float64(1787825614382),
		"streamEnabled": false,
	}

	session := fromUIChat(uiChat, "developer")
	metadata := stringMapOf(session["metadata"])
	if metadata["streamEnabled"] != "false" {
		t.Fatalf("upward metadata streamEnabled = %#v", metadata["streamEnabled"])
	}
	if err := json.Unmarshal(mustJSON(session), &types.Session{}); err != nil {
		t.Fatalf("mirror body must decode into types.Session: %v", err)
	}

	back := toUIChat(stringMapOf(session))
	if back["streamEnabled"] != false {
		t.Fatalf("downward chat streamEnabled = %#v", back["streamEnabled"])
	}

	streamingChat := map[string]any{
		"id":            "c-2",
		"title":         "会话",
		"createdAt":     float64(1),
		"updatedAt":     float64(2),
		"streamEnabled": true,
	}
	streamingSession := fromUIChat(streamingChat, "developer")
	if stringMapOf(streamingSession["metadata"])["streamEnabled"] != "true" {
		t.Fatalf("explicit streaming should persist as true: %#v", streamingSession["metadata"])
	}
	if _, exists := stringMapOf(streamingSession["metadata"])["streamEnabled"]; !exists {
		t.Fatalf("metadata lost")
	}

	defaultChat := map[string]any{"id": "c-3", "title": "会话", "createdAt": float64(1), "updatedAt": float64(2)}
	defaultBack := toUIChat(fromUIChat(defaultChat, "developer"))
	if value, exists := defaultBack["streamEnabled"]; exists {
		t.Fatalf("default chat must stay implicit, got %#v", value)
	}
}

func stringMapOf(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
}
