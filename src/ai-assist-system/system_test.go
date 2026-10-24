package aiassist

import (
	"context"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

type fakeAssistStorage struct {
	session             types.Session
	loadedWorkspaceID   string
	loadedRoleID        string
	updatedWorkspaceID  string
	updatedRoleID       string
	updatedSessionTitle string
	updatedMessageID    string
	titleConfig         types.ChatTitleNamingConfig
	mermaidConfig       types.MermaidFixConfig
}

func enabledCoordinate() types.ModelCoordinate {
	return types.ModelCoordinate{ProviderID: "provider-1", ModelID: "model-1"}
}

func (f *fakeAssistStorage) LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error) {
	return f.session, nil
}

func (f *fakeAssistStorage) LoadWorkspaceSession(ctx context.Context, workspaceID string, roleID string, sessionID string) (types.Session, error) {
	f.loadedWorkspaceID = workspaceID
	f.loadedRoleID = roleID
	return f.session, nil
}

func (f *fakeAssistStorage) UpdateSessionTitle(ctx context.Context, roleID string, sessionID string, title string) (types.Session, error) {
	f.updatedSessionTitle = title
	return f.session, nil
}

func (f *fakeAssistStorage) UpdateWorkspaceSessionTitle(ctx context.Context, workspaceID string, roleID string, sessionID string, title string) (types.Session, error) {
	f.updatedWorkspaceID = workspaceID
	f.updatedRoleID = roleID
	f.updatedSessionTitle = title
	return f.session, nil
}

func (f *fakeAssistStorage) UpdateSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string, patch types.SessionMessagePatch) (types.Message, error) {
	f.updatedMessageID = messageID
	return types.Message{ID: messageID}, nil
}

func (f *fakeAssistStorage) UpdateWorkspaceSessionMessage(ctx context.Context, workspaceID string, roleID string, sessionID string, messageID string, patch types.SessionMessagePatch) (types.Message, error) {
	f.updatedWorkspaceID = workspaceID
	f.updatedRoleID = roleID
	f.updatedMessageID = messageID
	return types.Message{ID: messageID}, nil
}

func (f *fakeAssistStorage) LoadMermaidFixConfig(ctx context.Context) (types.MermaidFixConfig, error) {
	return f.mermaidConfig, nil
}

func (f *fakeAssistStorage) LoadChatTitleNamingConfig(ctx context.Context) (types.ChatTitleNamingConfig, error) {
	return f.titleConfig, nil
}

func (f *fakeAssistStorage) LoadStickerCategory(ctx context.Context, categoryName string) (types.StickerCategory, error) {
	return types.StickerCategory{}, nil
}

func (f *fakeAssistStorage) LoadStickerImage(ctx context.Context, relPath string) (string, error) {
	return "", nil
}

func (f *fakeAssistStorage) RenameSticker(ctx context.Context, categoryName string, oldStickerName string, newStickerName string) (types.StickerItem, error) {
	return types.StickerItem{}, nil
}

func (f *fakeAssistStorage) LoadStickerNamingConfig(ctx context.Context) (types.StickerNamingConfig, error) {
	return types.StickerNamingConfig{}, nil
}

type fakeAssistModels struct {
	content string
}

func (f *fakeAssistModels) Complete(ctx context.Context, request types.ModelRequest) (types.ModelResponse, error) {
	return types.ModelResponse{Content: f.content}, nil
}

func TestGenerateChatTitleUsesWorkspaceSessionIdentity(t *testing.T) {
	storage := &fakeAssistStorage{
		session:     types.Session{ID: "session-1", WorkspaceID: "workspace-1", RoleID: "role-1", Messages: []types.Message{{ID: "m1", Type: "user", Content: "讨论工作区会话身份"}}},
		titleConfig: types.ChatTitleNamingConfig{Enabled: true, Coordinate: enabledCoordinate(), SystemPrompt: "title"},
	}
	system, err := NewSystem(Config{}, storage, &fakeAssistModels{content: "工作区会话身份"})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}

	result, err := system.GenerateChatTitle(context.Background(), types.ChatTitleRequest{WorkspaceID: "workspace-1", RoleID: "role-1", SessionID: "session-1"})
	if err != nil {
		t.Fatalf("GenerateChatTitle() error = %v", err)
	}
	if result.Title != "工作区会话身份" {
		t.Fatalf("title = %q", result.Title)
	}
	if storage.loadedWorkspaceID != "workspace-1" || storage.loadedRoleID != "role-1" {
		t.Fatalf("loaded workspace identity = %q/%q", storage.loadedWorkspaceID, storage.loadedRoleID)
	}
	if storage.updatedWorkspaceID != "workspace-1" || storage.updatedRoleID != "role-1" || storage.updatedSessionTitle != result.Title {
		t.Fatalf("updated workspace title = %q/%q/%q", storage.updatedWorkspaceID, storage.updatedRoleID, storage.updatedSessionTitle)
	}
}

func TestFixMermaidUsesWorkspaceSessionIdentity(t *testing.T) {
	source := "graph TD\nA-->C"
	storage := &fakeAssistStorage{
		session:       types.Session{ID: "session-1", WorkspaceID: "workspace-1", RoleID: "role-1", Messages: []types.Message{{ID: "m1", Type: "assistant", Content: "```mermaid\n" + source + "\n```"}}},
		mermaidConfig: types.MermaidFixConfig{Enabled: true, Coordinate: enabledCoordinate(), SystemPrompt: "mermaid"},
	}
	system, err := NewSystem(Config{}, storage, &fakeAssistModels{content: "graph TD\nA-->B"})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}

	result, err := system.FixMermaidInMessage(context.Background(), types.MermaidFixRequest{WorkspaceID: "workspace-1", RoleID: "role-1", SessionID: "session-1", MessageID: "m1", MermaidSource: source})
	if err != nil {
		t.Fatalf("FixMermaidInMessage() error = %v", err)
	}
	if !strings.Contains(result.UpdatedContent, "A-->B") {
		t.Fatalf("updated content = %q", result.UpdatedContent)
	}
	if storage.updatedWorkspaceID != "workspace-1" || storage.updatedRoleID != "role-1" || storage.updatedMessageID != "m1" {
		t.Fatalf("updated workspace message = %q/%q/%q", storage.updatedWorkspaceID, storage.updatedRoleID, storage.updatedMessageID)
	}
}

func TestFixMermaidInWorkspaceSessionToleratesFenceWhitespace(t *testing.T) {
	source := "graph TD\n    A --> B"
	storage := &fakeAssistStorage{
		session:       types.Session{ID: "session-1", WorkspaceID: "workspace-1", RoleID: "role-1", Messages: []types.Message{{ID: "m1", Type: "assistant", Content: "```mermaid\n\n" + source + " \n\n```"}}},
		mermaidConfig: types.MermaidFixConfig{Enabled: true, Coordinate: enabledCoordinate(), SystemPrompt: "mermaid"},
	}
	system, err := NewSystem(Config{}, storage, &fakeAssistModels{content: "```mermaid\ngraph TD\n    A --> C\n```"})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}

	result, err := system.FixMermaidInMessage(context.Background(), types.MermaidFixRequest{WorkspaceID: "workspace-1", RoleID: "role-1", SessionID: "session-1", MessageID: "m1", MermaidSource: source})
	if err != nil {
		t.Fatalf("FixMermaidInMessage() error = %v", err)
	}
	if result.MermaidSource != "graph TD\n    A --> C" {
		t.Fatalf("mermaid source = %q", result.MermaidSource)
	}
	if result.UpdatedContent != "```mermaid\ngraph TD\n    A --> C\n```" {
		t.Fatalf("updated content = %q", result.UpdatedContent)
	}
	if storage.updatedWorkspaceID != "workspace-1" || storage.updatedRoleID != "role-1" || storage.updatedMessageID != "m1" {
		t.Fatalf("updated workspace message = %q/%q/%q", storage.updatedWorkspaceID, storage.updatedRoleID, storage.updatedMessageID)
	}
}
