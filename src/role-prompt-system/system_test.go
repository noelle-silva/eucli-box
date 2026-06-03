package roleprompt

import (
	"context"
	"errors"
	"testing"
	"time"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

func TestSaveRoleValidatesModelAndStoresRole(t *testing.T) {
	storage := newFakeRoleStorage()
	providers := newFakeProviderResolver()
	providers.models["openai-main/gpt-4.1"] = types.ModelInfo{ID: "gpt-4.1", Name: "GPT"}
	system := newTestRoleSystem(t, storage, providers)
	role := validRole()
	if err := system.SaveRole(context.Background(), role); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}
	if _, ok := storage.roles[role.ID]; !ok {
		t.Fatalf("role was not saved")
	}
	if providers.resolveCount != 1 {
		t.Fatalf("resolveCount = %d", providers.resolveCount)
	}
}

func TestSaveRoleRejectsMissingPrompt(t *testing.T) {
	storage := newFakeRoleStorage()
	providers := newFakeProviderResolver()
	system := newTestRoleSystem(t, storage, providers)
	role := validRole()
	role.Prompts = nil
	err := system.SaveRole(context.Background(), role)
	assertAppErrorCode(t, err, "role.invalid_request")
}

func TestBuildContextUsesOnlyRoleSessionAndPassedTools(t *testing.T) {
	storage := newFakeRoleStorage()
	providers := newFakeProviderResolver()
	providers.models["openai-main/gpt-4.1"] = types.ModelInfo{ID: "gpt-4.1", Name: "GPT"}
	role := validRole()
	role.Prompts = []types.PromptMessage{
		{ID: "2", Role: "system", Content: "second", Order: 2},
		{ID: "1", Role: "system", Content: "first", Order: 1},
	}
	storage.roles[role.ID] = role
	system := newTestRoleSystem(t, storage, providers)
	session := types.Session{ID: "session-1", RoleID: role.ID, Messages: []types.Message{{ID: "m1", Type: "user", Content: "hello"}}}
	tools := []types.ToolDefinition{{ID: "file-reader", Name: "file-reader", Description: "Read"}}
	ctx, err := system.BuildContext(context.Background(), role.ID, session, tools)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	if ctx.Prompts[0].Content != "first" || ctx.Prompts[1].Content != "second" {
		t.Fatalf("prompts were not sorted: %#v", ctx.Prompts)
	}
	if len(ctx.Messages) != 1 || ctx.Messages[0].Content != "hello" {
		t.Fatalf("messages = %#v", ctx.Messages)
	}
	if len(ctx.Tools) != 1 || ctx.Tools[0].Name != "file-reader" {
		t.Fatalf("tools = %#v", ctx.Tools)
	}
}

func TestGetToolPolicyReturnsClone(t *testing.T) {
	storage := newFakeRoleStorage()
	providers := newFakeProviderResolver()
	role := validRole()
	storage.roles[role.ID] = role
	system := newTestRoleSystem(t, storage, providers)
	policy, err := system.GetToolPolicy(context.Background(), role.ID)
	if err != nil {
		t.Fatalf("GetToolPolicy() error = %v", err)
	}
	policy.Tools[0] = "changed"
	if storage.roles[role.ID].ToolPolicy.Tools[0] == "changed" {
		t.Fatalf("policy was not cloned")
	}
}

func TestGetToolRunModeFailsWhenMissing(t *testing.T) {
	storage := newFakeRoleStorage()
	providers := newFakeProviderResolver()
	role := validRole()
	storage.roles[role.ID] = role
	system := newTestRoleSystem(t, storage, providers)
	_, err := system.GetToolRunMode(context.Background(), role.ID, "missing")
	assertAppErrorCode(t, err, "role.tool_mode_missing")
}

func validRole() types.Role {
	return types.Role{
		ID:   "developer",
		Name: "Developer",
		Prompts: []types.PromptMessage{
			{ID: "p1", Role: "system", Content: "You write clear code", Order: 1},
		},
		ModelConfig: types.ModelConfig{Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"}, Temperature: 0.7},
		ToolPolicy:  types.ToolPolicy{Tools: []string{"file-reader"}, RunModes: map[string]types.ToolRunMode{"file-reader": types.ToolRunAsk}},
		UpdatedAt:   time.Now().UTC(),
	}
}

func newTestRoleSystem(t *testing.T, storage StorageSystem, providers ProviderSystem) System {
	t.Helper()
	system, err := NewSystem(Config{}, storage, providers)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	return system
}

type fakeRoleStorage struct {
	roles   map[string]types.Role
	avatars map[string]string
}

func newFakeRoleStorage() *fakeRoleStorage {
	return &fakeRoleStorage{roles: map[string]types.Role{}, avatars: map[string]string{}}
}

func (f *fakeRoleStorage) SaveRole(ctx context.Context, role types.Role) error {
	f.roles[role.ID] = role
	return nil
}

func (f *fakeRoleStorage) LoadRole(ctx context.Context, roleID string) (types.Role, error) {
	role, ok := f.roles[roleID]
	if !ok {
		return types.Role{}, errors.New("role missing")
	}
	return role, nil
}

func (f *fakeRoleStorage) ListRoles(ctx context.Context) ([]types.RoleSummary, error) {
	summaries := make([]types.RoleSummary, 0, len(f.roles))
	for _, role := range f.roles {
		summaries = append(summaries, types.RoleSummary{ID: role.ID, Name: role.Name, Avatar: role.Avatar, UpdatedAt: role.UpdatedAt})
	}
	return summaries, nil
}

func (f *fakeRoleStorage) DeleteRole(ctx context.Context, roleID string) error {
	delete(f.roles, roleID)
	delete(f.avatars, roleID)
	return nil
}

func (f *fakeRoleStorage) SaveRoleAvatar(ctx context.Context, roleID string, dataURL string) error {
	f.avatars[roleID] = dataURL
	return nil
}

func (f *fakeRoleStorage) LoadRoleAvatar(ctx context.Context, roleID string) (string, error) {
	avatar, ok := f.avatars[roleID]
	if !ok {
		return "", errors.New("role avatar missing")
	}
	return avatar, nil
}

func (f *fakeRoleStorage) DeleteRoleAvatar(ctx context.Context, roleID string) error {
	delete(f.avatars, roleID)
	return nil
}

type fakeProviderResolver struct {
	models       map[string]types.ModelInfo
	resolveCount int
}

func newFakeProviderResolver() *fakeProviderResolver {
	return &fakeProviderResolver{models: map[string]types.ModelInfo{}}
}

func (f *fakeProviderResolver) ResolveModel(ctx context.Context, coordinate types.ModelCoordinate) (types.Provider, types.ModelInfo, error) {
	f.resolveCount++
	model, ok := f.models[coordinate.ProviderID+"/"+coordinate.ModelID]
	if !ok {
		return types.Provider{}, types.ModelInfo{}, errors.New("model missing")
	}
	return types.Provider{ID: coordinate.ProviderID}, model, nil
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
