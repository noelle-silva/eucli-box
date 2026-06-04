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
	index, err := readJSON[sessionRoleIndex](context.Background(), filepath.Join(system.paths.root, "sessions", "index.json"))
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
	assertFile(t, filepath.Join(system.paths.root, "sessions", "developer", session.ID, "data.json"))
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

	updatedMessage, err := system.UpdateSessionMessage(context.Background(), "developer", "session-1", "m2", "changed")
	if err != nil {
		t.Fatalf("UpdateSessionMessage() error = %v", err)
	}
	if got := updatedMessage.Messages[1].Content; got != "changed" {
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
	tool := types.ToolDefinition{ID: "shell_command", Name: "shell_command", Description: "Run shell command", Type: "local", Directory: ".", Binaries: []types.ToolBinary{{GOOS: "windows", GOARCH: "amd64", Path: "binary/windows-amd64/shell_command.exe"}}}
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

func TestSaveToolUserConfigPreservesToolDefinition(t *testing.T) {
	system := newTestSystem(t)
	tool := types.ToolDefinition{
		ID:          "shell_command",
		Name:        "shell_command",
		Description: "Run shell command",
		Type:        "local",
		Directory:   ".",
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

	updated, err := system.SaveToolUserConfig(context.Background(), tool.ID, map[string]any{"timeoutMs": float64(2000)})
	if err != nil {
		t.Fatalf("SaveToolUserConfig() error = %v", err)
	}
	if updated.UserConfig["timeoutMs"] != float64(2000) {
		t.Fatalf("userConfig = %#v", updated.UserConfig)
	}
	if updated.Name != tool.Name || updated.Description != tool.Description || updated.Type != tool.Type || updated.DefaultConfig["provider"] != "git-bash" || len(updated.Binaries) != 1 || updated.UserConfigSchema["type"] != "object" {
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
