package datastorage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

func TestInitializeCreatesStorageLayout(t *testing.T) {
	system := newTestSystem(t)
	for _, dir := range []string{"sessions", "roles", "providers", "tools", "stickers", "recycle", "meta"} {
		assertDir(t, filepath.Join(system.paths.root, dir))
	}
	for _, dir := range []string{"roles", "groups", "workspaces"} {
		assertDir(t, filepath.Join(system.paths.root, "sessions", dir))
	}
	assertFile(t, filepath.Join(system.paths.root, "meta", "version.json"))
	assertFile(t, filepath.Join(system.paths.root, "sessions", "favorites.json"))
}

func TestSaveLoadListAndDeleteRole(t *testing.T) {
	system := newTestSystem(t)
	role := types.Role{ID: "developer", Name: "Developer", Avatar: "avatar.png", UpdatedAt: time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)}
	if err := system.SaveRole(context.Background(), role); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}
	loaded, err := system.LoadRole(context.Background(), "developer")
	if err != nil {
		t.Fatalf("LoadRole() error = %v", err)
	}
	if loaded.Name != "Developer" {
		t.Fatalf("loaded role name = %s", loaded.Name)
	}
	roles, err := system.ListRoles(context.Background())
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	if len(roles) != 1 || roles[0].ID != "developer" {
		t.Fatalf("roles = %#v", roles)
	}
	if err := system.DeleteRole(context.Background(), "developer"); err != nil {
		t.Fatalf("DeleteRole() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(system.paths.root, "roles", "developer")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("role directory still exists err=%v", err)
	}
	recycleEntries, err := os.ReadDir(filepath.Join(system.paths.root, "recycle"))
	if err != nil {
		t.Fatalf("ReadDir(recycle) error = %v", err)
	}
	if len(recycleEntries) == 0 {
		t.Fatalf("expected recycle item")
	}
}

func TestSaveLoadAndDeleteRoleAvatarImageFile(t *testing.T) {
	system := newTestSystem(t)
	role := types.Role{ID: "developer", Name: "Developer"}
	if err := system.SaveRole(context.Background(), role); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}
	avatar := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lz2YNgAAAABJRU5ErkJggg=="
	if err := system.SaveRoleAvatar(context.Background(), role.ID, avatar); err != nil {
		t.Fatalf("SaveRoleAvatar() error = %v", err)
	}

	avatarDir := filepath.Join(system.paths.root, "roles", role.ID, "attachments", "avatar")
	assertFile(t, filepath.Join(avatarDir, "avatar.png"))
	assertNoFile(t, filepath.Join(avatarDir, "avatar.bin"))
	assertNoFile(t, filepath.Join(avatarDir, "avatar.json"))

	loaded, err := system.LoadRoleAvatar(context.Background(), role.ID)
	if err != nil {
		t.Fatalf("LoadRoleAvatar() error = %v", err)
	}
	if loaded != avatar {
		t.Fatalf("loaded avatar = %q, want %q", loaded, avatar)
	}

	if err := system.DeleteRoleAvatar(context.Background(), role.ID); err != nil {
		t.Fatalf("DeleteRoleAvatar() error = %v", err)
	}
	assertNoFile(t, filepath.Join(avatarDir, "avatar.png"))
}

func TestSaveLoadModelRequestConfig(t *testing.T) {
	system := newTestSystem(t)
	config, err := system.LoadModelRequestConfig(context.Background())
	if err != nil {
		t.Fatalf("LoadModelRequestConfig(default) error = %v", err)
	}
	if config.ListModelsTimeoutMs != types.ModelRequestListModelsTimeoutDefaultMs || config.CompletionTimeoutMs != types.ModelRequestCompletionTimeoutDefaultMs || config.StreamIdleTimeoutMs != types.ModelRequestStreamIdleTimeoutDefaultMs {
		t.Fatalf("default config = %#v", config)
	}

	saved, err := system.SaveModelRequestConfig(context.Background(), types.ModelRequestConfig{ListModelsTimeoutMs: 45_000, CompletionTimeoutMs: 180_000, StreamIdleTimeoutMs: 90_000})
	if err != nil {
		t.Fatalf("SaveModelRequestConfig() error = %v", err)
	}
	loaded, err := system.LoadModelRequestConfig(context.Background())
	if err != nil {
		t.Fatalf("LoadModelRequestConfig(saved) error = %v", err)
	}
	if loaded.ListModelsTimeoutMs != saved.ListModelsTimeoutMs || loaded.CompletionTimeoutMs != saved.CompletionTimeoutMs || loaded.StreamIdleTimeoutMs != saved.StreamIdleTimeoutMs {
		t.Fatalf("loaded config = %#v saved=%#v", loaded, saved)
	}
	assertFile(t, filepath.Join(system.paths.root, "meta", "model-request.json"))
}

func TestSessionsAreListedByLastActive(t *testing.T) {
	system := newTestSystem(t)
	oldSession := types.Session{ID: "old", RoleID: "developer", Title: "old", LastActive: time.Date(2026, 5, 30, 8, 0, 0, 0, time.UTC)}
	newSession := types.Session{ID: "new", RoleID: "developer", Title: "new", LastActive: time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)}
	if err := system.SaveSession(context.Background(), oldSession); err != nil {
		t.Fatalf("SaveSession(old) error = %v", err)
	}
	if err := system.SaveSession(context.Background(), newSession); err != nil {
		t.Fatalf("SaveSession(new) error = %v", err)
	}
	sessions, err := system.ListSessions(context.Background(), "developer")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 2 || sessions[0].ID != "new" || sessions[1].ID != "old" {
		t.Fatalf("sessions = %#v", sessions)
	}
	index, err := readJSON[sessionRoleIndex](context.Background(), filepath.Join(system.paths.root, "sessions", "roles", "index.json"))
	if err != nil {
		t.Fatalf("read session root index error = %v", err)
	}
	if len(index.Folders) != 1 || index.Folders[0].ID != "developer" {
		t.Fatalf("session root index = %#v", index)
	}
}

func TestCreateSessionCreatesCanonicalSession(t *testing.T) {
	system := newTestSystem(t)
	session, err := system.CreateSession(context.Background(), "developer", "Fresh chat")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if !strings.HasPrefix(session.ID, "session-") {
		t.Fatalf("session id = %q", session.ID)
	}
	if session.RoleID != "developer" || session.Title != "Fresh chat" || session.Status != string(types.RunStatusCreated) {
		t.Fatalf("session = %#v", session)
	}
	if len(session.Messages) != 0 || session.CreatedAt.IsZero() || session.LastActive.IsZero() {
		t.Fatalf("session timestamps/messages = %#v", session)
	}
	assertFile(t, filepath.Join(system.paths.root, "sessions", "roles", "developer", session.ID, "data.json"))
}

func TestSaveSessionStoresMessageTextOnlyInParts(t *testing.T) {
	system := newTestSystem(t)
	now := time.Date(2026, 6, 4, 16, 28, 0, 0, time.UTC)
	session := types.Session{
		ID:        "session-text-storage",
		RoleID:    "developer",
		Title:     "Text Storage",
		Status:    string(types.RunStatusCompleted),
		CreatedAt: now,
		UpdatedAt: now,
		Messages: []types.Message{
			{ID: "m1", Type: "user", Content: "你好呀呀呀呀", BranchID: "main", CreatedAt: now, UpdatedAt: now},
			{ID: "m2", Type: "assistant", Content: "收到啦", ParentMessageID: "m1", BranchID: "main", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
			{ID: "m3", Type: "tool", Content: "tool output", ParentMessageID: "m2", BranchID: "main", CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second)},
		},
		LastActive: now.Add(2 * time.Second),
	}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	dataFile := filepath.Join(system.paths.root, "sessions", "roles", "developer", "session-text-storage", "data.json")
	stored, err := readJSON[map[string]any](context.Background(), dataFile)
	if err != nil {
		t.Fatalf("read stored session error = %v", err)
	}
	messages, ok := stored["messages"].([]any)
	if !ok || len(messages) != 3 {
		t.Fatalf("stored messages = %#v", stored["messages"])
	}
	userMessage, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("stored user message = %#v", messages[0])
	}
	if _, exists := userMessage["content"]; exists {
		t.Fatalf("stored user message duplicated content: %#v", userMessage)
	}
	userParts, ok := userMessage["parts"].([]any)
	if !ok || len(userParts) != 1 {
		t.Fatalf("stored user parts = %#v", userMessage["parts"])
	}
	userTextPart, ok := userParts[0].(map[string]any)
	if !ok || userTextPart["text"] != "你好呀呀呀呀" {
		t.Fatalf("stored user text part = %#v", userParts[0])
	}
	assistantMessage, ok := messages[1].(map[string]any)
	if !ok {
		t.Fatalf("stored assistant message = %#v", messages[1])
	}
	if _, exists := assistantMessage["content"]; exists {
		t.Fatalf("stored assistant message duplicated content: %#v", assistantMessage)
	}
	toolMessage, ok := messages[2].(map[string]any)
	if !ok || toolMessage["content"] != "tool output" {
		t.Fatalf("stored tool message content = %#v", messages[2])
	}

	loaded, err := system.LoadSession(context.Background(), "developer", "session-text-storage")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if len(loaded.Messages) != 3 || loaded.Messages[0].Content != "你好呀呀呀呀" || loaded.Messages[1].Content != "收到啦" || loaded.Messages[2].Content != "tool output" {
		t.Fatalf("loaded messages = %#v", loaded.Messages)
	}
}

func TestSaveSessionPreservesAsyncToolResultMessageType(t *testing.T) {
	system := newTestSystem(t)
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	session := types.Session{
		ID:        "session-async-tool-result",
		RoleID:    "developer",
		Title:     "Async Tool Result",
		Status:    string(types.RunStatusCompleted),
		CreatedAt: now,
		UpdatedAt: now,
		Messages: []types.Message{{
			ID:        "m1",
			Type:      types.MessageTypeAsyncToolResult,
			Content:   "以下是异步任务 async-1 的执行结果",
			BranchID:  "main",
			CreatedAt: now,
			UpdatedAt: now,
		}},
		LastActive: now,
	}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	loaded, err := system.LoadSession(context.Background(), "developer", "session-async-tool-result")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Type != types.MessageTypeAsyncToolResult || !strings.Contains(loaded.Messages[0].Content, "异步任务 async-1") {
		t.Fatalf("loaded messages = %#v", loaded.Messages)
	}
}

func TestSaveSessionMessageAttachmentStoresImagesAndText(t *testing.T) {
	system := newTestSystem(t)
	session := types.Session{ID: "session-1", RoleID: "developer", Title: "Attachments"}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	imageDataURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lz2YNgAAAABJRU5ErkJggg=="
	image, err := system.SaveSessionMessageAttachment(context.Background(), "developer", "session-1", types.RunAttachment{Kind: "image", Name: "shot.png", DataURL: imageDataURL})
	if err != nil {
		t.Fatalf("SaveSessionMessageAttachment(image) error = %v", err)
	}
	if image.Kind != "image" || image.Path == "" || image.Mime != "image/png" {
		t.Fatalf("image attachment = %#v", image)
	}
	assertFile(t, filepath.Join(system.paths.root, filepath.FromSlash(image.Path)))
	loaded, err := system.LoadSessionAttachmentImage(context.Background(), image.Path)
	if err != nil {
		t.Fatalf("LoadSessionAttachmentImage() error = %v", err)
	}
	if loaded != imageDataURL {
		t.Fatalf("loaded image = %q want %q", loaded, imageDataURL)
	}

	text, err := system.SaveSessionMessageAttachment(context.Background(), "developer", "session-1", types.RunAttachment{Kind: "md", Name: "note.md", Text: "# hello", FullLen: 7, SendLen: 7, SendPct: 100})
	if err != nil {
		t.Fatalf("SaveSessionMessageAttachment(text) error = %v", err)
	}
	if text.Kind != "md" || text.Lang != "markdown" || text.Text != "# hello" || text.Path != "" {
		t.Fatalf("text attachment = %#v", text)
	}
}

func TestSessionFavoritesStore(t *testing.T) {
	system := newTestSystem(t)
	favorites := types.SessionFavorites{
		Folders: []types.SessionFavoriteFolder{{ID: "favf-1", Name: "Important", CreatedAt: 1, UpdatedAt: 1}},
		ChatRefsByFolderID: map[string][]types.SessionFavoriteChatRef{
			"favf-1": {{TargetKind: "role", TargetID: "developer", ChatID: "session-1", AddedAt: 2}},
		},
	}

	saved, err := system.SaveSessionFavorites(context.Background(), favorites)
	if err != nil {
		t.Fatalf("SaveSessionFavorites() error = %v", err)
	}
	if len(saved.Folders) != 1 || saved.Folders[0].ID != "favf-1" || len(saved.ChatRefsByFolderID["favf-1"]) != 1 {
		t.Fatalf("saved favorites = %#v", saved)
	}
	assertFile(t, filepath.Join(system.paths.root, "sessions", "favorites.json"))

	loaded, err := system.LoadSessionFavorites(context.Background())
	if err != nil {
		t.Fatalf("LoadSessionFavorites() error = %v", err)
	}
	if len(loaded.Folders) != 1 || loaded.ChatRefsByFolderID["favf-1"][0].ChatID != "session-1" {
		t.Fatalf("loaded favorites = %#v", loaded)
	}
}

func TestSessionMessageRootActions(t *testing.T) {
	system := newTestSystem(t)
	now := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)
	session := types.Session{
		ID:        "session-1",
		RoleID:    "developer",
		Title:     "Original",
		Status:    string(types.RunStatusCreated),
		CreatedAt: now,
		UpdatedAt: now,
		Messages: []types.Message{
			{ID: "m1", Type: "user", Content: "hello", BranchID: "main", CreatedAt: now, UpdatedAt: now},
			{ID: "m2", Type: "assistant", Content: "hi", ParentMessageID: "m1", BranchID: "main", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
			{ID: "m3", Type: "user", Content: "again", ParentMessageID: "m2", BranchID: "main", CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second)},
		},
		LastActive: now.Add(2 * time.Second),
	}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	updatedTitle, err := system.UpdateSessionTitle(context.Background(), "developer", "session-1", "  Renamed  chat  ")
	if err != nil {
		t.Fatalf("UpdateSessionTitle() error = %v", err)
	}
	if updatedTitle.Title != "Renamed chat" {
		t.Fatalf("title = %q", updatedTitle.Title)
	}

	updatedMessage, err := system.UpdateSessionMessage(context.Background(), "developer", "session-1", "m2", types.SessionMessagePatch{Content: ptrString("changed")})
	if err != nil {
		t.Fatalf("UpdateSessionMessage() error = %v", err)
	}
	if got := updatedMessage.Content; got != "changed" {
		t.Fatalf("message content = %q", got)
	}

	deletedSingle, err := system.DeleteSessionMessage(context.Background(), "developer", "session-1", "m2")
	if err != nil {
		t.Fatalf("DeleteSessionMessage() error = %v", err)
	}
	if len(deletedSingle.Messages) != 2 || deletedSingle.Messages[1].ID != "m3" || deletedSingle.Messages[1].ParentMessageID != "m1" {
		t.Fatalf("messages after single delete = %#v", deletedSingle.Messages)
	}

	deletedSubtree, err := system.DeleteSessionMessageSubtree(context.Background(), "developer", "session-1", "m1")
	if err != nil {
		t.Fatalf("DeleteSessionMessageSubtree() error = %v", err)
	}
	if len(deletedSubtree.Messages) != 0 {
		t.Fatalf("messages after subtree delete = %#v", deletedSubtree.Messages)
	}
}

func TestUpdateSessionMessagePersistsPartsPatch(t *testing.T) {
	system := newTestSystem(t)
	now := time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC)
	session := types.Session{
		ID:        "session-patch-parts",
		RoleID:    "developer",
		Title:     "Parts Patch",
		Status:    string(types.RunStatusCreated),
		CreatedAt: now,
		UpdatedAt: now,
		Messages: []types.Message{{
			ID:        "m1",
			Type:      "assistant",
			Content:   "checking",
			BranchID:  "main",
			CreatedAt: now,
			UpdatedAt: now,
		}},
		LastActive: now,
	}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	parts := []types.MessagePart{{
		ID:       "part-tool-1",
		Type:     "tool",
		CallID:   "call-1",
		ToolName: "shell_command",
		State:    "completed",
		Input:    map[string]any{"command": "pwd"},
		Result:   &types.ToolPartResult{ID: "result-1", ActionID: "call-1", ToolName: "shell_command", Status: types.ToolStatusSuccess, Content: "ok"},
		Display:  map[string]any{"hideResult": true},
	}}
	updatedMessage, err := system.UpdateSessionMessage(context.Background(), "developer", "session-patch-parts", "m1", types.SessionMessagePatch{Content: ptrString("checking updated"), Parts: &parts})
	if err != nil {
		t.Fatalf("UpdateSessionMessage() error = %v", err)
	}
	if updatedMessage.Content != "checking updated" {
		t.Fatalf("content = %q", updatedMessage.Content)
	}
	if len(updatedMessage.Parts) != 2 || updatedMessage.Parts[1].Display["hideResult"] != true {
		t.Fatalf("updated parts = %#v", updatedMessage.Parts)
	}
	loaded, err := system.LoadSession(context.Background(), "developer", "session-patch-parts")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if len(loaded.Messages) != 1 || len(loaded.Messages[0].Parts) != 2 || loaded.Messages[0].Parts[1].Display["hideResult"] != true {
		t.Fatalf("loaded parts = %#v", loaded.Messages)
	}
}

func TestSessionMessagePartsAreNormalized(t *testing.T) {
	system := newTestSystem(t)
	now := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)
	session := types.Session{
		ID:        "session-parts",
		RoleID:    "developer",
		Title:     "Parts",
		Status:    string(types.RunStatusCreated),
		CreatedAt: now,
		UpdatedAt: now,
		Messages: []types.Message{{
			ID:        "m1",
			Type:      "assistant",
			Content:   "checking",
			BranchID:  "main",
			CreatedAt: now,
			UpdatedAt: now,
			Parts:     []types.MessagePart{{Type: "tool", Source: types.ToolCallSourceTextProtocol, Raw: "<<<TOOL_REQUEST>>>\n[tool]: shell_command\n[command]: pwd\n<<<END_TOOL_REQUEST>>>", CallID: "call-1", ToolName: "shell_command", State: "completed", Input: map[string]any{"command": "pwd"}, Result: &types.ToolPartResult{ID: "result-1", ActionID: "call-1", ToolName: "shell_command", Status: types.ToolStatusSuccess, Content: "ok"}}},
		}},
		LastActive: now,
	}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	loaded, err := system.LoadSession(context.Background(), "developer", "session-parts")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if len(loaded.Messages) != 1 || len(loaded.Messages[0].Parts) != 2 {
		t.Fatalf("parts = %#v", loaded.Messages)
	}
	if loaded.Messages[0].Parts[0].Type != "text" || loaded.Messages[0].Parts[0].Text != "checking" {
		t.Fatalf("text part = %#v", loaded.Messages[0].Parts[0])
	}
	if loaded.Messages[0].Parts[1].Type != "tool" || loaded.Messages[0].Parts[1].Source != types.ToolCallSourceTextProtocol || loaded.Messages[0].Parts[1].Raw == "" || loaded.Messages[0].Parts[1].CallID != "call-1" || loaded.Messages[0].Parts[1].Result == nil {
		t.Fatalf("tool part = %#v", loaded.Messages[0].Parts[1])
	}
}

func TestSessionReasoningPartsAreNormalized(t *testing.T) {
	system := newTestSystem(t)
	now := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	session := types.Session{
		ID:        "session-reasoning-parts",
		RoleID:    "developer",
		Title:     "Reasoning Parts",
		Status:    string(types.RunStatusCreated),
		CreatedAt: now,
		UpdatedAt: now,
		Messages: []types.Message{{
			ID:        "m1",
			Type:      "assistant",
			Content:   "正式回答",
			BranchID:  "main",
			CreatedAt: now,
			UpdatedAt: now,
			Parts:     []types.MessagePart{{Type: "reasoning", Text: "第一步分析\n第二步判断", Source: "model"}},
		}},
		LastActive: now,
	}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	loaded, err := system.LoadSession(context.Background(), "developer", "session-reasoning-parts")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if len(loaded.Messages) != 1 || len(loaded.Messages[0].Parts) != 2 {
		t.Fatalf("parts = %#v", loaded.Messages)
	}
	if loaded.Messages[0].Parts[0].Type != "text" || loaded.Messages[0].Parts[0].Text != "正式回答" {
		t.Fatalf("text part = %#v", loaded.Messages[0].Parts[0])
	}
	if loaded.Messages[0].Parts[1].Type != "reasoning" || loaded.Messages[0].Parts[1].Text != "第一步分析\n第二步判断" || loaded.Messages[0].Parts[1].Source != "model" {
		t.Fatalf("reasoning part = %#v", loaded.Messages[0].Parts[1])
	}
}

func TestSessionReasoningPartPreservesSource(t *testing.T) {
	system := newTestSystem(t)
	now := time.Date(2026, 5, 30, 10, 30, 0, 0, time.UTC)
	session := types.Session{
		ID:        "session-reasoning-source",
		RoleID:    "developer",
		Title:     "Reasoning Source",
		Status:    string(types.RunStatusCreated),
		CreatedAt: now,
		UpdatedAt: now,
		Messages: []types.Message{{
			ID:        "m1",
			Type:      "assistant",
			Content:   "答复",
			BranchID:  "main",
			CreatedAt: now,
			UpdatedAt: now,
			Parts:     []types.MessagePart{{Type: "reasoning", Text: "按上下文推导", Source: "op"}},
		}},
		LastActive: now,
	}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	loaded, err := system.LoadSession(context.Background(), "developer", "session-reasoning-source")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if len(loaded.Messages) != 1 || len(loaded.Messages[0].Parts) != 2 {
		t.Fatalf("parts = %#v", loaded.Messages)
	}
	if loaded.Messages[0].Parts[1].Type != "reasoning" || loaded.Messages[0].Parts[1].Source != "op" {
		t.Fatalf("reasoning part source = %#v", loaded.Messages[0].Parts[1])
	}
}

func TestSaveSessionMessagesRejectsExternallyEditedOwnedMessage(t *testing.T) {
	system := newTestSystem(t)
	now := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	session := types.Session{ID: "session-save-conflict-edit", RoleID: "developer", Title: "Conflict", Status: string(types.RunStatusRunning), CreatedAt: now, UpdatedAt: now, LastActive: now, Messages: []types.Message{{ID: "u1", Type: "user", Content: "question", BranchID: "main", CreatedAt: now, UpdatedAt: now}, {ID: "a1", Type: "assistant", Content: "draft", ParentMessageID: "u1", BranchID: "main", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)}}}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	loaded, err := system.LoadSession(context.Background(), "developer", session.ID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	expected := mustTestMessage(t, loaded.Messages, "a1")

	if _, err := system.UpdateSessionMessage(context.Background(), "developer", session.ID, "a1", types.SessionMessagePatch{Content: ptrString("user edited")}); err != nil {
		t.Fatalf("UpdateSessionMessage() error = %v", err)
	}
	incoming := expected
	incoming.Content = "runtime overwrite"
	incoming.UpdatedAt = now.Add(2 * time.Second)
	err = system.SaveSessionMessages(context.Background(), types.SessionMessageSave{Session: loaded, Writes: []types.SessionMessageWrite{{Message: incoming, Expected: &expected}}, Status: types.RunStatusRunning})
	assertAppErrorCode(t, err, "storage.conflict")
	after, err := system.LoadSession(context.Background(), "developer", session.ID)
	if err != nil {
		t.Fatalf("LoadSession(after) error = %v", err)
	}
	if got := mustTestMessage(t, after.Messages, "a1").Content; got != "user edited" {
		t.Fatalf("content after conflict = %q", got)
	}
}

func TestSaveSessionMessagesRejectsExternallyDeletedOwnedMessage(t *testing.T) {
	system := newTestSystem(t)
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	session := types.Session{ID: "session-save-conflict-delete", RoleID: "developer", Title: "Conflict", Status: string(types.RunStatusRunning), CreatedAt: now, UpdatedAt: now, LastActive: now, Messages: []types.Message{{ID: "u1", Type: "user", Content: "question", BranchID: "main", CreatedAt: now, UpdatedAt: now}, {ID: "a1", Type: "assistant", Content: "draft", ParentMessageID: "u1", BranchID: "main", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)}}}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	loaded, err := system.LoadSession(context.Background(), "developer", session.ID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	expected := mustTestMessage(t, loaded.Messages, "a1")
	if _, err := system.DeleteSessionMessage(context.Background(), "developer", session.ID, "a1"); err != nil {
		t.Fatalf("DeleteSessionMessage() error = %v", err)
	}
	incoming := expected
	incoming.Content = "runtime revive"
	incoming.UpdatedAt = now.Add(2 * time.Second)
	err = system.SaveSessionMessages(context.Background(), types.SessionMessageSave{Session: loaded, Writes: []types.SessionMessageWrite{{Message: incoming, Expected: &expected}}, Status: types.RunStatusRunning})
	assertAppErrorCode(t, err, "storage.conflict")
	after, err := system.LoadSession(context.Background(), "developer", session.ID)
	if err != nil {
		t.Fatalf("LoadSession(after) error = %v", err)
	}
	if _, ok := storedSessionMessageByID(after, "a1"); ok {
		t.Fatalf("deleted message was revived: %#v", after.Messages)
	}
}

func TestSaveSessionMessagesRejectsChangedDependency(t *testing.T) {
	system := newTestSystem(t)
	now := time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC)
	session := types.Session{ID: "session-save-conflict-dependency", RoleID: "developer", Title: "Conflict", Status: string(types.RunStatusRunning), CreatedAt: now, UpdatedAt: now, LastActive: now, Messages: []types.Message{{ID: "u1", Type: "user", Content: "question", BranchID: "main", CreatedAt: now, UpdatedAt: now}}}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	loaded, err := system.LoadSession(context.Background(), "developer", session.ID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	dependency := mustTestMessage(t, loaded.Messages, "u1")
	if _, err := system.UpdateSessionMessage(context.Background(), "developer", session.ID, "u1", types.SessionMessagePatch{Content: ptrString("changed question")}); err != nil {
		t.Fatalf("UpdateSessionMessage() error = %v", err)
	}
	assistant := types.Message{ID: "a1", Type: "assistant", Content: "answer", ParentMessageID: "u1", BranchID: "main", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)}
	err = system.SaveSessionMessages(context.Background(), types.SessionMessageSave{Session: loaded, Writes: []types.SessionMessageWrite{{Message: assistant}}, Conditions: []types.SessionMessageCondition{{MessageID: "u1", Expected: &dependency}}, Status: types.RunStatusRunning})
	assertAppErrorCode(t, err, "storage.conflict")
	after, err := system.LoadSession(context.Background(), "developer", session.ID)
	if err != nil {
		t.Fatalf("LoadSession(after) error = %v", err)
	}
	if _, ok := storedSessionMessageByID(after, "a1"); ok {
		t.Fatalf("assistant was written despite dependency conflict: %#v", after.Messages)
	}
}

func TestSaveSessionMessagesRejectsOccupiedBranchSlot(t *testing.T) {
	system := newTestSystem(t)
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	session := types.Session{ID: "session-save-conflict-branch-slot", RoleID: "developer", Title: "Conflict", Status: string(types.RunStatusRunning), CreatedAt: now, UpdatedAt: now, LastActive: now, Messages: []types.Message{{ID: "u1", Type: "user", Content: "question", BranchID: "main", CreatedAt: now, UpdatedAt: now}, {ID: "a1", Type: "assistant", Content: "answer", ParentMessageID: "u1", BranchID: "main", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)}}}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	loaded, err := system.LoadSession(context.Background(), "developer", session.ID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	first := types.Message{ID: "u2", Type: "user", Content: "first follow up", ParentMessageID: "a1", BranchID: "main", CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second)}
	second := types.Message{ID: "u3", Type: "user", Content: "second follow up", ParentMessageID: "a1", BranchID: "main", CreatedAt: now.Add(3 * time.Second), UpdatedAt: now.Add(3 * time.Second)}
	if err := system.SaveSessionMessages(context.Background(), types.SessionMessageSave{Session: loaded, Writes: []types.SessionMessageWrite{{Message: first}}, Status: types.RunStatusRunning}); err != nil {
		t.Fatalf("SaveSessionMessages(first) error = %v", err)
	}
	err = system.SaveSessionMessages(context.Background(), types.SessionMessageSave{Session: loaded, Writes: []types.SessionMessageWrite{{Message: second}}, Status: types.RunStatusRunning})
	assertAppErrorCode(t, err, "storage.conflict")
	after, err := system.LoadSession(context.Background(), "developer", session.ID)
	if err != nil {
		t.Fatalf("LoadSession(after) error = %v", err)
	}
	if _, ok := storedSessionMessageByID(after, "u2"); !ok {
		t.Fatalf("first message missing after conflict: %#v", after.Messages)
	}
	if _, ok := storedSessionMessageByID(after, "u3"); ok {
		t.Fatalf("second message was written despite branch slot conflict: %#v", after.Messages)
	}
}

func TestProviderAndToolStores(t *testing.T) {
	system := newTestSystem(t)
	provider := types.Provider{ID: "openai-main", Name: "OpenAI", Protocol: types.ProviderProtocolOpenAI, UpdatedAt: time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)}
	tool := types.ToolDefinition{ID: "file-reader", Name: "File Reader", Type: "local", UpdatedAt: time.Date(2026, 5, 30, 11, 0, 0, 0, time.UTC)}
	if err := system.SaveProvider(context.Background(), provider); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	if err := system.SaveTool(context.Background(), tool); err != nil {
		t.Fatalf("SaveTool() error = %v", err)
	}
	providers, err := system.ListProviders(context.Background())
	if err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	tools, err := system.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(providers) != 1 || providers[0].ID != provider.ID {
		t.Fatalf("providers = %#v", providers)
	}
	if len(tools) != 1 || tools[0].ID != tool.ID {
		t.Fatalf("tools = %#v", tools)
	}
}

func TestLoadToolResolvesRelativeDirectoryAgainstToolFolder(t *testing.T) {
	system := newTestSystem(t)
	tool := types.ToolDefinition{ID: "shell_command", Name: "shell_command", Description: "Run shell command", DefaultInvocationMode: types.ToolInvocationModeSync, Type: "local", Directory: ".", Binaries: []types.ToolBinary{{GOOS: "windows", GOARCH: "amd64", Path: "binary/windows-amd64/shell_command.exe"}}}
	if err := system.SaveTool(context.Background(), tool); err != nil {
		t.Fatalf("SaveTool() error = %v", err)
	}
	loaded, err := system.LoadTool(context.Background(), tool.ID)
	if err != nil {
		t.Fatalf("LoadTool() error = %v", err)
	}
	want := filepath.Join(system.paths.root, "tools", tool.ID)
	if loaded.Directory != want {
		t.Fatalf("directory = %q, want %q", loaded.Directory, want)
	}
}

func TestSaveToolUserSettingsPreservesToolDefinition(t *testing.T) {
	system := newTestSystem(t)
	tool := types.ToolDefinition{
		ID:                    "shell_command",
		Name:                  "shell_command",
		Description:           "Run shell command",
		DefaultInvocationMode: types.ToolInvocationModeSync,
		Type:                  "local",
		Directory:             ".",
		UserConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"timeoutMs": map[string]any{"type": "integer"},
			},
		},
		DefaultConfig: map[string]any{"provider": "git-bash"},
		UserConfig:    map[string]any{"timeoutMs": float64(1000)},
		Binaries:      []types.ToolBinary{{GOOS: "windows", GOARCH: "amd64", Path: "binary/windows-amd64/shell_command.exe"}},
	}
	if err := system.SaveTool(context.Background(), tool); err != nil {
		t.Fatalf("SaveTool() error = %v", err)
	}

	updated, err := system.SaveToolUserSettings(context.Background(), tool.ID, types.ToolUserSettings{UserConfig: map[string]any{"timeoutMs": float64(2000)}, PromptDescriptionOverride: "Use shell carefully"})
	if err != nil {
		t.Fatalf("SaveToolUserSettings() error = %v", err)
	}
	if updated.UserConfig["timeoutMs"] != float64(2000) {
		t.Fatalf("userConfig = %#v", updated.UserConfig)
	}
	if updated.PromptDescriptionOverride != "Use shell carefully" {
		t.Fatalf("promptDescriptionOverride = %q", updated.PromptDescriptionOverride)
	}
	if updated.Name != tool.Name || updated.Description != tool.Description || updated.DefaultInvocationMode != tool.DefaultInvocationMode || updated.Type != tool.Type || updated.DefaultConfig["provider"] != "git-bash" || len(updated.Binaries) != 1 || updated.UserConfigSchema["type"] != "object" {
		t.Fatalf("tool definition was not preserved: %#v", updated)
	}
	if updated.Directory != filepath.Join(system.paths.root, "tools", tool.ID) {
		t.Fatalf("directory = %q", updated.Directory)
	}
}

func TestRebuildIndexesRestoresDeletedIndexes(t *testing.T) {
	system := newTestSystem(t)
	role := types.Role{ID: "developer", Name: "Developer"}
	if err := system.SaveRole(context.Background(), role); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}
	index := filepath.Join(system.paths.root, "roles", "index.json")
	if err := os.Remove(index); err != nil {
		t.Fatalf("Remove(index) error = %v", err)
	}
	if err := system.RebuildIndexes(context.Background()); err != nil {
		t.Fatalf("RebuildIndexes() error = %v", err)
	}
	assertFile(t, index)
}

func TestRejectsUnsafeIDs(t *testing.T) {
	system := newTestSystem(t)
	err := system.SaveRole(context.Background(), types.Role{ID: "../escape", Name: "bad"})
	assertAppErrorCode(t, err, "storage.invalid_request")
}

func newTestSystem(t *testing.T) *system {
	t.Helper()
	created, err := NewSystem(Config{RootDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	system, ok := created.(*system)
	if !ok {
		t.Fatalf("unexpected system type %T", created)
	}
	if err := system.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	return system
}

func ptrString(value string) *string { return &value }

func mustTestMessage(t *testing.T, messages []types.Message, messageID string) types.Message {
	t.Helper()
	for _, message := range messages {
		if message.ID == messageID {
			return message
		}
	}
	t.Fatalf("message %s was not found in %#v", messageID, messages)
	return types.Message{}
}

func assertDir(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not directory", path)
	}
}

func assertFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("%s is directory", path)
	}
}

func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to not exist, err=%v", path, err)
	}
}

func assertAppErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not AppError", err)
	}
	if appErr.Code != code {
		t.Fatalf("code = %s, want %s", appErr.Code, code)
	}
}
