package agentruntime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

func TestStartRunCompletesWithoutTool(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "done"}}
	system := newTestRuntime(t, fakes, Config{})
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

func TestStartRunUsesDefaultTitleForNewSession(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "done"}}
	system := newTestRuntime(t, fakes, Config{})
	message := "请帮我分析这个会话标题是否会和正文重复保存"
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: message})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitRun(t, system, state.ID)
	session := fakes.storage.lastSession()
	if session.Title != types.DefaultSessionTitle {
		t.Fatalf("session title = %q, want %q", session.Title, types.DefaultSessionTitle)
	}
	if len(session.Messages) == 0 || session.Messages[0].Content != message {
		t.Fatalf("messages = %#v", session.Messages)
	}
}

func TestStartRunSavesAttachmentsAndPassesThemToModel(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "done"}}
	system := newTestRuntime(t, fakes, Config{})
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

func TestStartRunDoesNotAutoInjectToolInstructionsOrNativeTools(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "done"}}
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "hello"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	request := fakes.provider.lastRequest()
	if len(request.Tools) != 0 {
		t.Fatalf("native tools = %#v", request.Tools)
	}
	for _, message := range request.Messages {
		if strings.Contains(message.Content, "<<<TOOL_REQUEST>>>") {
			t.Fatalf("tool instructions were auto injected: %#v", request.Messages)
		}
	}
}

func TestStartRunPassesReasoningEffortToModelAndSessionMetadata(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "done"}}
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "hello", ReasoningEffort: types.ReasoningEffortHigh})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	request := fakes.provider.lastRequest()
	if request.ReasoningEffort != types.ReasoningEffortHigh {
		t.Fatalf("reasoning effort = %q", request.ReasoningEffort)
	}
	session := fakes.storage.lastSession()
	if session.Metadata["reasoningEffort"] != string(types.ReasoningEffortHigh) {
		t.Fatalf("session metadata = %#v", session.Metadata)
	}
}

func TestRunMessageSaveDoesNotOverwriteNewerSessionReasoningEffort(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.block = make(chan struct{})
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "hello", ReasoningEffort: types.ReasoningEffortHigh})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	session := fakes.storage.lastSession()
	session.Metadata["reasoningEffort"] = string(types.ReasoningEffortLow)
	if err := fakes.storage.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	close(fakes.provider.block)
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	after := fakes.storage.lastSession()
	if after.Metadata["reasoningEffort"] != string(types.ReasoningEffortLow) {
		t.Fatalf("newer session reasoning effort was overwritten: %#v", after.Metadata)
	}
}

func TestStartRunPersistsNewReasoningEffortForExistingSession(t *testing.T) {
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	fakes.storage.sessions["developer/session-1"] = types.Session{ID: "session-1", RoleID: "developer", Title: "Existing", Metadata: map[string]string{"reasoningEffort": string(types.ReasoningEffortLow)}, Messages: []types.Message{{ID: "u1", Type: "user", Content: "hello", BranchID: "main", CreatedAt: now, UpdatedAt: now}}, CreatedAt: now, UpdatedAt: now, LastActive: now}
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "done"}}
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", UserMessageID: "u1", ReasoningEffort: types.ReasoningEffortHigh})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	after := fakes.storage.sessions["developer/session-1"]
	if after.Metadata["reasoningEffort"] != string(types.ReasoningEffortHigh) {
		t.Fatalf("reasoning effort was not persisted: %#v", after.Metadata)
	}
}

func TestStartRunRejectsInvalidReasoningEffort(t *testing.T) {
	fakes := newRuntimeFakes()
	system := newTestRuntime(t, fakes, Config{})
	_, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "hello", ReasoningEffort: "extreme"})
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "runtime.invalid_request" {
		t.Fatalf("error = %#v", err)
	}
}

func TestStartRunPassesOnlyNativeToolsToProvider(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.roles.policy = types.ToolPolicy{
		Tools:       []string{"file-reader", "web-search"},
		NativeTools: []string{"web-search"},
		RunModes:    map[string]types.ToolRunMode{"file-reader": types.ToolRunAsk, "web-search": types.ToolRunAsk},
	}
	fakes.tool.toolSummaries = []types.ToolSummary{{ID: "file-reader", Name: "file-reader", Description: "Read files", Type: "local"}, {ID: "web-search", Name: "web-search", Description: "Search web", Type: "network"}}
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "done"}}
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "hello"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	request := fakes.provider.lastRequest()
	if len(request.Tools) != 1 || request.Tools[0].Name != "web-search" {
		t.Fatalf("native tools = %#v", request.Tools)
	}
	if len(request.Messages) != 1 || request.Messages[0].Role != "user" {
		t.Fatalf("model messages = %#v", request.Messages)
	}
}

func TestStartRunPersistsInputMessageBeforeReturning(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.block = make(chan struct{})
	system := newTestRuntime(t, fakes, Config{})
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
	fakes.provider.block = make(chan struct{})
	fakes.provider.streamEvents = []types.ModelStreamEvent{
		{Type: types.ModelStreamEventReasoningDelta, ReasoningDelta: "先想一下", Reasoning: "先想一下", CreatedAt: time.Now().UTC()},
		{Type: types.ModelStreamEventContentDelta, ContentDelta: "he", Content: "he", CreatedAt: time.Now().UTC()},
		{Type: types.ModelStreamEventContentDelta, ContentDelta: "llo", Content: "hello", CreatedAt: time.Now().UTC()},
	}
	fakes.provider.streamResponse = types.ModelResponse{ID: "stream-1", Content: "hello", Reasoning: "先想一下"}
	system := newTestRuntime(t, fakes, Config{})
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
	if state.InputMessageID == "" || state.LastMessageID != state.InputMessageID {
		t.Fatalf("initial stream message ids = input %q last %q", state.InputMessageID, state.LastMessageID)
	}
	sessionBeforeModel := fakes.storage.lastSession()
	if len(sessionBeforeModel.Messages) != 1 || sessionBeforeModel.Messages[0].Type != "user" {
		t.Fatalf("stream initial messages = %#v", sessionBeforeModel.Messages)
	}
	close(fakes.provider.block)
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted || final.LastMessageID == "" || final.LastMessageID == state.LastMessageID {
		t.Fatalf("final = %#v initial=%#v", final, state)
	}
	session := fakes.storage.lastSession()
	if len(session.Messages) != 2 || session.Messages[1].Content != "hello" || session.Messages[1].ID != final.LastMessageID {
		t.Fatalf("final stream messages = %#v", session.Messages)
	}
	if part := reasoningPartByType(session.Messages[1]); part == nil || part.Text != "先想一下" {
		t.Fatalf("reasoning part = %#v", session.Messages[1].Parts)
	}
	gotDeltas := []string{}
	gotReasoning := []string{}
	deadline := time.After(2 * time.Second)
	for len(gotDeltas) < 2 || len(gotReasoning) < 1 {
		select {
		case event := <-events:
			if event.Type == "assistant_message_update" {
				payload, ok := event.Payload.(types.RunAssistantMessageUpdate)
				if !ok {
					t.Fatalf("assistant update payload = %#v", event.Payload)
				}
				part := reasoningPartByType(payload.Message)
				if part != nil && part.Text != "" {
					gotReasoning = append(gotReasoning, part.Text)
				}
				continue
			}
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
	if gotReasoning[0] != "先想一下" {
		t.Fatalf("reasoning updates = %#v", gotReasoning)
	}
}

func TestListActiveRunsReturnsOnlyLiveRuns(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.block = make(chan struct{})
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "hello"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	runs, err := system.ListActiveRuns(context.Background())
	if err != nil {
		t.Fatalf("ListActiveRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].ID != state.ID || runs[0].Status != types.RunStatusRunning || runs[0].InputMessageID == "" {
		t.Fatalf("active runs = %#v state=%#v", runs, state)
	}
	close(fakes.provider.block)
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("final = %#v", final)
	}
	runs, err = system.ListActiveRuns(context.Background())
	if err != nil {
		t.Fatalf("ListActiveRuns() after complete error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("active runs after complete = %#v", runs)
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
	system := newTestRuntime(t, fakes, Config{})
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
	system := newTestRuntime(t, fakes, Config{})
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
	system := newTestRuntime(t, fakes, Config{})
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
	system := newTestRuntime(t, fakes, Config{})
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
	system := newTestRuntime(t, fakes, Config{})
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

func TestConcurrentRunsOnDifferentBranchesPreserveMessages(t *testing.T) {
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID:     "session-1",
		RoleID: "developer",
		Title:  "Parallel",
		Status: string(types.RunStatusCompleted),
		Messages: []types.Message{
			{ID: "u1", Type: "user", Content: "root", BranchID: "main", CreatedAt: now, UpdatedAt: now},
			{ID: "a1", Type: "assistant", Content: "answer 1", ParentMessageID: "u1", BranchID: "main", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
			{ID: "u2", Type: "user", Content: "branch 1", ParentMessageID: "a1", BranchID: "main", CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second)},
			{ID: "a2", Type: "assistant", Content: "answer 2", ParentMessageID: "u1", BranchID: "branch-a2", CreatedAt: now.Add(3 * time.Second), UpdatedAt: now.Add(3 * time.Second)},
			{ID: "u3", Type: "user", Content: "branch 2", ParentMessageID: "a2", BranchID: "branch-a2", CreatedAt: now.Add(4 * time.Second), UpdatedAt: now.Add(4 * time.Second)},
		},
		CreatedAt:  now,
		UpdatedAt:  now.Add(4 * time.Second),
		LastActive: now.Add(4 * time.Second),
	}
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "reply one"}, {ID: "m2", Content: "reply two"}}
	system := newTestRuntime(t, fakes, Config{})
	run1, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", UserMessageID: "u2"})
	if err != nil {
		t.Fatalf("StartRun(run1) error = %v", err)
	}
	run2, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", UserMessageID: "u3"})
	if err != nil {
		t.Fatalf("StartRun(run2) error = %v", err)
	}
	waitRun(t, system, run1.ID)
	waitRun(t, system, run2.ID)
	session := fakes.storage.sessions["developer/session-1"]
	if len(session.Messages) != 7 {
		t.Fatalf("messages = %#v", session.Messages)
	}
	if _, ok := messageByID(session.Messages, run1.LastMessageID); !ok {
		t.Fatalf("run1 message missing: %q messages=%#v", run1.LastMessageID, session.Messages)
	}
	if _, ok := messageByID(session.Messages, run2.LastMessageID); !ok {
		t.Fatalf("run2 message missing: %q messages=%#v", run2.LastMessageID, session.Messages)
	}
}

func TestConcurrentRunsFromSameUserMessageCreateDistinctReplySlots(t *testing.T) {
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	fakes.storage.sessions["developer/session-1"] = types.Session{
		ID:        "session-1",
		RoleID:    "developer",
		Title:     "Parallel replies",
		Status:    string(types.RunStatusCompleted),
		Messages:  []types.Message{{ID: "u1", Type: "user", Content: "question", BranchID: "main", CreatedAt: now, UpdatedAt: now}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	fakes.provider.block = make(chan struct{})
	fakes.provider.responses = []types.ModelResponse{{ID: "m1", Content: "first"}, {ID: "m2", Content: "second"}}
	system := newTestRuntime(t, fakes, Config{})
	run1, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", UserMessageID: "u1"})
	if err != nil {
		t.Fatalf("StartRun(run1) error = %v", err)
	}
	run2, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", UserMessageID: "u1"})
	if err != nil {
		t.Fatalf("StartRun(run2) error = %v", err)
	}
	close(fakes.provider.block)
	waitRun(t, system, run1.ID)
	waitRun(t, system, run2.ID)
	session := fakes.storage.sessions["developer/session-1"]
	answers := []types.Message{}
	for _, message := range session.Messages {
		if message.Type == "assistant" && message.ParentMessageID == "u1" {
			answers = append(answers, message)
		}
	}
	if len(answers) != 2 {
		t.Fatalf("answers = %#v messages=%#v", answers, session.Messages)
	}
	if answers[0].BranchID == "" || answers[1].BranchID == "" || answers[0].BranchID == answers[1].BranchID {
		t.Fatalf("answers share branch: %#v", answers)
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
	system := newTestRuntime(t, fakes, Config{})
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
	rawRequest := "<<<TOOL_REQUEST>>>\n[tool]: file-reader\n[path]: README.md\n<<<END_TOOL_REQUEST>>>"
	fakes.tool.parsedIntents = []types.ToolIntent{{ID: "text-intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}, Source: types.ToolCallSourceTextProtocol, Raw: rawRequest}}
	system := newTestRuntime(t, fakes, Config{})
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
	if len(session.Messages) < 2 || session.Messages[1].Content != "I will check.\n\n"+rawRequest {
		t.Fatalf("assistant message = %#v", session.Messages)
	}
	if got := toolPartByCallID(session.Messages[1], "text-intent-1"); got == nil || got.State != "completed" || got.Source != types.ToolCallSourceTextProtocol || got.Raw != rawRequest || got.Result == nil || got.Result.Content != "tool ok" || !got.IsToolInvocationHidden() {
		t.Fatalf("tool part = %#v", session.Messages[1].Parts)
	}
}

func TestRunPreservesPureTextToolRequestAsAssistantContent(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "<<<TOOL_REQUEST>>>\n[tool]: file-reader\n[path]: README.md\n<<<END_TOOL_REQUEST>>>"},
		{ID: "m2", Content: "final"},
	}
	rawRequest := "<<<TOOL_REQUEST>>>\n[tool]: file-reader\n[path]: README.md\n<<<END_TOOL_REQUEST>>>"
	fakes.tool.parsedIntents = []types.ToolIntent{{ID: "text-intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}, Source: types.ToolCallSourceTextProtocol, Raw: rawRequest}}
	system := newTestRuntime(t, fakes, Config{})
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
	if got := toolPartByCallID(session.Messages[1], "text-intent-1"); got == nil || got.State != "completed" || got.Source != types.ToolCallSourceTextProtocol || got.Raw != rawRequest || got.Result == nil || !got.IsToolInvocationHidden() {
		t.Fatalf("assistant lacks completed text protocol tool part: %#v", session.Messages)
	}
}

func TestRunTextProtocolToolUsesUnifiedConfirmationState(t *testing.T) {
	fakes := newRuntimeFakes()
	rawRequest := "<<<TOOL_REQUEST>>>\n[tool]: file-reader\n[path]: README.md\n<<<END_TOOL_REQUEST>>>"
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: rawRequest},
		{ID: "m2", Content: "final"},
	}
	fakes.tool.parsedIntents = []types.ToolIntent{{ID: "text-intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}, Source: types.ToolCallSourceTextProtocol, Raw: rawRequest}}
	fakes.tool.prepareDecision = types.PermissionDecision{ID: "decision-1", ActionID: "text-intent-1", ToolName: "file-reader", Status: types.PermissionStatusNeedsConfirmation}
	fakes.tool.confirmedDecision = types.PermissionDecision{ID: "decision-1", ActionID: "text-intent-1", ToolName: "file-reader", Status: types.PermissionStatusAllowed}
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use text tool"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitStatus(t, system, state.ID, types.RunStatusWaitingConfirmation)
	session := fakes.storage.lastSession()
	if len(session.Messages) < 2 || session.Messages[1].Content != rawRequest {
		t.Fatalf("assistant content = %#v", session.Messages)
	}
	part := toolPartByCallID(session.Messages[1], "text-intent-1")
	if part == nil || part.State != "needs_confirmation" || part.Source != types.ToolCallSourceTextProtocol || part.Raw != rawRequest || part.Decision == nil || !part.IsToolInvocationHidden() {
		t.Fatalf("text protocol confirmation part = %#v", session.Messages[1].Parts)
	}
	if err := system.SubmitToolConfirmation(context.Background(), types.ToolConfirmation{DecisionID: "decision-1", Approved: true}); err != nil {
		t.Fatalf("SubmitToolConfirmation() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	finalSession := fakes.storage.lastSession()
	part = toolPartByCallID(finalSession.Messages[1], "text-intent-1")
	if part == nil || part.State != "completed" || part.Result == nil || part.Result.Content != "tool ok" || !part.IsToolInvocationHidden() {
		t.Fatalf("completed text protocol part = %#v", finalSession.Messages[1].Parts)
	}
}

func TestRunExecutesMultipleToolIntentsFromOneModelResponse(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "need tools", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}}, {ID: "intent-2", ToolName: "file-reader", Arguments: map[string]any{"path": "CHANGELOG.md"}}}},
		{ID: "m2", Content: "final"},
	}
	system := newTestRuntime(t, fakes, Config{})
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

func TestRunPublishesAssistantMessageUpdateWhenEachToolFinishes(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "need tools", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "slow.md"}}, {ID: "intent-2", ToolName: "file-reader", Arguments: map[string]any{"path": "fast.md"}}}},
		{ID: "m2", Content: "final"},
	}
	fakes.tool.executeDelays = map[string]time.Duration{"intent-1": 180 * time.Millisecond, "intent-2": 20 * time.Millisecond}
	system := newTestRuntime(t, fakes, Config{MaxParallelTools: 2})
	events, unsubscribe, err := system.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use tools"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Type != "assistant_message_update" {
				continue
			}
			payload, ok := event.Payload.(types.RunAssistantMessageUpdate)
			if !ok {
				t.Fatalf("assistant update payload = %#v", event.Payload)
			}
			fast := toolPartByCallID(payload.Message, "intent-2")
			slow := toolPartByCallID(payload.Message, "intent-1")
			if fast != nil && fast.State == "completed" && fast.Result != nil {
				if slow != nil && slow.State == "completed" && slow.Result != nil {
					t.Fatalf("fast tool update was only visible after slow tool completed: %#v", payload.Message.Parts)
				}
				final := waitRun(t, system, state.ID)
				if final.Status != types.RunStatusCompleted {
					t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
				}
				return
			}
		case <-deadline:
			t.Fatalf("did not receive fast tool assistant update before timeout")
		}
	}
}

func TestRunDoesNotPublishEmptyWaitingAssistantAfterToolResults(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.streamResponses = []types.ModelResponse{
		{ID: "m1", Content: "need tool", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}}}},
		{ID: "m2", Content: "final"},
	}
	system := newTestRuntime(t, fakes, Config{})
	events, unsubscribe, err := system.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use tool", Stream: true})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	toolCompleted := false
	emptyWaitingAfterTool := false
	deadline := time.After(100 * time.Millisecond)
	for {
		select {
		case event := <-events:
			if event.Type != "assistant_message_update" {
				continue
			}
			payload, ok := event.Payload.(types.RunAssistantMessageUpdate)
			if !ok {
				t.Fatalf("assistant update payload = %#v", event.Payload)
			}
			part := toolPartByCallID(payload.Message, "intent-1")
			if part != nil && part.State == "completed" && part.Result != nil {
				toolCompleted = true
				continue
			}
			if toolCompleted && payload.Status == types.RunStatusRunning && payload.Message.Type == "assistant" && strings.TrimSpace(payload.Message.Content) == "" && len(payload.Message.Parts) == 0 {
				emptyWaitingAfterTool = true
			}
		case <-deadline:
			if !toolCompleted {
				t.Fatalf("did not receive completed tool assistant update")
			}
			if emptyWaitingAfterTool {
				t.Fatalf("received empty assistant waiting update after tool result")
			}
			return
		}
	}
}

func TestRunPersistsReasoningWhenToolIntentHasNoVisibleAssistantText(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "", Reasoning: "先判断该调用哪个工具", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}}}},
		{ID: "m2", Content: "final"},
	}
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use tool"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	session := fakes.storage.lastSession()
	if len(session.Messages) < 2 {
		t.Fatalf("messages = %#v", session.Messages)
	}
	part := reasoningPartByType(session.Messages[1])
	if part == nil || part.Text != "先判断该调用哪个工具" {
		t.Fatalf("reasoning part = %#v", session.Messages[1].Parts)
	}
	if session.Messages[len(session.Messages)-1].Content != "final" {
		t.Fatalf("final assistant content = %#v", session.Messages)
	}
}

func TestRunExecutesToolBatchWithParallelLimit(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "need tools", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}}, {ID: "intent-2", ToolName: "file-reader", Arguments: map[string]any{"path": "CHANGELOG.md"}}, {ID: "intent-3", ToolName: "file-reader", Arguments: map[string]any{"path": "LICENSE"}}}},
		{ID: "m2", Content: "final"},
	}
	fakes.tool.executeDelay = 80 * time.Millisecond
	system := newTestRuntime(t, fakes, Config{MaxParallelTools: 2})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use tools"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	if fakes.tool.executeCount != 3 || fakes.tool.maxActiveExecutions != 2 {
		t.Fatalf("tool execution count=%d maxActive=%d", fakes.tool.executeCount, fakes.tool.maxActiveExecutions)
	}
}

func TestRunWaitsForMultipleToolConfirmationsThenExecutesBatch(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "need tools", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}}, {ID: "intent-2", ToolName: "file-reader", Arguments: map[string]any{"path": "CHANGELOG.md"}}}},
		{ID: "m2", Content: "final"},
	}
	fakes.tool.prepareDecisions = map[string]types.PermissionDecision{
		"intent-1": {ID: "decision-1", ActionID: "intent-1", ToolName: "file-reader", Status: types.PermissionStatusNeedsConfirmation},
		"intent-2": {ID: "decision-2", ActionID: "intent-2", ToolName: "file-reader", Status: types.PermissionStatusNeedsConfirmation},
	}
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use tools"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitStatus(t, system, state.ID, types.RunStatusWaitingConfirmation)
	if err := system.SubmitToolConfirmation(context.Background(), types.ToolConfirmation{DecisionID: "decision-2", Approved: true}); err != nil {
		t.Fatalf("SubmitToolConfirmation(decision-2) error = %v", err)
	}
	interim, err := system.GetRun(context.Background(), state.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if interim.Status != types.RunStatusWaitingConfirmation {
		t.Fatalf("interim status = %s", interim.Status)
	}
	if err := system.SubmitToolConfirmation(context.Background(), types.ToolConfirmation{DecisionID: "decision-1", Approved: true}); err != nil {
		t.Fatalf("SubmitToolConfirmation(decision-1) error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	if fakes.tool.executeCount != 2 {
		t.Fatalf("executeCount = %d", fakes.tool.executeCount)
	}
	session := fakes.storage.lastSession()
	if got := completedToolPartCount(session.Messages[1]); got != 2 {
		t.Fatalf("completed tool part count = %d messages=%#v", got, session.Messages)
	}
}

func TestRunPreservesNonErrorToolResultStates(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "need tool", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}}}},
		{ID: "m2", Content: "final"},
	}
	fakes.tool.executeResult = types.ToolResult{ID: "result-cancelled", ActionID: "intent-1", ToolName: "file-reader", Status: types.ToolStatusCancelled, Error: "cancelled by tool", CreatedAt: time.Now().UTC()}
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use tool"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	session := fakes.storage.lastSession()
	part := toolPartByCallID(session.Messages[1], "intent-1")
	if part == nil || part.State != "cancelled" || part.Result == nil || part.Result.Status != types.ToolStatusCancelled {
		t.Fatalf("tool part = %#v", part)
	}
}

func TestRunContinuesWhenToolExecutionReturnsError(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "need tool", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}}}},
		{ID: "m2", Content: "final after tool failure"},
	}
	fakes.tool.executeErr = errors.New("tool process failed")
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use tool"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	session := fakes.storage.lastSession()
	part := toolPartByCallID(session.Messages[1], "intent-1")
	if part == nil || part.State != "error" || part.Result == nil || part.Result.Status != types.ToolStatusFailed || !strings.Contains(part.Result.Error, "tool process failed") {
		t.Fatalf("tool part = %#v", part)
	}
	if got := session.Messages[len(session.Messages)-1].Content; got != "final after tool failure" {
		t.Fatalf("final assistant content = %q", got)
	}
}

func TestRunCancellationDuringToolExecutionDoesNotRecordToolFailure(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "need tool", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}}}},
		{ID: "m2", Content: "should not continue after cancellation"},
	}
	fakes.tool.executeDelay = time.Second
	fakes.tool.executeCancelResult = types.ToolResult{ID: "result-after-cancel", Status: types.ToolStatusFailed, Error: "tool observed cancellation", CreatedAt: time.Now().UTC()}
	system := newTestRuntime(t, fakes, Config{})
	events, unsubscribe, err := system.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use tool"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	fakes.tool.waitExecuteCount(t, 1)
	if err := system.CancelRun(context.Background(), state.ID); err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCancelled {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	session := waitStoredSessionStatus(t, fakes.storage, types.RunStatusCancelled)
	part := toolPartByCallID(session.Messages[1], "intent-1")
	if part == nil {
		t.Fatalf("tool part was not recorded: %#v", session.Messages)
	}
	if part.State != "cancelled" || part.Result == nil || part.Result.Status != types.ToolStatusCancelled {
		t.Fatalf("cancelled run did not settle tool part: %#v", part)
	}
	if part.Result.Status == types.ToolStatusFailed || part.State == "error" {
		t.Fatalf("cancelled run recorded tool failure: %#v", part)
	}
	waitCancelledToolPartUpdate(t, events, state.ID, "intent-1")
	if got := fakes.provider.callCount(); got != 1 {
		t.Fatalf("model call count = %d", got)
	}
}

func waitCancelledToolPartUpdate(t *testing.T, events <-chan types.RunEvent, runID string, callID string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			if event.RunID != runID || event.Type != "assistant_message_update" {
				continue
			}
			payload, ok := event.Payload.(types.RunAssistantMessageUpdate)
			if !ok || payload.Status != types.RunStatusCancelled {
				continue
			}
			part := toolPartByCallID(payload.Message, callID)
			if part != nil && part.State == "cancelled" && part.Result != nil && part.Result.Status == types.ToolStatusCancelled {
				return
			}
		case <-deadline:
			t.Fatalf("cancelled assistant message update did not settle tool part")
		}
	}
}

func TestRunContinuesWhenToolPrepareReturnsError(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.responses = []types.ModelResponse{
		{ID: "m1", Content: "need tool", ToolIntents: []types.ToolIntent{{ID: "intent-1", ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}}}},
		{ID: "m2", Content: "final after prepare failure"},
	}
	fakes.tool.prepareErr = errors.New("tool executable missing")
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use tool"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("status = %s reason=%s", final.Status, final.Reason)
	}
	session := fakes.storage.lastSession()
	part := toolPartByCallID(session.Messages[1], "intent-1")
	if part == nil || part.State != "error" || part.Result == nil || part.Result.Status != types.ToolStatusFailed || !strings.Contains(part.Result.Error, "tool executable missing") {
		t.Fatalf("tool part = %#v", part)
	}
	if got := session.Messages[len(session.Messages)-1].Content; got != "final after prepare failure" {
		t.Fatalf("final assistant content = %q", got)
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

func reasoningPartByType(message types.Message) *types.MessagePart {
	for index := range message.Parts {
		if message.Parts[index].Type == "reasoning" {
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

func TestRunContinuesAcrossManyToolRounds(t *testing.T) {
	fakes := newRuntimeFakes()
	const rounds = 10
	for index := 0; index < rounds; index++ {
		intentID := "intent-" + string(rune('a'+index))
		fakes.provider.responses = append(fakes.provider.responses, types.ModelResponse{ID: "m-tool", Content: "need tool", ToolIntents: []types.ToolIntent{{ID: intentID, ToolName: "file-reader", Arguments: map[string]any{"path": "README.md"}}}})
	}
	fakes.provider.responses = append(fakes.provider.responses, types.ModelResponse{ID: "m-final", Content: "final"})
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "use tools"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusCompleted {
		t.Fatalf("final = %#v", final)
	}
	if fakes.tool.executeCount != rounds {
		t.Fatalf("executeCount = %d, want %d", fakes.tool.executeCount, rounds)
	}
}

func TestRunFailureStoresAssistantErrorWithoutFailureMessage(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.err = apperrors.WrapWithDetails("model-provider-system", "provider.service_failed", "upstream says no", nil, map[string]any{"body": `{"error":{"message":"upstream says no"}}`})
	system := newTestRuntime(t, fakes, Config{})
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "hello"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusFailed || final.Error == nil || final.Error.Message != "failed to complete model request" || final.Error.Cause == nil || final.Error.Cause.Message != "upstream says no" {
		t.Fatalf("final = %#v", final)
	}
	session := fakes.storage.lastSession()
	if len(session.Messages) != 2 {
		t.Fatalf("messages = %#v", session.Messages)
	}
	if session.Messages[1].Type != "assistant" || session.Messages[1].Error == nil || session.Messages[1].Error.Message != "failed to complete model request" || session.Messages[1].Error.Cause == nil || session.Messages[1].Error.Cause.Message != "upstream says no" {
		t.Fatalf("assistant error = %#v", session.Messages[1])
	}
	for _, message := range session.Messages {
		if message.Type == "failure" {
			t.Fatalf("failure message should not be appended: %#v", session.Messages)
		}
	}
}

func TestRunFailsWhenOwnedInputMessageIsExternallyEdited(t *testing.T) {
	fakes := newRuntimeFakes()
	fakes.provider.block = make(chan struct{})
	system := newTestRuntime(t, fakes, Config{})
	events, unsubscribe, err := system.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", Message: "hello"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if state.InputMessageID == "" {
		t.Fatalf("input message id is empty: %#v", state)
	}
	fakes.storage.updateMessageContent(t, "developer", state.SessionID, state.InputMessageID, "user edited")
	close(fakes.provider.block)
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusFailed || !strings.Contains(final.Reason, "message was changed or deleted") {
		t.Fatalf("final = %#v", final)
	}
	session := fakes.storage.lastSession()
	if session.Status != string(types.RunStatusFailed) {
		t.Fatalf("session status after conflict = %q", session.Status)
	}
	if len(session.Messages) != 1 || session.Messages[0].Content != "user edited" {
		t.Fatalf("session messages after conflict = %#v", session.Messages)
	}
	assertNoAssistantUpdateBeforeRunFailed(t, events, state.ID)
}

func TestRunFailsWhenDependencyMessageIsExternallyEdited(t *testing.T) {
	fakes := newRuntimeFakes()
	now := time.Now().UTC()
	fakes.storage.sessions["developer/session-1"] = types.Session{ID: "session-1", RoleID: "developer", Title: "Existing", Status: string(types.RunStatusCompleted), CreatedAt: now, UpdatedAt: now, LastActive: now, Messages: []types.Message{{ID: "u1", Type: "user", Content: "question", BranchID: "main", CreatedAt: now, UpdatedAt: now}}}
	fakes.provider.block = make(chan struct{})
	system := newTestRuntime(t, fakes, Config{})
	events, unsubscribe, err := system.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()
	state, err := system.StartRun(context.Background(), types.RunRequest{RoleID: "developer", SessionID: "session-1", UserMessageID: "u1"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	fakes.storage.updateMessageContent(t, "developer", "session-1", "u1", "edited question")
	close(fakes.provider.block)
	final := waitRun(t, system, state.ID)
	if final.Status != types.RunStatusFailed || !strings.Contains(final.Reason, "message was changed or deleted") {
		t.Fatalf("final = %#v", final)
	}
	session := fakes.storage.sessions["developer/session-1"]
	if session.Status != string(types.RunStatusFailed) {
		t.Fatalf("session status after conflict = %q", session.Status)
	}
	if len(session.Messages) != 1 || session.Messages[0].Content != "edited question" {
		t.Fatalf("session messages after conflict = %#v", session.Messages)
	}
	assertNoAssistantUpdateBeforeRunFailed(t, events, state.ID)
}

func assertNoAssistantUpdateBeforeRunFailed(t *testing.T, events <-chan types.RunEvent, runID string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			if event.RunID != runID {
				continue
			}
			if event.Type == "assistant_message_update" {
				t.Fatalf("assistant update published for unsaved conflict message: %#v", event.Payload)
			}
			if event.Type == "run_failed" {
				return
			}
		case <-deadline:
			t.Fatalf("run_failed event was not published for %s", runID)
		}
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

func waitStoredSessionStatus(t *testing.T, storage *fakeRuntimeStorage, status types.RunStatus) types.Session {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		session := storage.lastSession()
		if session.Status == string(status) {
			return session
		}
		time.Sleep(10 * time.Millisecond)
	}
	session := storage.lastSession()
	t.Fatalf("stored session did not reach status %s, last session = %#v", status, session)
	return types.Session{}
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

func (f *fakeRuntimeStorage) SaveSessionMessages(ctx context.Context, save types.SessionMessageSave) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	session := save.Session
	key := session.RoleID + "/" + session.ID
	merged, ok := f.sessions[key]
	if !ok {
		merged = session
		merged.Messages = nil
	}
	merged.Status = string(save.Status)
	for _, condition := range save.Conditions {
		messageID := strings.TrimSpace(condition.MessageID)
		if messageID == "" && condition.Expected != nil {
			messageID = condition.Expected.ID
		}
		if err := fakeValidateMessageExpected(merged, messageID, condition.Expected); err != nil {
			return err
		}
	}
	for _, delete := range save.Deletes {
		messageID := strings.TrimSpace(delete.MessageID)
		if messageID == "" && delete.Expected != nil {
			messageID = delete.Expected.ID
		}
		if err := fakeValidateMessageExpected(merged, messageID, delete.Expected); err != nil {
			return err
		}
	}
	validated := merged
	for _, delete := range save.Deletes {
		validated = fakeRemoveSessionMessage(validated, delete.MessageID)
	}
	for _, write := range save.Writes {
		if err := fakeValidateMessageExpected(validated, write.Message.ID, write.Expected); err != nil {
			return err
		}
		if fakeShouldValidateMessageBranchSlot(validated, write.Message) {
			if err := fakeValidateMessageBranchSlotAvailable(validated, write.Message); err != nil {
				return err
			}
		}
		validated = fakeUpsertSessionMessage(validated, write.Message)
	}
	merged = validated
	f.sessions[key] = merged
	return nil
}

func fakeValidateMessageExpected(session types.Session, messageID string, expected *types.Message) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return errors.New("message id is required")
	}
	current, ok := messageByID(session.Messages, messageID)
	if expected == nil {
		if ok {
			return apperrors.New("fake-storage", "storage.conflict", "message already exists")
		}
		return nil
	}
	if !ok || !fakeMessageMatchesExpected(current, *expected) {
		return apperrors.New("fake-storage", "storage.conflict", "message was changed or deleted")
	}
	return nil
}

func fakeMessageMatchesExpected(current types.Message, expected types.Message) bool {
	return current.ID == expected.ID && current.Type == expected.Type && strings.TrimSpace(current.ParentMessageID) == strings.TrimSpace(expected.ParentMessageID) && fakeNormalizeBranchID(current.BranchID) == fakeNormalizeBranchID(expected.BranchID) && current.UpdatedAt.Equal(expected.UpdatedAt)
}

func fakeShouldValidateMessageBranchSlot(session types.Session, message types.Message) bool {
	current, ok := messageByID(session.Messages, message.ID)
	if !ok {
		return true
	}
	return !fakeSameMessageBranchSlot(current, message)
}

func fakeValidateMessageBranchSlotAvailable(session types.Session, message types.Message) error {
	messageID := strings.TrimSpace(message.ID)
	if messageID == "" {
		return errors.New("message id is required")
	}
	for _, current := range session.Messages {
		if strings.TrimSpace(current.ID) == messageID {
			continue
		}
		if fakeSameMessageBranchSlot(current, message) {
			return apperrors.New("fake-storage", "storage.conflict", "message branch slot is already occupied")
		}
	}
	return nil
}

func fakeSameMessageBranchSlot(left types.Message, right types.Message) bool {
	return strings.TrimSpace(left.ParentMessageID) == strings.TrimSpace(right.ParentMessageID) && fakeNormalizeBranchID(left.BranchID) == fakeNormalizeBranchID(right.BranchID)
}

func fakeNormalizeBranchID(branchID string) string {
	branchID = strings.TrimSpace(branchID)
	if branchID == "" {
		return defaultRuntimeBranchID
	}
	return branchID
}

func fakeRemoveSessionMessage(session types.Session, messageID string) types.Session {
	messageID = strings.TrimSpace(messageID)
	next := make([]types.Message, 0, len(session.Messages))
	for _, message := range session.Messages {
		if message.ID == messageID {
			continue
		}
		next = append(next, message)
	}
	session.Messages = next
	return session
}

func fakeUpsertSessionMessage(session types.Session, message types.Message) types.Session {
	for index := range session.Messages {
		if session.Messages[index].ID != message.ID {
			continue
		}
		session.Messages[index] = message
		return session
	}
	session.Messages = append(session.Messages, message)
	return session
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

func (f *fakeRuntimeStorage) updateMessageContent(t *testing.T, roleID string, sessionID string, messageID string, content string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	key := roleID + "/" + sessionID
	session, ok := f.sessions[key]
	if !ok {
		t.Fatalf("session %s was not found", key)
	}
	for index := range session.Messages {
		if session.Messages[index].ID != messageID {
			continue
		}
		updatedAt := session.Messages[index].UpdatedAt.Add(time.Second)
		if !updatedAt.After(session.Messages[index].UpdatedAt) {
			updatedAt = time.Now().UTC()
		}
		session.Messages[index].Content = content
		session.Messages[index].UpdatedAt = updatedAt
		f.sessions[key] = session
		return
	}
	t.Fatalf("message %s was not found in %#v", messageID, session.Messages)
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

type fakeRuntimeRoles struct {
	policy types.ToolPolicy
}

func (f *fakeRuntimeRoles) BuildContext(ctx context.Context, roleID string, session types.Session, tools []types.ToolDefinition) (types.RoleContext, error) {
	policy := f.currentPolicy()
	return types.RoleContext{RoleID: roleID, RoleName: "Developer", ModelConfig: types.ModelConfig{Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"}, Temperature: 0.7}, Messages: session.Messages, Tools: tools, NativeTools: fakeRuntimeNativeTools(tools, policy.NativeTools), ToolPolicy: policy}, nil
}

func (f *fakeRuntimeRoles) GetToolPolicy(ctx context.Context, roleID string) (types.ToolPolicy, error) {
	return f.currentPolicy(), nil
}

func (f *fakeRuntimeRoles) currentPolicy() types.ToolPolicy {
	if len(f.policy.Tools) > 0 || len(f.policy.NativeTools) > 0 || len(f.policy.RunModes) > 0 {
		return f.policy
	}
	return types.ToolPolicy{Tools: []string{"file-reader"}, RunModes: map[string]types.ToolRunMode{"file-reader": types.ToolRunAsk}}
}

func fakeRuntimeNativeTools(tools []types.ToolDefinition, names []string) []types.ToolDefinition {
	if len(tools) == 0 || len(names) == 0 {
		return nil
	}
	byName := map[string]types.ToolDefinition{}
	for _, tool := range tools {
		if tool.ID != "" {
			byName[tool.ID] = tool
		}
		if tool.Name != "" {
			byName[tool.Name] = tool
		}
	}
	nativeTools := make([]types.ToolDefinition, 0, len(names))
	for _, name := range names {
		if tool, ok := byName[name]; ok {
			nativeTools = append(nativeTools, tool)
		}
	}
	return nativeTools
}

type fakeRuntimeProvider struct {
	mu              sync.Mutex
	responses       []types.ModelResponse
	streamEvents    []types.ModelStreamEvent
	streamResponse  types.ModelResponse
	streamResponses []types.ModelResponse
	alwaysTool      bool
	calls           int
	requests        []types.ModelRequest
	block           chan struct{}
	err             error
}

func (f *fakeRuntimeProvider) Complete(ctx context.Context, request types.ModelRequest) (types.ModelResponse, error) {
	if f.err != nil {
		return types.ModelResponse{}, f.err
	}
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
	if f.err != nil {
		return types.ModelResponse{}, f.err
	}
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
	if len(f.streamResponses) > 0 {
		response = f.streamResponses[0]
		f.streamResponses = f.streamResponses[1:]
	}
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
	mu                  sync.Mutex
	prepareDecision     types.PermissionDecision
	prepareDecisions    map[string]types.PermissionDecision
	prepareErr          error
	confirmedDecision   types.PermissionDecision
	executeCount        int
	activeExecutions    int
	maxActiveExecutions int
	executeDelay        time.Duration
	executeDelays       map[string]time.Duration
	executeResult       types.ToolResult
	executeCancelResult types.ToolResult
	executeErr          error
	toolSummaries       []types.ToolSummary
	parsedIntents       []types.ToolIntent
	parseErr            error
	normalizedIntents   []types.ToolIntent
}

func newFakeRuntimeTools() *fakeRuntimeTools {
	return &fakeRuntimeTools{prepareDecision: types.PermissionDecision{ID: "decision-1", Status: types.PermissionStatusAllowed}}
}

func (f *fakeRuntimeTools) ParseTextToolRequests(ctx context.Context, content string) ([]types.ToolIntent, error) {
	if f.parseErr != nil {
		return nil, f.parseErr
	}
	if strings.Contains(content, "<<<TOOL_REQUEST>>>") {
		intents := append([]types.ToolIntent(nil), f.parsedIntents...)
		f.parsedIntents = nil
		return intents, nil
	}
	return nil, nil
}

func (f *fakeRuntimeTools) NormalizeIntent(ctx context.Context, intent types.ToolIntent) (types.ToolAction, error) {
	f.normalizedIntents = append(f.normalizedIntents, intent)
	return types.ToolAction{ID: intent.ID, ToolName: intent.ToolName, Arguments: intent.Arguments, Source: intent.Source, Raw: intent.Raw}, nil
}

func (f *fakeRuntimeTools) Prepare(ctx context.Context, roleID string, action types.ToolAction) (types.ToolRunPlan, error) {
	if f.prepareErr != nil {
		return types.ToolRunPlan{}, f.prepareErr
	}
	decision := f.prepareDecision
	if f.prepareDecisions != nil {
		if configured, ok := f.prepareDecisions[action.ID]; ok {
			decision = configured
		}
	}
	if decision.ID == "" {
		decision.ID = "decision-" + action.ID
	}
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
	if decision.Status == types.PermissionStatusDenied {
		plan.PlanStatus = types.ToolPlanStatusDenied
	} else {
		plan.PlanStatus = types.ToolPlanStatusReady
	}
	return plan, nil
}

func (f *fakeRuntimeTools) Execute(ctx context.Context, plan types.ToolRunPlan) (types.ToolResult, error) {
	f.mu.Lock()
	f.executeCount++
	f.activeExecutions++
	if f.activeExecutions > f.maxActiveExecutions {
		f.maxActiveExecutions = f.activeExecutions
	}
	delay := f.executeDelay
	if f.executeDelays != nil {
		if configured, ok := f.executeDelays[plan.Action.ID]; ok {
			delay = configured
		}
	}
	f.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			f.mu.Lock()
			f.activeExecutions--
			result := f.executeCancelResult
			f.mu.Unlock()
			if result.ID != "" {
				result.ActionID = plan.Action.ID
				result.ToolName = plan.Action.ToolName
				return result, nil
			}
			return types.ToolResult{}, ctx.Err()
		}
	}
	f.mu.Lock()
	f.activeExecutions--
	f.mu.Unlock()
	if f.executeResult.ID != "" {
		result := f.executeResult
		result.ActionID = plan.Action.ID
		result.ToolName = plan.Action.ToolName
		return result, nil
	}
	if f.executeErr != nil {
		return types.ToolResult{}, f.executeErr
	}
	return types.ToolResult{ID: "result-1", ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.ToolStatusSuccess, Content: "tool ok", CreatedAt: time.Now().UTC()}, nil
}

func (f *fakeRuntimeTools) waitExecuteCount(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if f.executeCountSnapshot() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("execute count did not reach %d, got %d", want, f.executeCountSnapshot())
}

func (f *fakeRuntimeTools) executeCountSnapshot() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.executeCount
}

func (f *fakeRuntimeTools) LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error) {
	for _, summary := range f.listToolSummaries() {
		if summary.ID == toolID {
			return types.ToolDefinition{ID: summary.ID, Name: summary.Name, Description: summary.Description, Type: summary.Type}, nil
		}
	}
	return types.ToolDefinition{ID: toolID, Name: toolID, Description: "tool", Type: "local"}, nil
}

func (f *fakeRuntimeTools) ListTools(ctx context.Context) ([]types.ToolSummary, error) {
	return f.listToolSummaries(), nil
}

func (f *fakeRuntimeTools) listToolSummaries() []types.ToolSummary {
	if len(f.toolSummaries) > 0 {
		return append([]types.ToolSummary(nil), f.toolSummaries...)
	}
	return []types.ToolSummary{{ID: "file-reader", Name: "file-reader", Description: "Read files", Type: "local"}}
}
