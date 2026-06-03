package agentruntime

import (
	"context"
	"errors"
	"strings"
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

func TestStartRunSavesAttachmentsAndPassesThemToModel(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "done"}}
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	imageDataURL := "data:image/png;base64,iVBORw0KGgo="
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Attachments: []types.RunAttachment{{Kind: "image", Name: "shot.png", DataURL: imageDataURL}, {Kind: "md", Name: "note.md", Lang: "markdown", Text: "# hello", FullLen: 7, SendLen: 7, SendPct: 100}}})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	session := fakes.storage.lastSession()
	if len(session.Messages) != 2 || len(session.Messages[0].Attachments) != 2 {
		t.Fatalf("messages = %#v", session.Messages)
	}
	request := fakes.provider.lastRequest()
	if len(request.Messages) != 1 || len(request.Messages[0].Images) != 1 || request.Messages[0].Images[0].DataURL != imageDataURL {
		t.Fatalf("model request messages = %#v", request.Messages)
	}
	if !strings.Contains(request.Messages[0].Content, "附件：note.md") || !strings.Contains(request.Messages[0].Content, "# hello") {
		t.Fatalf("prompt content = %q", request.Messages[0].Content)
	}
}

func TestStartRunAddsTextToolInstructionsToModelContext(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "done"}}
	fakes.tool.instructionContent = "Tool calling instructions:\n<<<TOOL_REQUEST>>>\n[tool]: tool-name\n<<<END_TOOL_REQUEST>>>"
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "hello"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	request := fakes.provider.lastRequest()
	if len(request.Messages) < 2 || request.Messages[0].Role != "system" || !strings.Contains(request.Messages[0].Content, "<<<TOOL_REQUEST>>>") {
		t.Fatalf("model messages = %#v", request.Messages)
	}
}

func TestStartRunPersistsInputMessageBeforeReturning(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.block = make(chan struct{})
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "hello"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if state.InputMessageID == "" || state.LastMessageID != state.InputMessageID {
		t.Fatalf("initial run message ids = input %q last %q", state.InputMessageID, state.LastMessageID)
	}
	session := fakes.storage.lastSession()
	if len(session.Messages) != 1 {
		t.Fatalf("messages before model response = %#v", session.Messages)
	}
	if session.Messages[0].ID != state.InputMessageID || session.Messages[0].Type != "user" || session.Messages[0].Content != "hello" {
		t.Fatalf("input message = %#v state=%#v", session.Messages[0], state)
	}
	close(fakes.provider.block)
	waitRun(t, system, state.ID)
}

func TestStartRunStreamCreatesAssistantAndPublishesDeltas(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.streamEvents = []types.ModelStreamEvent{
		{Type: types.ModelStreamEventContentDelta, ContentDelta: "he", Content: "he", CreatedAt: time.Now().UTC()},
		{Type: types.ModelStreamEventContentDelta, ContentDelta: "llo", Content: "hello", CreatedAt: time.Now().UTC()},
	}
	fakes.provider.streamResponse = types.ModelResponse{ID: "stream-1", Content: "hello"}
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	events, unsubscribe, err := system.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "hello", Stream: true})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if !state.Stream {
		t.Fatalf("state stream = false")
	}
	if state.InputMessageID == "" || state.LastMessageID == "" || state.InputMessageID == state.LastMessageID {
		t.Fatalf("initial stream message ids = input %q last %q", state.InputMessageID, state.LastMessageID)
	}
	sessionBeforeModel := fakes.storage.lastSession()
	if len(sessionBeforeModel.Messages) != 2 || sessionBeforeModel.Messages[1].Type != "assistant" || sessionBeforeModel.Messages[1].Content != "" {
		t.Fatalf("stream initial messages = %#v", sessionBeforeModel.Messages)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted || final.LastMessageID != state.LastMessageID {
		t.Fatalf("final = %#v initial=%#v", final, state)
	}
	session := fakes.storage.lastSession()
	if len(session.Messages) != 2 || session.Messages[1].Content != "hello" || session.Messages[1].ID != state.LastMessageID {
		t.Fatalf("final stream messages = %#v", session.Messages)
	}
	gotDeltas := []string{}
	deadline := time.After(2 * time.Second)
	for len(gotDeltas) < 2 {
		select {
		case event := <-events:
			if event.Type != "model_stream_delta" {
				continue
			}
			payload, ok := event.Payload.(types.RunStreamDelta)
			if !ok {
				t.Fatalf("stream payload = %#v", event.Payload)
			}
			gotDeltas = append(gotDeltas, payload.Content)
		case <-deadline:
			t.Fatalf("stream deltas = %#v", gotDeltas)
		}
	}
	if gotDeltas[0] != "he" || gotDeltas[1] != "hello" {
		t.Fatalf("stream deltas = %#v", gotDeltas)
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

func TestRunParsesTextToolRequestsIntoUnifiedToolFlow(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "I will check.\n\n<<<TOOL_REQUEST>>>\n[tool]: file-reader\n[path]: README.md\n<<<END_TOOL_REQUEST>>>"},
		{ID: "m2", Content: "final"},
	}
	fakes.tool.parsedContent = "I will check."
	rawRequest := "<<<TOOL_REQUEST>>>\n[tool]: file-reader\n[path]: README.md\n<<<END_TOOL_REQUEST>>>"
	fakes.tool.parsedIntents = []types.ToolIntent{{ID: "text-intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}, Source: types.ToolCallSourceTextProtocol, Raw: rawRequest}}
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use text tool"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	if fakes.tool.executeCount != 1 || len(fakes.tool.normalizedIntents) != 1 || fakes.tool.normalizedIntents[0].ToolName != "file-reader" {
		t.Fatalf("tool flow execute=%d intents=%#v", fakes.tool.executeCount, fakes.tool.normalizedIntents)
	}
	session := fakes.storage.lastSession()
	if len(session.Messages) < 2 || !strings.Contains(session.Messages[1].Content, rawRequest) {
		t.Fatalf("assistant message = %#v", session.Messages)
	}
	if got := toolPartByCallID(session.Messages[1], "text-intent-1"); got == nil || got.State != "completed" || got.Source != types.ToolCallSourceTextProtocol || got.Raw != rawRequest || got.Result == nil || got.Result.Content != "tool ok" {
		t.Fatalf("tool part = %#v", session.Messages[1].Parts)
	}
}

func TestRunDoesNotPersistEmptyAssistantForPureTextToolRequest(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "<<<TOOL_REQUEST>>>\n[tool]: file-reader\n[path]: README.md\n<<<END_TOOL_REQUEST>>>"},
		{ID: "m2", Content: "final"},
	}
	fakes.tool.parsedContent = ""
	rawRequest := "<<<TOOL_REQUEST>>>\n[tool]: file-reader\n[path]: README.md\n<<<END_TOOL_REQUEST>>>"
	fakes.tool.parsedIntents = []types.ToolIntent{{ID: "text-intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}, Source: types.ToolCallSourceTextProtocol, Raw: rawRequest}}
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use pure text tool"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	session := fakes.storage.lastSession()
	if len(session.Messages) < 3 {
		t.Fatalf("messages = %#v", session.Messages)
	}
	if session.Messages[1].Type != "assistant" || session.Messages[1].Content != rawRequest {
		t.Fatalf("assistant raw content = %#v", session.Messages)
	}
	if got := toolPartByCallID(session.Messages[1], "text-intent-1"); got == nil || got.State != "completed" || got.Source != types.ToolCallSourceTextProtocol || got.Raw != rawRequest || got.Result == nil {
		t.Fatalf("assistant lacks completed text protocol tool part: %#v", session.Messages)
	}
}

func TestRunExecutesMultipleToolIntentsFromOneModelResponse(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "need tools", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}}, {ID: "intent-2", ToolName: "file-reader", Arguments: map[string]any{"path": "CHANGELOG.md"}}}},
		{ID: "m2", Content: "final"},
	}
	system := newTestRuntime(t, fakes, Config{MaxSteps: 3})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use tools"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	if fakes.tool.executeCount != 2 || len(fakes.tool.normalizedIntents) != 2 {
		t.Fatalf("tool flow execute=%d intents=%#v", fakes.tool.executeCount, fakes.tool.normalizedIntents)
	}
	session := fakes.storage.lastSession()
	if got := completedToolPartCount(session.Messages[1]); got != 2 {
		t.Fatalf("completed tool part count = %d messages=%#v", got, session.Messages)
	}
}

func toolPartByCallID(message types.Message, callID string) *types.MessagePart {
	for index := range message.Parts {
		if message.Parts[index].Type == "tool" && message.Parts[index].CallID == callID {
			return &message.Parts[index]
		}
	}
	return nil
}

func completedToolPartCount(message types.Message) int {
	count := 0
	for _, part := range message.Parts {
		if part.Type == "tool" && part.State == "completed" && part.Result != nil {
			count++
		}
	}
	return count
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
	images   map[string]string
}

func newFakeRuntimeStorage() *fakeRuntimeStorage {
	return &fakeRuntimeStorage{sessions: map[string]types.Session{}, images: map[string]string{}}
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

func (f *fakeRuntimeStorage) SaveSessionMessageAttachment(ctx context.Context, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if attachment.Kind == "image" {
		path := "sessions/" + roleID + "/" + sessionID + "/attachments/att-image/image.png"
		f.images[path] = attachment.DataURL
		return types.MessageAttachment{ID: "att-image", Kind: "image", Name: attachment.Name, Mime: "image/png", Path: path}, nil
	}
	return types.MessageAttachment{ID: "att-text", Kind: attachment.Kind, Name: attachment.Name, Lang: attachment.Lang, Text: attachment.Text, FullLen: attachment.FullLen, SendLen: attachment.SendLen, SendPct: attachment.SendPct}, nil
}

func (f *fakeRuntimeStorage) LoadSessionAttachmentImage(ctx context.Context, relPath string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	dataURL := f.images[relPath]
	if dataURL == "" {
		return "", errors.New("image missing")
	}
	return dataURL, nil
}

type fakeRuntimeRoles struct{}

func (f *fakeRuntimeRoles) BuildContext(ctx context.Context, roleID string, session types.Session, tools []types.ToolDefinition) (types.RoleContext, error) {
	return types.RoleContext{RoleID: roleID, RoleName: "Developer", ModelConfig: types.ModelConfig{Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"}, Temperature: 0.7}, Messages: session.Messages, Tools: tools, ToolPolicy: types.ToolPolicy{Tools: []string{"file-reader"}}}, nil
}

func (f *fakeRuntimeRoles) GetToolPolicy(ctx context.Context, roleID string) (types.ToolPolicy, error) {
	return types.ToolPolicy{Tools: []string{"file-reader"}}, nil
}

type fakeRuntimeProvider struct {
	mu             sync.Mutex
	responses      []types.ModelResponse
	streamEvents   []types.ModelStreamEvent
	streamResponse types.ModelResponse
	alwaysTool     bool
	calls          int
	requests       []types.ModelRequest
	block          chan struct{}
}

func (f *fakeRuntimeProvider) Complete(ctx context.Context, request types.ModelRequest) (types.ModelResponse, error) {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return types.ModelResponse{}, ctx.Err()
		}
	}
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

func (f *fakeRuntimeProvider) CompleteStream(ctx context.Context, request types.ModelRequest, onEvent types.ModelStreamHandler) (types.ModelResponse, error) {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return types.ModelResponse{}, ctx.Err()
		}
	}
	f.mu.Lock()
	f.calls++
	f.requests = append(f.requests, request)
	events := append([]types.ModelStreamEvent(nil), f.streamEvents...)
	response := f.streamResponse
	if response.ID == "" && response.Content == "" && len(response.ToolIntents) == 0 {
		response = types.ModelResponse{ID: "default-stream", Content: "done"}
	}
	f.mu.Unlock()
	for _, event := range events {
		if onEvent != nil {
			if err := onEvent(event); err != nil {
				return types.ModelResponse{}, err
			}
		}
	}
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

func (f *fakeRuntimeProvider) lastRequest() types.ModelRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return types.ModelRequest{}
	}
	return f.requests[len(f.requests)-1]
}

func (f *fakeRuntimeProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeRuntimeTools struct {
	prepareDecision    types.PermissionDecision
	confirmedDecision  types.PermissionDecision
	executeCount       int
	instructionContent string
	parsedContent      string
	parsedIntents      []types.ToolIntent
	parseErr           error
	normalizedIntents  []types.ToolIntent
}

func newFakeRuntimeTools() *fakeRuntimeTools {
	return &fakeRuntimeTools{prepareDecision: types.PermissionDecision{ID: "decision-1", Status: types.PermissionStatusAllowed}}
}

func (f *fakeRuntimeTools) ParseTextToolRequests(ctx context.Context, content string) (string, []types.ToolIntent, error) {
	if f.parseErr != nil {
		return "", nil, f.parseErr
	}
	if strings.Contains(content, "<<<TOOL_REQUEST>>>") {
		intents := append([]types.ToolIntent(nil), f.parsedIntents...)
		f.parsedIntents = nil
		return f.parsedContent, intents, nil
	}
	return content, nil, nil
}

func (f *fakeRuntimeTools) TextToolInstructions(ctx context.Context, tools []types.ToolDefinition) (types.PromptMessage, error) {
	if strings.TrimSpace(f.instructionContent) != "" {
		return types.PromptMessage{Role: "system", Content: f.instructionContent, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, nil
	}
	return types.PromptMessage{}, nil
}

func (f *fakeRuntimeTools) NormalizeIntent(ctx context.Context, intent types.ToolIntent) (types.ToolAction, error) {
	f.normalizedIntents = append(f.normalizedIntents, intent)
	return types.ToolAction{ID: intent.ID, ToolName: intent.ToolName, Arguments: intent.Arguments, Source: intent.Source, Raw: intent.Raw}, nil
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
