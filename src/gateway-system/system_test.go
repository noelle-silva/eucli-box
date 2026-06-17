package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"eucli-box/pkg/types"
)

func TestStartRunRoute(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)
	req := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(`{"roleId":"developer","message":"hello"}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.runtime.started.Message != "hello" {
		t.Fatalf("started = %#v", fakes.runtime.started)
	}
}

func TestStartRunRouteAcceptsAttachmentsOnlyMessage(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)
	req := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(`{"roleId":"developer","attachments":[{"kind":"txt","name":"note.txt","text":"hello","fullLen":5,"sendLen":5,"sendPct":100}]}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.runtime.started.Message != "" || len(fakes.runtime.started.Attachments) != 1 || fakes.runtime.started.Attachments[0].Name != "note.txt" {
		t.Fatalf("started = %#v", fakes.runtime.started)
	}
}

func TestStartRunRouteAcceptsUserMessageID(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)
	req := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(`{"roleId":"developer","sessionId":"session-1","userMessageId":"u1"}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.runtime.started.UserMessageID != "u1" || fakes.runtime.started.Message != "" || fakes.runtime.started.SessionID != "session-1" {
		t.Fatalf("started = %#v", fakes.runtime.started)
	}
}

func TestStartRunRouteAcceptsContextMessageID(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)
	req := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(`{"roleId":"developer","sessionId":"session-1","contextMessageId":"a1"}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.runtime.started.ContextMessageID != "a1" || fakes.runtime.started.Message != "" || fakes.runtime.started.SessionID != "session-1" {
		t.Fatalf("started = %#v", fakes.runtime.started)
	}
}

func TestStartRunRouteAcceptsParentMessageID(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)
	req := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(`{"roleId":"developer","sessionId":"session-1","message":"hello","parentMessageId":"a1"}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.runtime.started.Message != "hello" || fakes.runtime.started.ParentMessageID != "a1" || fakes.runtime.started.UserMessageID != "" {
		t.Fatalf("started = %#v", fakes.runtime.started)
	}
}

func TestStartRunRouteAcceptsModelOverride(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)
	req := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(`{"roleId":"developer","message":"hello","modelOverride":{"kind":"provider","providerId":"anthropic-main","modelId":"claude-sonnet-4"}}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.runtime.started.ModelOverride == nil || fakes.runtime.started.ModelOverride.ProviderID != "anthropic-main" || fakes.runtime.started.ModelOverride.ModelID != "claude-sonnet-4" {
		t.Fatalf("started = %#v", fakes.runtime.started)
	}
}

func TestStartRunRouteRejectsAmbiguousRunTarget(t *testing.T) {
	system := newTestGateway(t, newGatewayFakes())
	req := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(`{"roleId":"developer","sessionId":"session-1","message":"hello","userMessageId":"u1"}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStartRunRouteRejectsInvalidModelOverride(t *testing.T) {
	system := newTestGateway(t, newGatewayFakes())
	req := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(`{"roleId":"developer","message":"hello","modelOverride":{"kind":"provider","modelId":"claude-sonnet-4"}}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateSessionRoute(t *testing.T) {
	fakes := newGatewayFakes()
	if err := fakes.roles.SaveRole(context.Background(), types.Role{ID: "developer", Name: "Developer"}); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}
	system := newTestGateway(t, fakes)
	req := httptest.NewRequest(http.MethodPost, "/api/roles/developer/sessions/create", strings.NewReader(`{"title":"Fresh chat"}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	session := decodeResponseData[types.Session](t, rec.Body.String())
	if session.ID == "" || session.RoleID != "developer" || session.Title != "Fresh chat" {
		t.Fatalf("session = %#v", session)
	}
	if _, ok := fakes.sessions.sessions["developer/"+session.ID]; !ok {
		t.Fatalf("session was not stored: %#v", fakes.sessions.sessions)
	}
}

func TestSessionMessageRoutes(t *testing.T) {
	fakes := newGatewayFakes()
	now := time.Now().UTC()
	fakes.sessions.sessions["developer/session-1"] = types.Session{
		ID:        "session-1",
		RoleID:    "developer",
		Title:     "Old title",
		Status:    string(types.RunStatusCreated),
		CreatedAt: now,
		UpdatedAt: now,
		Messages: []types.Message{
			{ID: "m1", Type: "user", Content: "hello", BranchID: "main", CreatedAt: now, UpdatedAt: now},
			{ID: "m2", Type: "assistant", Content: "hi", ParentMessageID: "m1", BranchID: "main", CreatedAt: now, UpdatedAt: now},
		},
		LastActive: now,
	}
	system := newTestGateway(t, fakes)

	req := httptest.NewRequest(http.MethodPatch, "/api/roles/developer/sessions/session-1/title", strings.NewReader(`{"title":"New title"}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("title status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := fakes.sessions.sessions["developer/session-1"].Title; got != "New title" {
		t.Fatalf("title = %q", got)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/roles/developer/sessions/session-1/messages/m1", strings.NewReader(`{"content":"updated"}`))
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("message status = %d body=%s", rec.Code, rec.Body.String())
	}
	updatedMessage := decodeResponseData[types.Message](t, rec.Body.String())
	if updatedMessage.ID != "m1" || updatedMessage.Content != "updated" {
		t.Fatalf("updated message = %#v", updatedMessage)
	}
	if got := fakes.sessions.sessions["developer/session-1"].Messages[0].Content; got != "updated" {
		t.Fatalf("message content = %q", got)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/roles/developer/sessions/session-1/messages/m2", strings.NewReader(`{"content":"hi","parts":[{"id":"part-tool-1","type":"tool","callId":"call-1","toolName":"shell_command","state":"completed","input":{"command":"pwd"},"display":{"hideResult":true},"createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-01-02T03:04:06Z","decision":{"id":"decision-1","actionId":"call-1","toolName":"shell_command","status":"allowed","createdAt":"2026-01-02T03:04:05Z"},"result":{"id":"result-1","actionId":"call-1","toolName":"shell_command","status":"success","content":"ok","createdAt":"2026-01-02T03:04:06Z"}}]}`))
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("parts message status = %d body=%s", rec.Code, rec.Body.String())
	}
	updatedMessage = decodeResponseData[types.Message](t, rec.Body.String())
	if updatedMessage.ID != "m2" || len(updatedMessage.Parts) != 1 || updatedMessage.Parts[0].Display["hideResult"] != true || updatedMessage.Parts[0].CreatedAt.IsZero() || updatedMessage.Parts[0].Result == nil || updatedMessage.Parts[0].Result.CreatedAt.IsZero() {
		t.Fatalf("updated message parts = %#v", updatedMessage)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/roles/developer/sessions/session-1/messages/m1", nil)
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(fakes.sessions.sessions["developer/session-1"].Messages) != 1 {
		t.Fatalf("messages = %#v", fakes.sessions.sessions["developer/session-1"].Messages)
	}
}

func TestSessionFavoritesRoutes(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)

	req := httptest.NewRequest(http.MethodPut, "/api/sessions/favorites", strings.NewReader(`{"folders":[{"id":"favf-1","name":"Important","parentId":"","createdAt":1,"updatedAt":1}],"chatRefsByFolderId":{"favf-1":[{"targetKind":"role","targetId":"developer","chatId":"session-1","addedAt":2}]}}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save favorites status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(fakes.sessions.favorites.Folders) != 1 || fakes.sessions.favorites.ChatRefsByFolderID["favf-1"][0].ChatID != "session-1" {
		t.Fatalf("favorites = %#v", fakes.sessions.favorites)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/sessions/favorites", nil)
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "favf-1") {
		t.Fatalf("load favorites status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHookPromptLibraryRoutes(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)

	req := httptest.NewRequest(http.MethodPut, "/api/hook-prompts", strings.NewReader(`{"presets":[{"id":"preset-1","name":"审查助手","messages":[{"id":"msg-1","role":"system","position":"session_top","content":"请严格审查","order":0}]}]}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save hook prompts status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(fakes.hooks.library.Presets) != 1 || fakes.hooks.library.Presets[0].ID != "preset-1" {
		t.Fatalf("hook library = %#v", fakes.hooks.library)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/hook-prompts", nil)
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "审查助手") {
		t.Fatalf("load hook prompts status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSessionHookPromptRoute(t *testing.T) {
	fakes := newGatewayFakes()
	now := time.Now().UTC()
	fakes.sessions.sessions["developer/session-1"] = types.Session{ID: "session-1", RoleID: "developer", Title: "Chat", Status: string(types.RunStatusCreated), CreatedAt: now, UpdatedAt: now, LastActive: now}
	system := newTestGateway(t, fakes)

	req := httptest.NewRequest(http.MethodPatch, "/api/roles/developer/sessions/session-1/hook-prompt", strings.NewReader(`{"presetId":"preset-1"}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set hook prompt status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := fakes.sessions.sessions["developer/session-1"].Metadata[types.SessionMetadataHookPromptPresetID]; got != "preset-1" {
		t.Fatalf("metadata preset = %q", got)
	}
	if got := fakes.sessions.sessions["developer/session-1"].Metadata[types.SessionMetadataHookPromptMode]; got != types.HookPromptSelectionModePreset {
		t.Fatalf("metadata mode = %q", got)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/roles/developer/sessions/session-1/hook-prompt", strings.NewReader(`{"presetId":""}`))
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear hook prompt status = %d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := fakes.sessions.sessions["developer/session-1"].Metadata[types.SessionMetadataHookPromptPresetID]; ok {
		t.Fatalf("metadata was not cleared: %#v", fakes.sessions.sessions["developer/session-1"].Metadata)
	}
	if got := fakes.sessions.sessions["developer/session-1"].Metadata[types.SessionMetadataHookPromptMode]; got != types.HookPromptSelectionModeNone {
		t.Fatalf("metadata mode = %q", got)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/roles/developer/sessions/session-1/hook-prompt", strings.NewReader(`{"mode":"inherit"}`))
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("inherit hook prompt status = %d body=%s", rec.Code, rec.Body.String())
	}
	if metadata := fakes.sessions.sessions["developer/session-1"].Metadata; len(metadata) != 0 {
		t.Fatalf("metadata was not reset to inherit: %#v", metadata)
	}
}

func TestRoleRoutes(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)
	req := httptest.NewRequest(http.MethodPost, "/api/roles", strings.NewReader(`{"id":"developer","name":"Developer"}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/roles/developer", nil)
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Developer") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInvalidJSONReturnsBadRequest(t *testing.T) {
	system := newTestGateway(t, newGatewayFakes())
	req := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(`{"roleId":"developer","extra":true}`))
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProviderAndToolRoutes(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)
	providerReq := httptest.NewRequest(http.MethodPost, "/api/providers", strings.NewReader(`{"id":"openai-main","name":"OpenAI","baseUrl":"https://api.test/v1","key":"secret","protocol":"openai"}`))
	providerRec := httptest.NewRecorder()
	system.Handler().ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusNoContent {
		t.Fatalf("provider status = %d body=%s", providerRec.Code, providerRec.Body.String())
	}
	toolReq := httptest.NewRequest(http.MethodPost, "/api/tools", strings.NewReader(`{"id":"file-reader","name":"file-reader","description":"Read","type":"local"}`))
	toolRec := httptest.NewRecorder()
	system.Handler().ServeHTTP(toolRec, toolReq)
	if toolRec.Code != http.StatusNoContent {
		t.Fatalf("tool status = %d body=%s", toolRec.Code, toolRec.Body.String())
	}
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/providers/openai-main/models/refresh", nil)
	refreshRec := httptest.NewRecorder()
	system.Handler().ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusOK || !strings.Contains(refreshRec.Body.String(), "gpt-4.1") {
		t.Fatalf("refresh status = %d body=%s", refreshRec.Code, refreshRec.Body.String())
	}
	configReq := httptest.NewRequest(http.MethodPut, "/api/tools/file-reader/user-config", strings.NewReader(`{"userConfig":{"limit":20},"promptDescriptionOverride":"Read files only when requested"}`))
	configRec := httptest.NewRecorder()
	system.Handler().ServeHTTP(configRec, configReq)
	if configRec.Code != http.StatusOK || !strings.Contains(configRec.Body.String(), "\"limit\":20") || !strings.Contains(configRec.Body.String(), "Read files only when requested") {
		t.Fatalf("tool config status = %d body=%s", configRec.Code, configRec.Body.String())
	}

	modelRequestConfigReq := httptest.NewRequest(http.MethodPut, "/api/providers/model-request-config", strings.NewReader(`{"listModelsTimeoutMs":30000,"completionTimeoutMs":180000,"streamIdleTimeoutMs":90000}`))
	modelRequestConfigRec := httptest.NewRecorder()
	system.Handler().ServeHTTP(modelRequestConfigRec, modelRequestConfigReq)
	if modelRequestConfigRec.Code != http.StatusOK || !strings.Contains(modelRequestConfigRec.Body.String(), "streamIdleTimeoutMs") {
		t.Fatalf("model request config save status = %d body=%s", modelRequestConfigRec.Code, modelRequestConfigRec.Body.String())
	}

	modelRequestConfigReq = httptest.NewRequest(http.MethodGet, "/api/providers/model-request-config", nil)
	modelRequestConfigRec = httptest.NewRecorder()
	system.Handler().ServeHTTP(modelRequestConfigRec, modelRequestConfigReq)
	if modelRequestConfigRec.Code != http.StatusOK || !strings.Contains(modelRequestConfigRec.Body.String(), "180000") {
		t.Fatalf("model request config load status = %d body=%s", modelRequestConfigRec.Code, modelRequestConfigRec.Body.String())
	}
}

func TestStickerRoutes(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)

	req := httptest.NewRequest(http.MethodGet, "/api/assist/stickers/name/config", nil)
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "openai-main") {
		t.Fatalf("load config status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/api/assist/stickers/name/config", strings.NewReader(`{"enabled":true,"coordinate":{"providerId":"anthropic-main","modelId":"claude-3-5-sonnet"},"systemPrompt":"test","temperature":0.3}`))
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "anthropic-main") {
		t.Fatalf("save config status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/stickers/categories", strings.NewReader(`{"categoryName":"通用"}`))
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create category status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/stickers/items", strings.NewReader(`{"categoryName":"通用","stickerName":"开心","dataUrl":"data:image/png;base64,ok"}`))
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add sticker status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/stickers", nil)
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "开心") {
		t.Fatalf("library status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/assist/stickers/name", strings.NewReader(`{"categoryName":"通用","stickerName":"开心"}`))
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "AI 命名") {
		t.Fatalf("assist status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAssistRoutes(t *testing.T) {
	fakes := newGatewayFakes()
	fakes.sessions.sessions["developer/session-1"] = types.Session{
		ID:         "session-1",
		RoleID:     "developer",
		Title:      "旧标题",
		Status:     string(types.RunStatusCreated),
		Messages:   []types.Message{{ID: "m1", Type: "assistant", Content: "```mermaid\ngraph TD\nA-->C\n```", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}},
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
		LastActive: time.Now().UTC(),
	}
	system := newTestGateway(t, fakes)

	req := httptest.NewRequest(http.MethodGet, "/api/assist/mermaid-fix/config", nil)
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "openai-main") {
		t.Fatalf("mermaid config status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/assist/chat-title", strings.NewReader(`{"roleId":"developer","sessionId":"session-1"}`))
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "AI 标题") {
		t.Fatalf("chat title status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/assist/mermaid-fix", strings.NewReader(`{"roleId":"developer","sessionId":"session-1","messageId":"m1","mermaidSource":"graph TD\nA-->C"}`))
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "graph TD") {
		t.Fatalf("mermaid fix status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWebSocketForwardsRuntimeEvents(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)
	server := httptest.NewServer(system.Handler())
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/events"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	fakes.runtime.waitSubscribers(t, 1)
	fakes.runtime.publish(types.RunEvent{ID: "e1", RunID: "run-1", Type: "run_started", CreatedAt: time.Now().UTC()})
	var event types.RunEvent
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	if event.Type != "run_started" {
		t.Fatalf("event = %#v", event)
	}
}

func newTestGateway(t *testing.T, fakes *gatewayFakes) System {
	t.Helper()
	system, err := NewSystem(Config{}, fakes.runtime, fakes.roles, fakes.groups, fakes.workspaces, fakes.providers, fakes.tools, fakes.sessions, fakes.stickers, fakes.hooks, fakes.assist)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	return system
}

type gatewayFakes struct {
	runtime    *fakeGatewayRuntime
	roles      *fakeGatewayRoles
	groups     *fakeGatewayGroups
	workspaces *fakeGatewayWorkspaces
	providers  *fakeGatewayProviders
	tools      *fakeGatewayTools
	sessions   *fakeGatewaySessions
	stickers   *fakeGatewayStickers
	hooks      *fakeGatewayHooks
	assist     *fakeGatewayAssist
}

func newGatewayFakes() *gatewayFakes {
	stickers := newFakeGatewayStickers()
	return &gatewayFakes{runtime: newFakeGatewayRuntime(), roles: newFakeGatewayRoles(), groups: newFakeGatewayGroups(), workspaces: newFakeGatewayWorkspaces(), providers: newFakeGatewayProviders(), tools: newFakeGatewayTools(), sessions: newFakeGatewaySessions(), stickers: stickers, hooks: &fakeGatewayHooks{}, assist: &fakeGatewayAssist{stickers: stickers}}
}

type fakeGatewayHooks struct {
	library types.HookPromptLibrary
}

func (f *fakeGatewayHooks) LoadHookPromptLibrary(ctx context.Context) (types.HookPromptLibrary, error) {
	return f.library, nil
}

func (f *fakeGatewayHooks) SaveHookPromptLibrary(ctx context.Context, library types.HookPromptLibrary) (types.HookPromptLibrary, error) {
	f.library = library
	return f.library, nil
}

type fakeGatewayWorkspaces struct {
	workspaces map[string]types.Workspace
}

func newFakeGatewayWorkspaces() *fakeGatewayWorkspaces {
	return &fakeGatewayWorkspaces{workspaces: map[string]types.Workspace{}}
}

func (f *fakeGatewayWorkspaces) SaveWorkspace(ctx context.Context, workspace types.Workspace) error {
	f.workspaces[workspace.ID] = workspace
	return nil
}

func (f *fakeGatewayWorkspaces) LoadWorkspace(ctx context.Context, workspaceID string) (types.Workspace, error) {
	workspace, ok := f.workspaces[workspaceID]
	if !ok {
		return types.Workspace{}, errors.New("workspace missing")
	}
	return workspace, nil
}

func (f *fakeGatewayWorkspaces) ListWorkspaces(ctx context.Context) ([]types.WorkspaceSummary, error) {
	out := make([]types.WorkspaceSummary, 0, len(f.workspaces))
	for _, workspace := range f.workspaces {
		out = append(out, types.WorkspaceSummary{ID: workspace.ID, Name: workspace.Name, UpdatedAt: workspace.UpdatedAt})
	}
	return out, nil
}

func (f *fakeGatewayWorkspaces) DeleteWorkspace(ctx context.Context, workspaceID string) error {
	delete(f.workspaces, workspaceID)
	return nil
}

type fakeGatewaySessions struct {
	sessions  map[string]types.Session
	favorites types.SessionFavorites
}

func newFakeGatewaySessions() *fakeGatewaySessions {
	return &fakeGatewaySessions{sessions: map[string]types.Session{}, favorites: types.SessionFavorites{Folders: []types.SessionFavoriteFolder{}, ChatRefsByFolderID: map[string][]types.SessionFavoriteChatRef{}}}
}

func (f *fakeGatewaySessions) CreateSession(ctx context.Context, roleID string, title string) (types.Session, error) {
	now := time.Now().UTC()
	session := types.Session{ID: "session-created", RoleID: roleID, Title: title, Status: string(types.RunStatusCreated), Messages: []types.Message{}, CreatedAt: now, LastActive: now}
	f.sessions[f.sessionKey(session)] = session
	return session, nil
}

func (f *fakeGatewaySessions) CreateGroupSession(ctx context.Context, groupID string, title string) (types.Session, error) {
	now := time.Now().UTC()
	session := types.Session{ID: "session-created", GroupID: groupID, Title: title, Status: string(types.RunStatusCreated), Messages: []types.Message{}, CreatedAt: now, LastActive: now}
	f.sessions[f.sessionKey(session)] = session
	return session, nil
}

func (f *fakeGatewaySessions) CreateWorkspaceSession(ctx context.Context, workspaceID string, roleID string, title string) (types.Session, error) {
	now := time.Now().UTC()
	session := types.Session{ID: "session-created", RoleID: roleID, WorkspaceID: workspaceID, Title: title, Status: string(types.RunStatusCreated), Messages: []types.Message{}, CreatedAt: now, LastActive: now}
	f.sessions[f.sessionKey(session)] = session
	return session, nil
}

func (f *fakeGatewaySessions) SaveSession(ctx context.Context, session types.Session) error {
	f.sessions[f.sessionKey(session)] = session
	return nil
}

func (f *fakeGatewaySessions) LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error) {
	return f.sessions[roleID+"/"+sessionID], nil
}

func (f *fakeGatewaySessions) LoadGroupSession(ctx context.Context, groupID string, sessionID string) (types.Session, error) {
	return f.sessions["groups/"+groupID+"/"+sessionID], nil
}

func (f *fakeGatewaySessions) LoadWorkspaceSession(ctx context.Context, workspaceID string, roleID string, sessionID string) (types.Session, error) {
	return f.sessions["workspaces/"+workspaceID+"/"+roleID+"/"+sessionID], nil
}

func (f *fakeGatewaySessions) ListSessions(ctx context.Context, roleID string) ([]types.SessionSummary, error) {
	out := []types.SessionSummary{}
	for _, s := range f.sessions {
		if s.RoleID != roleID {
			continue
		}
		out = append(out, types.SessionSummary{ID: s.ID, RoleID: s.RoleID, Title: s.Title, Status: s.Status, LastActive: s.LastActive})
	}
	return out, nil
}

func (f *fakeGatewaySessions) ListGroupSessions(ctx context.Context, groupID string) ([]types.SessionSummary, error) {
	out := []types.SessionSummary{}
	for _, s := range f.sessions {
		if s.GroupID != groupID {
			continue
		}
		out = append(out, types.SessionSummary{ID: s.ID, GroupID: s.GroupID, Title: s.Title, Status: s.Status, LastActive: s.LastActive})
	}
	return out, nil
}

func (f *fakeGatewaySessions) ListWorkspaceSessions(ctx context.Context, workspaceID string, roleID string) ([]types.SessionSummary, error) {
	out := []types.SessionSummary{}
	for _, s := range f.sessions {
		if s.WorkspaceID != workspaceID || s.RoleID != roleID {
			continue
		}
		out = append(out, types.SessionSummary{ID: s.ID, RoleID: s.RoleID, WorkspaceID: s.WorkspaceID, Title: s.Title, Status: s.Status, LastActive: s.LastActive})
	}
	return out, nil
}

func (f *fakeGatewaySessions) DeleteSession(ctx context.Context, roleID string, sessionID string) error {
	delete(f.sessions, roleID+"/"+sessionID)
	return nil
}

func (f *fakeGatewaySessions) DeleteGroupSession(ctx context.Context, groupID string, sessionID string) error {
	delete(f.sessions, "groups/"+groupID+"/"+sessionID)
	return nil
}

func (f *fakeGatewaySessions) DeleteWorkspaceSession(ctx context.Context, workspaceID string, roleID string, sessionID string) error {
	delete(f.sessions, "workspaces/"+workspaceID+"/"+roleID+"/"+sessionID)
	return nil
}

func (f *fakeGatewaySessions) UpdateSessionTitle(ctx context.Context, roleID string, sessionID string, title string) (types.Session, error) {
	session := f.sessions[roleID+"/"+sessionID]
	session.Title = title
	session.UpdatedAt = time.Now().UTC()
	f.sessions[roleID+"/"+sessionID] = session
	return session, nil
}

func (f *fakeGatewaySessions) UpdateGroupSessionTitle(ctx context.Context, groupID string, sessionID string, title string) (types.Session, error) {
	session := f.sessions["groups/"+groupID+"/"+sessionID]
	session.Title = title
	session.UpdatedAt = time.Now().UTC()
	f.sessions["groups/"+groupID+"/"+sessionID] = session
	return session, nil
}

func (f *fakeGatewaySessions) UpdateWorkspaceSessionTitle(ctx context.Context, workspaceID string, roleID string, sessionID string, title string) (types.Session, error) {
	session := f.sessions["workspaces/"+workspaceID+"/"+roleID+"/"+sessionID]
	session.Title = title
	session.UpdatedAt = time.Now().UTC()
	f.sessions["workspaces/"+workspaceID+"/"+roleID+"/"+sessionID] = session
	return session, nil
}

func (f *fakeGatewaySessions) UpdateSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string, patch types.SessionMessagePatch) (types.Message, error) {
	session := f.sessions[roleID+"/"+sessionID]
	updated := types.Message{}
	for i := range session.Messages {
		if session.Messages[i].ID == messageID {
			if patch.Content != nil {
				session.Messages[i].Content = *patch.Content
			}
			if patch.Parts != nil {
				session.Messages[i].Parts = append([]types.MessagePart(nil), (*patch.Parts)...)
			}
			session.Messages[i].UpdatedAt = time.Now().UTC()
			updated = session.Messages[i]
		}
	}
	f.sessions[roleID+"/"+sessionID] = session
	return updated, nil
}

func (f *fakeGatewaySessions) UpdateGroupSessionMessage(ctx context.Context, groupID string, sessionID string, messageID string, patch types.SessionMessagePatch) (types.Message, error) {
	return f.updateSessionMessageByKey("groups/"+groupID+"/"+sessionID, messageID, patch)
}

func (f *fakeGatewaySessions) UpdateWorkspaceSessionMessage(ctx context.Context, workspaceID string, roleID string, sessionID string, messageID string, patch types.SessionMessagePatch) (types.Message, error) {
	return f.updateSessionMessageByKey("workspaces/"+workspaceID+"/"+roleID+"/"+sessionID, messageID, patch)
}

func (f *fakeGatewaySessions) DeleteSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string) (types.Session, error) {
	session := f.sessions[roleID+"/"+sessionID]
	next := make([]types.Message, 0, len(session.Messages))
	for _, message := range session.Messages {
		if message.ID != messageID {
			next = append(next, message)
		}
	}
	session.Messages = next
	f.sessions[roleID+"/"+sessionID] = session
	return session, nil
}

func (f *fakeGatewaySessions) DeleteGroupSessionMessage(ctx context.Context, groupID string, sessionID string, messageID string) (types.Session, error) {
	return f.deleteSessionMessageByKey("groups/"+groupID+"/"+sessionID, messageID)
}

func (f *fakeGatewaySessions) DeleteWorkspaceSessionMessage(ctx context.Context, workspaceID string, roleID string, sessionID string, messageID string) (types.Session, error) {
	return f.deleteSessionMessageByKey("workspaces/"+workspaceID+"/"+roleID+"/"+sessionID, messageID)
}

func (f *fakeGatewaySessions) DeleteSessionMessageSubtree(ctx context.Context, roleID string, sessionID string, messageID string) (types.Session, error) {
	return f.DeleteSessionMessage(ctx, roleID, sessionID, messageID)
}

func (f *fakeGatewaySessions) DeleteGroupSessionMessageSubtree(ctx context.Context, groupID string, sessionID string, messageID string) (types.Session, error) {
	return f.DeleteGroupSessionMessage(ctx, groupID, sessionID, messageID)
}

func (f *fakeGatewaySessions) DeleteWorkspaceSessionMessageSubtree(ctx context.Context, workspaceID string, roleID string, sessionID string, messageID string) (types.Session, error) {
	return f.DeleteWorkspaceSessionMessage(ctx, workspaceID, roleID, sessionID, messageID)
}

func (f *fakeGatewaySessions) sessionKey(session types.Session) string {
	if strings.TrimSpace(session.GroupID) != "" {
		return "groups/" + session.GroupID + "/" + session.ID
	}
	if strings.TrimSpace(session.WorkspaceID) != "" {
		return "workspaces/" + session.WorkspaceID + "/" + session.RoleID + "/" + session.ID
	}
	return session.RoleID + "/" + session.ID
}

func (f *fakeGatewaySessions) updateSessionMessageByKey(key string, messageID string, patch types.SessionMessagePatch) (types.Message, error) {
	session := f.sessions[key]
	updated := types.Message{}
	for i := range session.Messages {
		if session.Messages[i].ID == messageID {
			if patch.Content != nil {
				session.Messages[i].Content = *patch.Content
			}
			if patch.Parts != nil {
				session.Messages[i].Parts = append([]types.MessagePart(nil), (*patch.Parts)...)
			}
			session.Messages[i].UpdatedAt = time.Now().UTC()
			updated = session.Messages[i]
		}
	}
	f.sessions[key] = session
	return updated, nil
}

func (f *fakeGatewaySessions) deleteSessionMessageByKey(key string, messageID string) (types.Session, error) {
	session := f.sessions[key]
	next := make([]types.Message, 0, len(session.Messages))
	for _, message := range session.Messages {
		if message.ID != messageID {
			next = append(next, message)
		}
	}
	session.Messages = next
	f.sessions[key] = session
	return session, nil
}

func (f *fakeGatewaySessions) LoadSessionAttachmentImage(ctx context.Context, relPath string) (string, error) {
	if strings.TrimSpace(relPath) == "" {
		return "", errors.New("path missing")
	}
	return "data:image/png;base64,iVBORw0KGgo=", nil
}

func (f *fakeGatewaySessions) LoadSessionFavorites(ctx context.Context) (types.SessionFavorites, error) {
	return f.favorites, nil
}

func (f *fakeGatewaySessions) SaveSessionFavorites(ctx context.Context, favorites types.SessionFavorites) (types.SessionFavorites, error) {
	f.favorites = favorites
	return f.favorites, nil
}

type fakeGatewayRuntime struct {
	mu          sync.Mutex
	started     types.RunRequest
	runs        map[string]types.RunState
	subscribers []chan types.RunEvent
}

func newFakeGatewayRuntime() *fakeGatewayRuntime {
	return &fakeGatewayRuntime{runs: map[string]types.RunState{"run-1": {ID: "run-1", Status: types.RunStatusRunning}}}
}

func (f *fakeGatewayRuntime) StartRun(ctx context.Context, request types.RunRequest) (types.RunState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = request
	state := types.RunState{ID: "run-1", RoleID: request.RoleID, Status: types.RunStatusCreated}
	f.runs[state.ID] = state
	return state, nil
}

func (f *fakeGatewayRuntime) SubmitToolConfirmation(ctx context.Context, confirmation types.ToolConfirmation) error {
	return nil
}
func (f *fakeGatewayRuntime) CancelRun(ctx context.Context, runID string) error { return nil }

func (f *fakeGatewayRuntime) GetRun(ctx context.Context, runID string) (types.RunState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, ok := f.runs[runID]
	if !ok {
		return types.RunState{}, errors.New("missing")
	}
	return state, nil
}

func (f *fakeGatewayRuntime) ListActiveRuns(ctx context.Context) ([]types.RunState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	runs := make([]types.RunState, 0, len(f.runs))
	for _, state := range f.runs {
		if state.Status == types.RunStatusCreated || state.Status == types.RunStatusRunning || state.Status == types.RunStatusWaitingConfirmation {
			runs = append(runs, state)
		}
	}
	return runs, nil
}

func (f *fakeGatewayRuntime) Subscribe(ctx context.Context) (<-chan types.RunEvent, func(), error) {
	ch := make(chan types.RunEvent, 16)
	f.mu.Lock()
	f.subscribers = append(f.subscribers, ch)
	f.mu.Unlock()
	unsubscribe := func() { close(ch) }
	return ch, unsubscribe, nil
}

func (f *fakeGatewayRuntime) waitSubscribers(t *testing.T, count int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		current := len(f.subscribers)
		f.mu.Unlock()
		if current >= count {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("runtime subscriber count did not reach %d", count)
}

func (f *fakeGatewayRuntime) publish(event types.RunEvent) {
	f.mu.Lock()
	subscribers := append([]chan types.RunEvent(nil), f.subscribers...)
	f.mu.Unlock()
	for _, ch := range subscribers {
		ch <- event
	}
}

type fakeGatewayRoles struct {
	roles   map[string]types.Role
	avatars map[string]string
}

func newFakeGatewayRoles() *fakeGatewayRoles {
	return &fakeGatewayRoles{roles: map[string]types.Role{}, avatars: map[string]string{}}
}
func (f *fakeGatewayRoles) SaveRole(ctx context.Context, role types.Role) error {
	f.roles[role.ID] = role
	return nil
}
func (f *fakeGatewayRoles) LoadRole(ctx context.Context, roleID string) (types.Role, error) {
	return f.roles[roleID], nil
}
func (f *fakeGatewayRoles) ListRoles(ctx context.Context) ([]types.RoleSummary, error) {
	roles := make([]types.RoleSummary, 0, len(f.roles))
	for _, role := range f.roles {
		roles = append(roles, types.RoleSummary{ID: role.ID, Name: role.Name})
	}
	return roles, nil
}
func (f *fakeGatewayRoles) DeleteRole(ctx context.Context, roleID string) error {
	delete(f.roles, roleID)
	delete(f.avatars, roleID)
	return nil
}
func (f *fakeGatewayRoles) SaveRoleAvatar(ctx context.Context, roleID string, dataURL string) error {
	f.avatars[roleID] = dataURL
	return nil
}
func (f *fakeGatewayRoles) LoadRoleAvatar(ctx context.Context, roleID string) (string, error) {
	return f.avatars[roleID], nil
}
func (f *fakeGatewayRoles) DeleteRoleAvatar(ctx context.Context, roleID string) error {
	delete(f.avatars, roleID)
	return nil
}

type fakeGatewayGroups struct {
	groups  map[string]types.ChatGroup
	avatars map[string]string
}

func newFakeGatewayGroups() *fakeGatewayGroups {
	return &fakeGatewayGroups{groups: map[string]types.ChatGroup{}, avatars: map[string]string{}}
}

func (f *fakeGatewayGroups) SaveChatGroup(ctx context.Context, group types.ChatGroup) error {
	f.groups[group.ID] = group
	return nil
}

func (f *fakeGatewayGroups) LoadChatGroup(ctx context.Context, groupID string) (types.ChatGroup, error) {
	group, ok := f.groups[groupID]
	if !ok {
		return types.ChatGroup{}, errors.New("group missing")
	}
	return group, nil
}

func (f *fakeGatewayGroups) ListChatGroups(ctx context.Context) ([]types.ChatGroupSummary, error) {
	groups := make([]types.ChatGroupSummary, 0, len(f.groups))
	for _, group := range f.groups {
		groups = append(groups, types.ChatGroupSummary{ID: group.ID, Name: group.Name, Avatar: group.Avatar, UpdatedAt: group.UpdatedAt})
	}
	return groups, nil
}

func (f *fakeGatewayGroups) DeleteChatGroup(ctx context.Context, groupID string) error {
	delete(f.groups, groupID)
	delete(f.avatars, groupID)
	return nil
}

func (f *fakeGatewayGroups) SaveChatGroupAvatar(ctx context.Context, groupID string, dataURL string) error {
	f.avatars[groupID] = dataURL
	return nil
}

func (f *fakeGatewayGroups) LoadChatGroupAvatar(ctx context.Context, groupID string) (string, error) {
	return f.avatars[groupID], nil
}

func (f *fakeGatewayGroups) DeleteChatGroupAvatar(ctx context.Context, groupID string) error {
	delete(f.avatars, groupID)
	return nil
}

type fakeGatewayProviders struct {
	providers          map[string]types.Provider
	modelGroups        []types.ModelGroup
	modelRequestConfig types.ModelRequestConfig
}

func newFakeGatewayProviders() *fakeGatewayProviders {
	return &fakeGatewayProviders{providers: map[string]types.Provider{}}
}
func (f *fakeGatewayProviders) SaveProvider(ctx context.Context, provider types.Provider) error {
	f.providers[provider.ID] = provider
	return nil
}
func (f *fakeGatewayProviders) LoadProvider(ctx context.Context, providerID string) (types.Provider, error) {
	return f.providers[providerID], nil
}
func (f *fakeGatewayProviders) ListProviders(ctx context.Context) ([]types.ProviderSummary, error) {
	providers := make([]types.ProviderSummary, 0, len(f.providers))
	for _, provider := range f.providers {
		apiKeyCount := len(provider.APIKeys)
		enabledAPIKeyCount := 0
		for _, key := range provider.APIKeys {
			if key.Enabled {
				enabledAPIKeyCount++
			}
		}
		if apiKeyCount == 0 && provider.Key != "" {
			apiKeyCount = 1
			enabledAPIKeyCount = 1
		}
		providers = append(providers, types.ProviderSummary{ID: provider.ID, Name: provider.Name, Protocol: provider.Protocol, APIKeyCount: apiKeyCount, EnabledAPIKeyCount: enabledAPIKeyCount, RegisteredModelCount: len(provider.RegisteredModels)})
	}
	return providers, nil
}
func (f *fakeGatewayProviders) DeleteProvider(ctx context.Context, providerID string) error {
	delete(f.providers, providerID)
	return nil
}
func (f *fakeGatewayProviders) LoadModelRequestConfig(ctx context.Context) (types.ModelRequestConfig, error) {
	return f.modelRequestConfig, nil
}
func (f *fakeGatewayProviders) SaveModelRequestConfig(ctx context.Context, config types.ModelRequestConfig) (types.ModelRequestConfig, error) {
	f.modelRequestConfig = config
	return f.modelRequestConfig, nil
}
func (f *fakeGatewayProviders) LoadModelGroups(ctx context.Context) ([]types.ModelGroup, error) {
	return f.modelGroups, nil
}
func (f *fakeGatewayProviders) SaveModelGroups(ctx context.Context, groups []types.ModelGroup) ([]types.ModelGroup, error) {
	f.modelGroups = groups
	return f.modelGroups, nil
}
func (f *fakeGatewayProviders) RefreshModels(ctx context.Context, providerID string) ([]types.ModelInfo, error) {
	return []types.ModelInfo{{ID: "gpt-4.1", Name: "GPT-4.1"}}, nil
}

type fakeGatewayTools struct {
	tools map[string]types.ToolDefinition
}

func newFakeGatewayTools() *fakeGatewayTools {
	return &fakeGatewayTools{tools: map[string]types.ToolDefinition{}}
}
func (f *fakeGatewayTools) SaveTool(ctx context.Context, tool types.ToolDefinition) error {
	f.tools[tool.ID] = tool
	return nil
}
func (f *fakeGatewayTools) LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error) {
	return f.tools[toolID], nil
}
func (f *fakeGatewayTools) ListTools(ctx context.Context) ([]types.ToolSummary, error) {
	tools := make([]types.ToolSummary, 0, len(f.tools))
	for _, tool := range f.tools {
		tools = append(tools, types.ToolSummary{ID: tool.ID, Name: tool.Name, Description: tool.Description, Type: tool.Type})
	}
	return tools, nil
}

func (f *fakeGatewayTools) SaveToolUserSettings(ctx context.Context, toolID string, settings types.ToolUserSettings) (types.ToolDefinition, error) {
	tool := f.tools[toolID]
	tool.UserConfig = settings.UserConfig
	tool.PromptDescriptionOverride = settings.PromptDescriptionOverride
	f.tools[toolID] = tool
	return tool, nil
}

type fakeGatewayStickers struct {
	categories        map[string]map[string]types.StickerItem
	images            map[string]string
	config            types.StickerNamingConfig
	mermaid           types.MermaidFixConfig
	title             types.ChatTitleNamingConfig
	compressionConfig types.ContextCompressionConfig
}

func newFakeGatewayStickers() *fakeGatewayStickers {
	now := time.Now().UTC()
	return &fakeGatewayStickers{
		categories: map[string]map[string]types.StickerItem{},
		images:     map[string]string{},
		config:     types.StickerNamingConfig{Enabled: true, Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"}, SystemPrompt: types.DefaultStickerNamingSystemPrompt, Temperature: 0.2, UpdatedAt: now},
		mermaid:    types.MermaidFixConfig{Enabled: true, Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"}, SystemPrompt: types.DefaultMermaidFixSystemPrompt, Temperature: 0.2, UpdatedAt: now},
		title:      types.ChatTitleNamingConfig{Enabled: true, Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"}, SystemPrompt: types.DefaultChatTitleNamingSystemPrompt, Temperature: 0.2, UpdatedAt: now},
		compressionConfig: types.ContextCompressionConfig{
			Coordinate:           types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"},
			RetainRecentMessages: types.DefaultContextCompressionRetainRecentMessages,
			Temperature:          0.2,
			UpdatedAt:            now,
		},
	}
}

func (f *fakeGatewayStickers) CreateStickerCategory(ctx context.Context, categoryName string) (types.StickerCategory, error) {
	name := strings.TrimSpace(categoryName)
	if name == "" {
		return types.StickerCategory{}, errors.New("category missing")
	}
	if f.categories[name] == nil {
		f.categories[name] = map[string]types.StickerItem{}
	}
	return types.StickerCategory{Name: name, Items: []types.StickerItem{}, UpdatedAt: time.Now().UTC()}, nil
}

func (f *fakeGatewayStickers) ListStickerCategories(ctx context.Context) ([]types.StickerCategorySummary, error) {
	summaries := make([]types.StickerCategorySummary, 0, len(f.categories))
	for name, items := range f.categories {
		summaries = append(summaries, types.StickerCategorySummary{Name: name, Count: len(items), UpdatedAt: time.Now().UTC()})
	}
	return summaries, nil
}

func (f *fakeGatewayStickers) LoadStickerCategory(ctx context.Context, categoryName string) (types.StickerCategory, error) {
	name := strings.TrimSpace(categoryName)
	itemsByName := f.categories[name]
	items := make([]types.StickerItem, 0, len(itemsByName))
	for _, item := range itemsByName {
		items = append(items, item)
	}
	return types.StickerCategory{Name: name, Items: items, UpdatedAt: time.Now().UTC()}, nil
}

func (f *fakeGatewayStickers) LoadStickerLibrary(ctx context.Context) (types.StickerLibrary, error) {
	summaries, _ := f.ListStickerCategories(ctx)
	library := types.StickerLibrary{Categories: summaries, Map: map[string][]types.StickerItem{}, UpdatedAt: time.Now().UTC()}
	for _, summary := range summaries {
		category, _ := f.LoadStickerCategory(ctx, summary.Name)
		library.Map[summary.Name] = category.Items
	}
	return library, nil
}

func (f *fakeGatewayStickers) AddSticker(ctx context.Context, categoryName string, stickerName string, dataURL string) (types.StickerItem, error) {
	categoryName = strings.TrimSpace(categoryName)
	stickerName = strings.TrimSpace(stickerName)
	if f.categories[categoryName] == nil {
		f.categories[categoryName] = map[string]types.StickerItem{}
	}
	item := types.StickerItem{ID: "sticker-1", Name: stickerName, RelPath: "stickers/" + categoryName + "/sticker-1/image.png", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	f.categories[categoryName][stickerName] = item
	f.images[item.RelPath] = dataURL
	return item, nil
}

func (f *fakeGatewayStickers) RenameSticker(ctx context.Context, categoryName string, oldStickerName string, newStickerName string) (types.StickerItem, error) {
	items := f.categories[strings.TrimSpace(categoryName)]
	item := items[strings.TrimSpace(oldStickerName)]
	delete(items, strings.TrimSpace(oldStickerName))
	item.Name = strings.TrimSpace(newStickerName)
	item.UpdatedAt = time.Now().UTC()
	items[item.Name] = item
	return item, nil
}

func (f *fakeGatewayStickers) DeleteSticker(ctx context.Context, categoryName string, stickerName string) error {
	delete(f.categories[strings.TrimSpace(categoryName)], strings.TrimSpace(stickerName))
	return nil
}

func (f *fakeGatewayStickers) DeleteStickerCategory(ctx context.Context, categoryName string) error {
	delete(f.categories, strings.TrimSpace(categoryName))
	return nil
}

func (f *fakeGatewayStickers) LoadStickerImage(ctx context.Context, relPath string) (string, error) {
	return f.images[strings.TrimSpace(relPath)], nil
}

func (f *fakeGatewayStickers) LoadStickerNamingConfig(ctx context.Context) (types.StickerNamingConfig, error) {
	return f.config, nil
}

func (f *fakeGatewayStickers) SaveStickerNamingConfig(ctx context.Context, config types.StickerNamingConfig) (types.StickerNamingConfig, error) {
	f.config = config
	return f.config, nil
}

func (f *fakeGatewayStickers) LoadMermaidFixConfig(ctx context.Context) (types.MermaidFixConfig, error) {
	return f.mermaid, nil
}

func (f *fakeGatewayStickers) SaveMermaidFixConfig(ctx context.Context, config types.MermaidFixConfig) (types.MermaidFixConfig, error) {
	f.mermaid = config
	return f.mermaid, nil
}

func (f *fakeGatewayStickers) LoadChatTitleNamingConfig(ctx context.Context) (types.ChatTitleNamingConfig, error) {
	return f.title, nil
}

func (f *fakeGatewayStickers) SaveChatTitleNamingConfig(ctx context.Context, config types.ChatTitleNamingConfig) (types.ChatTitleNamingConfig, error) {
	f.title = config
	return f.title, nil
}

func (f *fakeGatewayStickers) LoadContextCompressionConfig(ctx context.Context) (types.ContextCompressionConfig, error) {
	return f.compressionConfig, nil
}

func (f *fakeGatewayStickers) SaveContextCompressionConfig(ctx context.Context, config types.ContextCompressionConfig) (types.ContextCompressionConfig, error) {
	f.compressionConfig = config
	return f.compressionConfig, nil
}

type fakeGatewayAssist struct{ stickers *fakeGatewayStickers }

func (f *fakeGatewayAssist) GenerateStickerName(ctx context.Context, request types.StickerNameRequest) (types.StickerNameResult, error) {
	if !f.stickers.config.Enabled {
		return types.StickerNameResult{}, errors.New("disabled")
	}
	item, err := f.stickers.RenameSticker(ctx, request.CategoryName, request.StickerName, "AI 命名")
	if err != nil {
		return types.StickerNameResult{}, err
	}
	return types.StickerNameResult{Name: item.Name, Sticker: item, Changed: true}, nil
}

func (f *fakeGatewayAssist) GenerateChatTitle(ctx context.Context, request types.ChatTitleRequest) (types.ChatTitleResult, error) {
	if !f.stickers.title.Enabled {
		return types.ChatTitleResult{}, errors.New("disabled")
	}
	return types.ChatTitleResult{Title: "AI 标题"}, nil
}

func (f *fakeGatewayAssist) FixMermaidInMessage(ctx context.Context, request types.MermaidFixRequest) (types.MermaidFixResult, error) {
	if !f.stickers.mermaid.Enabled {
		return types.MermaidFixResult{}, errors.New("disabled")
	}
	return types.MermaidFixResult{MessageID: request.MessageID, MermaidSource: "graph TD\nA-->B", UpdatedContent: "```mermaid\ngraph TD\nA-->B\n```"}, nil
}

func decodeResponseData[T any](t *testing.T, body string) T {
	t.Helper()
	var payload struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	return payload.Data
}
