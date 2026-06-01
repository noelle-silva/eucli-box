package agentruntime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestStartRunCompletesWithoutTool(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "done"}}
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "hello"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	session := fakes.storage.lastSession()
	if len(session.Messages) != 2 || session.Messages[0].Type != "user" || session.Messages[1].Type != "assistant" {
		t.Fatalf("messages = %#v", session.Messages)
	}
	if session.Messages[1].ParentMessageID != session.Messages[0].ID {
		t.Fatalf("assistant parent = %q user=%q", session.Messages[1].ParentMessageID, session.Messages[0].ID)
	}
}

func TestStartRunFromUserMessageAppendsAssistantSibling(t *testing.T) {
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID:        "session-1",
		RoleID:    "developer",
		Title:     "Branching",
		Status:    string(types.RunStatusCompleted),
		CreatedAt: now,
		UpdatedAt: now,
		Messages: []types.Message{
			{ID: "u1", Type: "user", Content: "question", BranchID: "main", CreatedAt: now, UpdatedAt: now},
			{ID: "a1", Type: "assistant", Content: "old answer", ParentMessageID: "u1", BranchID: "main", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
		},
		LastActive: now.Add(time.Second),
	}
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "new answer"}}
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", UserMessageID: "u1"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	session := fakes.storage.sessions["developer/session-1"]
	if len(session.Messages) != 3 {
		t.Fatalf("messages = %#v", session.Messages)
	}
	last := session.Messages[2]
	if last.Type != "assistant" || last.Content != "new answer" || last.ParentMessageID != "u1" {
		t.Fatalf("last message = %#v", last)
	}
	if last.BranchID == "" || last.BranchID == "main" {
		t.Fatalf("last branch id = %q", last.BranchID)
	}
	assertPromptMessageIDs(t, fakes.provider.lastPromptMessageIDs(), []string{"u1"})
}

func TestStartRunFromUserMessageUsesOnlyParentChainContext(t *testing.T) {
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID:        "session-1",
		RoleID:    "developer",
		Title:     "Branching",
		Status:    string(types.RunStatusCompleted),
		CreatedAt: now,
		UpdatedAt: now,
		Messages: []types.Message{
			{ID: "u1", Type: "user", Content: "question", BranchID: "main", CreatedAt: now, UpdatedAt: now},
			{ID: "a1", Type: "assistant", Content: "old answer", ParentMessageID: "u1", BranchID: "main", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
			{ID: "a2", Type: "assistant", Content: "other answer", ParentMessageID: "u1", BranchID: "main", CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second)},
			{ID: "u2", Type: "user", Content: "follow up", ParentMessageID: "a1", BranchID: "main", CreatedAt: now.Add(3 * time.Second), UpdatedAt: now.Add(3 * time.Second)},
		},
		LastActive: now.Add(3 * time.Second),
	}
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "reply"}}
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", UserMessageID: "u2"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	assertPromptMessageIDs(t, fakes.provider.lastPromptMessageIDs(), []string{"u1", "a1", "u2"})
	session := fakes.storage.sessions["developer/session-1"]
	last := session.Messages[len(session.Messages)-1]
	if last.Type != "assistant" || last.ParentMessageID != "u2" {
		t.Fatalf("last message = %#v", last)
	}
}

func TestStartRunWithParentMessageIDAppendsUserAtSelectedMessage(t *testing.T) {
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID:        "session-1",
		RoleID:    "developer",
		Title:     "Branching",
		Status:    string(types.RunStatusCompleted),
		CreatedAt: now,
		UpdatedAt: now,
		Messages: []types.Message{
			{ID: "u1", Type: "user", Content: "question", BranchID: "main", CreatedAt: now, UpdatedAt: now},
			{ID: "a1", Type: "assistant", Content: "answer", ParentMessageID: "u1", BranchID: "main", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
			{ID: "u2", Type: "user", Content: "existing follow up", ParentMessageID: "a1", BranchID: "main", CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second)},
		},
		LastActive: now.Add(2 * time.Second),
	}
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "reply"}}
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", Message: "new branch", ParentMessageID: "a1"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	session := fakes.storage.sessions["developer/session-1"]
	if len(session.Messages) != 5 {
		t.Fatalf("messages = %#v", session.Messages)
	}
	newUser := session.Messages[3]
	newAssistant := session.Messages[4]
	if newUser.Type != "user" || newUser.Content != "new branch" || newUser.ParentMessageID != "a1" {
		t.Fatalf("new user = %#v", newUser)
	}
	if newUser.BranchID == "" || newUser.BranchID == "main" {
		t.Fatalf("new user branch id = %q", newUser.BranchID)
	}
	if newAssistant.Type != "assistant" || newAssistant.ParentMessageID != newUser.ID || newAssistant.BranchID != newUser.BranchID {
		t.Fatalf("new assistant = %#v user=%#v", newAssistant, newUser)
	}
	gotState, err := system.GetRun(context.Background(), state.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if gotState.InputMessageID != newUser.ID || gotState.LastMessageID != newAssistant.ID {
		t.Fatalf("run message ids = input %q last %q, want input %q last %q", gotState.InputMessageID, gotState.LastMessageID, newUser.ID, newAssistant.ID)
	}
	assertPromptMessageIDs(t, fakes.provider.lastPromptMessageIDs(), []string{"u1", "a1", newUser.ID})
}

func TestStartRunFromNonUserMessageFails(t *testing.T) {
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID:     "session-1",
		RoleID: "developer",
		Messages: []types.Message{
			{ID: "u1", Type: "user", Content: "question", BranchID: "main", CreatedAt: now, UpdatedAt: now},
			{ID: "a1", Type: "assistant", Content: "answer", ParentMessageID: "u1", BranchID: "main", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
		},
		CreatedAt: now,
		UpdatedAt: now.Add(time.Second),
	}
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", UserMessageID: "a1"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusFailed || final.Reason == "" {
		t.Fatalf("final = %#v", final)
	}
	if calls := fakes.provider.callCount(); calls != 0 {
		t.Fatalf("provider calls = %d", calls)
	}
	if got := len(fakes.storage.sessions["developer/session-1"].Messages); got != 2 {
		t.Fatalf("messages were mutated on invalid run target: len=%d", got)
	}
}

func TestStartRunWithMissingParentMessageDoesNotMutateSession(t *testing.T) {
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID:        "session-1",
		RoleID:    "developer",
		Messages:  []types.Message{{ID: "u1", Type: "user", Content: "question", BranchID: "main", CreatedAt: now, UpdatedAt: now}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", Message: "new branch", ParentMessageID: "missing"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusFailed || final.Reason == "" {
		t.Fatalf("final = %#v", final)
	}
	if calls := fakes.provider.callCount(); calls != 0 {
		t.Fatalf("provider calls = %d", calls)
	}
	if got := len(fakes.storage.sessions["developer/session-1"].Messages); got != 1 {
		t.Fatalf("messages were mutated on missing parent: len=%d", got)
	}
}

func TestRunWaitsForToolConfirmationThenCompletes(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "need tool", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}}}},
		{ID: "m2", Content: "final"},
	}
	fakes.tool.prepareDecision = types.PermissionDecision{ID: "decision-1", ActionID: "intent-1", ToolName: "file-reader", Status: types.PermissionStatusNeedsConfirmation}
	fakes.tool.confirmedDecision = types.PermissionDecision{ID: "decision-1", ActionID: "intent-1", ToolName: "file-reader", Status: types.PermissionStatusAllowed}
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use tool"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitStatus(t, system, state.ID, types.RunStatusWaitingConfirmation)
	if err := system.SubmitToolConfirmation(context.Background(), types.ToolConfirmation{DecisionID: "decision-1", Approved: true}); err != nil {
		t.Fatalf("SubmitToolConfirmation() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	if fakes.tool.executeCount != 1 {
		t.Fatalf("executeCount = %d", fakes.tool.executeCount)
	}
}

func TestRunFailsWhenMaxStepsExceeded(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.alwaysTool = true
	fakes.tool.prepareDecision = types.PermissionDecision{ID: "decision-1", ActionID: "intent-1", ToolName: "file-reader", Status: types.PermissionStatusAllowed}
	system := newTestRuntime(t, fakes, Config{MaxSteps: 1})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "loop"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusFailed || final.Reason != "run exceeded max steps" {
		t.Fatalf("final = %#v", final)
	}
}

func newTestRuntime(t *testing.T, fakes *runtimeFakes, config Config) System {
	t.Helper()
	system, err := NewSystem(config, fakes.storage, fakes.roles, fakes.provider, fakes.tool)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	return system
}

func waitRun(t *testing.T, system System, runID string) types.RunState {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, err := system.GetRun(context.Background(), runID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		if state.Status == types.RunStatusCompleted || state.Status == types.RunStatusFailed || state.Status == types.RunStatusCancelled {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	state, _ := system.GetRun(context.Background(), runID)
	t.Fatalf("run did not finish, last state = %#v", state)
	return types.RunState{}
}

func waitStatus(t *testing.T, system System, runID string, status types.RunStatus) types.RunState {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, err := system.GetRun(context.Background(), runID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		if state.Status == status {
			return state
		}
		if state.Status == types.RunStatusFailed || state.Status == types.RunStatusCancelled || state.Status == types.RunStatusCompleted {
			t.Fatalf("run reached terminal state before %s: %#v", status, state)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run did not reach status %s", status)
	return types.RunState{}
}

func assertPromptMessageIDs(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("prompt ids = %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("prompt ids = %#v want %#v", got, want)
		}
	}
}

type runtimeFakes struct {
	storage  *fakeRuntimeStorage
	roles    *fakeRuntimeRoles
	provider *fakeRuntimeProvider
	tool     *fakeRuntimeTools
}

func newRuntimeFakes() *runtimeFakes {
	return &runtimeFakes{storage: newFakeRuntimeStorage(), roles: &fakeRuntimeRoles{}, provider: &fakeRuntimeProvider{}, tool: newFakeRuntimeTools()}
}

type fakeRuntimeStorage struct {
	mu       sync.Mutex
	sessions map[string]types.Session
}

func newFakeRuntimeStorage() *fakeRuntimeStorage {
	return &fakeRuntimeStorage{sessions: map[string]types.Session{}}
}

func (f *fakeRuntimeStorage) SaveSession(ctx context.Context, session types.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[session.RoleID+"/"+session.ID] = session
	return nil
}

func (f *fakeRuntimeStorage) LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	session, ok := f.sessions[roleID+"/"+sessionID]
	if !ok {
		return types.Session{}, errors.New("session missing")
	}
	return session, nil
}

func (f *fakeRuntimeStorage) lastSession() types.Session {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, session := range f.sessions {
		return session
	}
	return types.Session{}
}

type fakeRuntimeRoles struct{}

func (f *fakeRuntimeRoles) BuildContext(ctx context.Context, roleID string, session types.Session, tools []types.ToolDefinition) (types.RoleContext, error) {
	return types.RoleContext{RoleID: roleID, RoleName: "Developer", ModelConfig: types.ModelConfig{Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"}, Temperature: 0.7}, Messages: session.Messages, Tools: tools, ToolPolicy: types.ToolPolicy{Mode: types.ToolPolicyWhitelist, Tools: []string{"file-reader"}}}, nil
}

func (f *fakeRuntimeRoles) GetToolPolicy(ctx context.Context, roleID string) (types.ToolPolicy, error) {
	return types.ToolPolicy{Mode: types.ToolPolicyWhitelist, Tools: []string{"file-reader"}}, nil
}

type fakeRuntimeProvider struct {
	mu         sync.Mutex
	responses  []types.ModelResponse
	alwaysTool bool
	calls      int
	requests   []types.ModelRequest
}

func (f *fakeRuntimeProvider) Complete(ctx context.Context, request types.ModelRequest) (types.ModelResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.requests = append(f.requests, request)
	if f.alwaysTool {
		return types.ModelResponse{ID: "m", Content: "tool", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader"}}}, nil
	}
	if len(f.responses) == 0 {
		return types.ModelResponse{ID: "default", Content: "done"}, nil
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

func (f *fakeRuntimeProvider) lastPromptMessageIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return nil
	}
	request := f.requests[len(f.requests)-1]
	ids := make([]string, 0, len(request.Messages))
	for _, message := range request.Messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func (f *fakeRuntimeProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeRuntimeTools struct {
	prepareDecision   types.PermissionDecision
	confirmedDecision types.PermissionDecision
	executeCount      int
}

func newFakeRuntimeTools() *fakeRuntimeTools {
	return &fakeRuntimeTools{prepareDecision: types.PermissionDecision{ID: "decision-1", Status: types.PermissionStatusAllowed}}
}

func (f *fakeRuntimeTools) NormalizeIntent(ctx context.Context, intent types.ToolIntent) (types.ToolAction, error) {
	return types.ToolAction{ID: intent.ID, ToolName: intent.ToolName, Arguments: intent.Arguments}, nil
}

func (f *fakeRuntimeTools) Prepare(ctx context.Context, roleID string, action types.ToolAction) (types.ToolRunPlan, error) {
	decision := f.prepareDecision
	decision.ActionID = action.ID
	decision.ToolName = action.ToolName
	planStatus := types.ToolPlanStatusReady
	if decision.Status == types.PermissionStatusDenied {
		planStatus = types.ToolPlanStatusDenied
	} else if decision.Status == types.PermissionStatusNeedsConfirmation {
		planStatus = types.ToolPlanStatusNeedsConfirmation
	}
	return types.ToolRunPlan{ID: "plan-1", Action: action, Tool: types.ToolDefinition{ID: action.ToolName, Name: action.ToolName, Description: "tool", Type: "local"}, Decision: decision, PlanStatus: planStatus}, nil
}

func (f *fakeRuntimeTools) ApplyConfirmation(ctx context.Context, plan types.ToolRunPlan, confirmation types.ToolConfirmation) (types.ToolRunPlan, error) {
	decision := f.confirmedDecision
	if decision.ID == "" {
		decision = types.PermissionDecision{ID: plan.Decision.ID, ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.PermissionStatusAllowed}
	}
	plan.Decision = decision
	return plan, nil
}

func (f *fakeRuntimeTools) Execute(ctx context.Context, plan types.ToolRunPlan) (types.ToolResult, error) {
	f.executeCount++
	return types.ToolResult{ID: "result-1", ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.ToolStatusSuccess, Content: "tool ok", CreatedAt: time.Now().UTC()}, nil
}

func (f *fakeRuntimeTools) LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error) {
	return types.ToolDefinition{ID: toolID, Name: toolID, Description: "tool", Type: "local"}, nil
}

func (f *fakeRuntimeTools) ListTools(ctx context.Context) ([]types.ToolSummary, error) {
	return []types.ToolSummary{{ID: "file-reader", Name: "file-reader", Description: "Read files", Type: "local"}}, nil
}
