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
	if got := fakes.sessions.sessions["developer/session-1"].Messages[0].Content; got != "updated" {
		t.Fatalf("message content = %q", got)
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
	system, err := NewSystem(Config{}, fakes.runtime, fakes.roles, fakes.providers, fakes.tools, fakes.sessions)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	return system
}

type gatewayFakes struct {
	runtime   *fakeGatewayRuntime
	roles     *fakeGatewayRoles
	providers *fakeGatewayProviders
	tools     *fakeGatewayTools
	sessions  *fakeGatewaySessions
}

func newGatewayFakes() *gatewayFakes {
	return &gatewayFakes{runtime: newFakeGatewayRuntime(), roles: newFakeGatewayRoles(), providers: newFakeGatewayProviders(), tools: newFakeGatewayTools(), sessions: newFakeGatewaySessions()}
}

type fakeGatewaySessions struct{ sessions map[string]types.Session }

func newFakeGatewaySessions() *fakeGatewaySessions {
	return &fakeGatewaySessions{sessions: map[string]types.Session{}}
}

func (f *fakeGatewaySessions) CreateSession(ctx context.Context, roleID string, title string) (types.Session, error) {
	now := time.Now().UTC()
	session := types.Session{ID: "session-created", RoleID: roleID, Title: title, Status: string(types.RunStatusCreated), Messages: []types.Message{}, CreatedAt: now, LastActive: now}
	f.sessions[session.RoleID+"/"+session.ID] = session
	return session, nil
}

func (f *fakeGatewaySessions) SaveSession(ctx context.Context, session types.Session) error {
	f.sessions[session.RoleID+"/"+session.ID] = session
	return nil
}

func (f *fakeGatewaySessions) LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error) {
	return f.sessions[roleID+"/"+sessionID], nil
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

func (f *fakeGatewaySessions) DeleteSession(ctx context.Context, roleID string, sessionID string) error {
	delete(f.sessions, roleID+"/"+sessionID)
	return nil
}

func (f *fakeGatewaySessions) UpdateSessionTitle(ctx context.Context, roleID string, sessionID string, title string) (types.Session, error) {
	session := f.sessions[roleID+"/"+sessionID]
	session.Title = title
	session.UpdatedAt = time.Now().UTC()
	f.sessions[roleID+"/"+sessionID] = session
	return session, nil
}

func (f *fakeGatewaySessions) UpdateSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string, content string) (types.Session, error) {
	session := f.sessions[roleID+"/"+sessionID]
	for i := range session.Messages {
		if session.Messages[i].ID == messageID {
			session.Messages[i].Content = content
			session.Messages[i].UpdatedAt = time.Now().UTC()
		}
	}
	f.sessions[roleID+"/"+sessionID] = session
	return session, nil
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

func (f *fakeGatewaySessions) DeleteSessionMessageSubtree(ctx context.Context, roleID string, sessionID string, messageID string) (types.Session, error) {
	return f.DeleteSessionMessage(ctx, roleID, sessionID, messageID)
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

type fakeGatewayProviders struct{ providers map[string]types.Provider }

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
		providers = append(providers, types.ProviderSummary{ID: provider.ID, Name: provider.Name, Protocol: provider.Protocol})
	}
	return providers, nil
}
func (f *fakeGatewayProviders) DeleteProvider(ctx context.Context, providerID string) error {
	delete(f.providers, providerID)
	return nil
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
