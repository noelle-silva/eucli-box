package agentruntime

import (
	"context"
	"sort"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func (s *system) StartRun(ctx context.Context, request types.RunRequest) (types.RunState, error) {
	if err := validateRunRequest(ctx, request); err != nil {
		return types.RunState{}, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	now := nowUTC()
	state := types.RunState{ID: utils.NewID("run"), RoleID: request.RoleID, SessionID: request.SessionID, Stream: request.Stream, Status: types.RunStatusCreated, CreatedAt: now, UpdatedAt: now}
	record := &runRecord{runID: state.ID, roleID: request.RoleID, state: state, stream: request.Stream, reasoningEffort: types.TrimReasoningEffort(request.ReasoningEffort), cancel: cancel}
	s.mu.Lock()
	s.runs[state.ID] = record
	s.mu.Unlock()
	state, contextSession, err := s.startRun(ctx, record, request)
	if err != nil {
		cancel()
		s.failRunStateOnly(record, err)
		if failed, ok := s.getRunState(record.runID); ok {
			return failed, nil
		}
		return state, nil
	}
	go s.continueRun(runCtx, record, contextSession)
	return state, nil
}

func (s *system) GetRun(ctx context.Context, runID string) (types.RunState, error) {
	if strings.TrimSpace(runID) == "" {
		return types.RunState{}, runtimeInvalid("run id is required", nil)
	}
	state, ok := s.getRunState(runID)
	if !ok {
		return types.RunState{}, runtimeNotFound("run was not found", nil)
	}
	return state, nil
}

func (s *system) ListActiveRuns(ctx context.Context) ([]types.RunState, error) {
	if err := ctx.Err(); err != nil {
		return nil, runtimeInvalid("list runs context is cancelled", err)
	}
	s.mu.Lock()
	states := make([]types.RunState, 0, len(s.runs))
	for _, record := range s.runs {
		if record == nil || !isActiveRunStatus(record.state.Status) {
			continue
		}
		states = append(states, runStateSnapshot(record))
	}
	s.mu.Unlock()
	sort.SliceStable(states, func(i, j int) bool {
		if !states[i].CreatedAt.Equal(states[j].CreatedAt) {
			return states[i].CreatedAt.Before(states[j].CreatedAt)
		}
		return states[i].ID < states[j].ID
	})
	return states, nil
}

func isActiveRunStatus(status types.RunStatus) bool {
	switch status {
	case types.RunStatusCreated, types.RunStatusRunning, types.RunStatusWaitingConfirmation:
		return true
	default:
		return false
	}
}

func (s *system) CancelRun(ctx context.Context, runID string) error {
	s.mu.Lock()
	record, ok := s.runs[runID]
	if !ok {
		s.mu.Unlock()
		return runtimeNotFound("run was not found", nil)
	}
	record.cancel()
	s.mu.Unlock()
	state, err := s.updateRun(runID, types.RunStatusCancelled, "cancelled by user")
	if err != nil {
		return err
	}
	s.publishAssistantMessageUpdate(record)
	s.publish(runID, "run_cancelled", state)
	return nil
}

func (s *system) startRun(ctx context.Context, record *runRecord, request types.RunRequest) (types.RunState, types.Session, error) {
	state, err := s.updateRun(record.runID, types.RunStatusRunning, "")
	if err != nil {
		return state, types.Session{}, err
	}
	s.publish(record.runID, "run_started", state)
	session, err := s.loadOrCreateSession(ctx, request)
	if err != nil {
		return state, types.Session{}, err
	}
	applyRunReasoningEffort(record, &session)
	if record.state.SessionID == "" {
		if err := s.setRunSessionID(record.runID, session.ID); err != nil {
			return state, types.Session{}, err
		}
	}
	session, contextSession, assistantParent, err := s.prepareRunSession(ctx, session, request)
	if err != nil {
		return state, types.Session{}, err
	}
	record.session = session
	record.messageParent = assistantParent
	record.anchorMessageID = assistantParent.ID
	if strings.TrimSpace(request.UserMessageID) == "" {
		markRunInputMessage(record, assistantParent)
	}
	markRunDependencyMessages(record, contextSession.Messages)
	record.forceBranchReply = shouldForceBranchReply(session, assistantParent) || s.hasActiveRunAtAnchor(record)
	lastMessageID := assistantParent.ID
	if err := s.setRunMessageIDs(record.runID, assistantParent.ID, lastMessageID); err != nil {
		return state, types.Session{}, err
	}
	if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
		return state, types.Session{}, err
	}
	state, _ = s.getRunState(record.runID)
	return state, contextSession, nil
}

func (s *system) continueRun(ctx context.Context, record *runRecord, contextSession types.Session) {
	assistantParent := record.messageParent
	for {
		if err := ctx.Err(); err != nil {
			s.cancelRunRecord(context.Background(), record, record.session)
			return
		}
		roleContext, err := s.buildRoleContext(ctx, record.roleID, contextSession)
		if err != nil {
			s.failRun(context.Background(), record, record.session, err)
			return
		}
		modelResponse, err := s.callModel(ctx, record, roleContext)
		if err != nil {
			s.failRun(context.Background(), record, record.session, err)
			return
		}
		modelResponse, err = s.mergeTextToolRequests(ctx, modelResponse)
		if err != nil {
			s.failRun(context.Background(), record, record.session, err)
			return
		}
		if err := ctx.Err(); err != nil {
			s.cancelRunRecord(context.Background(), record, record.session)
			return
		}
		assistantOutputRecorded := shouldRecordAssistantOutput(modelResponse)
		if assistantOutputRecorded {
			if record.messageParent.Type != "assistant" {
				appendRunAssistantReply(record, modelResponse.Content)
			} else {
				updateRunAssistantContent(record, modelResponse.Content)
			}
			if strings.TrimSpace(modelResponse.Reasoning) != "" || strings.TrimSpace(modelResponse.ReasoningSignature) != "" || strings.TrimSpace(modelResponse.ReasoningData) != "" {
				updateRunAssistantReasoning(record, modelResponse.Reasoning, modelResponse.ReasoningSource, modelResponse.ReasoningSignature, modelResponse.ReasoningData)
			}
		} else {
			dropEmptyAssistantOutput(record)
		}
		assistantParent = record.messageParent
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, assistantParent.ID); err != nil {
			s.failRun(context.Background(), record, record.session, err)
			return
		}
		if assistantOutputRecorded {
			contextSession = appendMessage(contextSession, assistantParent)
		}
		s.publish(record.runID, "model_output", modelResponse)
		if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
			s.failRun(context.Background(), record, record.session, err)
			return
		}
		if assistantOutputRecorded {
			s.publishAssistantMessageUpdate(record)
		}
		if len(modelResponse.ToolIntents) == 0 {
			s.completeRun(context.Background(), record, record.session)
			return
		}
		_, err = s.handleToolIntents(ctx, record, modelResponse.ToolIntents)
		if err != nil {
			if ctx.Err() != nil {
				s.cancelRunRecord(context.Background(), record, record.session)
				return
			}
			s.failRun(context.Background(), record, record.session, err)
			return
		}
		assistantParent = record.messageParent
		contextSession = upsertSessionMessage(contextSession, assistantParent)
		record.activeAssistantID = ""
		if err := ctx.Err(); err != nil {
			s.cancelRunRecord(context.Background(), record, record.session)
			return
		}
		if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
			s.failRun(context.Background(), record, record.session, err)
			return
		}
	}
}

func shouldRecordAssistantOutput(response types.ModelResponse) bool {
	return strings.TrimSpace(response.Content) != "" || strings.TrimSpace(response.Reasoning) != "" || strings.TrimSpace(response.ReasoningSignature) != "" || strings.TrimSpace(response.ReasoningData) != "" || len(response.ToolIntents) == 0
}

func (s *system) mergeTextToolRequests(ctx context.Context, response types.ModelResponse) (types.ModelResponse, error) {
	textIntents, err := s.tools.ParseTextToolRequests(ctx, response.Content)
	if err != nil {
		return types.ModelResponse{}, runtimeToolFailed("failed to parse text tool requests", err)
	}
	if len(textIntents) > 0 {
		response.ToolIntents = append(response.ToolIntents, textIntents...)
	}
	return response, nil
}

func validateRunRequest(ctx context.Context, request types.RunRequest) error {
	if err := ctx.Err(); err != nil {
		return runtimeInvalid("start run context is cancelled", err)
	}
	if strings.TrimSpace(request.RoleID) == "" {
		return runtimeInvalid("role id is required", nil)
	}
	hasAttachments := len(request.Attachments) > 0
	hasMessage := strings.TrimSpace(request.Message) != "" || hasAttachments
	hasUserMessageID := strings.TrimSpace(request.UserMessageID) != ""
	if hasMessage == hasUserMessageID {
		return runtimeInvalid("exactly one of message or userMessageId is required", nil)
	}
	if hasUserMessageID && strings.TrimSpace(request.ParentMessageID) != "" {
		return runtimeInvalid("parentMessageId cannot be combined with userMessageId", nil)
	}
	if hasUserMessageID && hasAttachments {
		return runtimeInvalid("attachments cannot be combined with userMessageId", nil)
	}
	if hasUserMessageID && strings.TrimSpace(request.SessionID) == "" {
		return runtimeInvalid("session id is required when userMessageId is provided", nil)
	}
	if strings.TrimSpace(request.ParentMessageID) != "" && strings.TrimSpace(request.SessionID) == "" {
		return runtimeInvalid("session id is required when parentMessageId is provided", nil)
	}
	if effort := types.TrimReasoningEffort(request.ReasoningEffort); effort != "" && !types.IsReasoningEffort(effort) {
		return runtimeInvalid("reasoningEffort is invalid", nil)
	}
	return nil
}

func applyRunReasoningEffort(record *runRecord, session *types.Session) {
	if record == nil || session == nil {
		return
	}
	const key = "reasoningEffort"
	effort := types.TrimReasoningEffort(record.reasoningEffort)
	if effort == "" && session.Metadata != nil {
		effort = types.TrimReasoningEffort(types.ReasoningEffort(session.Metadata[key]))
	}
	if effort == "" {
		return
	}
	record.reasoningEffort = effort
	if session.Metadata == nil {
		session.Metadata = map[string]string{}
	}
	if session.Metadata[key] != string(effort) {
		record.reasoningPersistPending = true
	}
	session.Metadata[key] = string(effort)
}

func (s *system) prepareRunSession(ctx context.Context, session types.Session, request types.RunRequest) (types.Session, types.Session, types.Message, error) {
	if strings.TrimSpace(request.UserMessageID) == "" {
		var err error
		session, err = s.appendUserMessageForRun(ctx, session, request)
		if err != nil {
			return session, types.Session{}, types.Message{}, err
		}
		parent := lastSessionMessage(session)
		contextSession, _, err := sessionContextThroughMessage(session, parent.ID)
		if err != nil {
			return session, types.Session{}, types.Message{}, err
		}
		return session, contextSession, parent, nil
	}
	contextSession, parent, err := sessionContextThroughMessage(session, request.UserMessageID)
	if err != nil {
		return session, types.Session{}, types.Message{}, err
	}
	return session, contextSession, parent, nil
}

func lastSessionMessage(session types.Session) types.Message {
	if len(session.Messages) == 0 {
		return types.Message{}
	}
	return session.Messages[len(session.Messages)-1]
}

func appendSessionMessages(session types.Session, messages []types.Message) types.Session {
	for _, message := range messages {
		session = appendMessage(session, message)
	}
	return session
}

func upsertSessionMessage(session types.Session, message types.Message) types.Session {
	if strings.TrimSpace(message.ID) == "" {
		return session
	}
	for index := range session.Messages {
		if session.Messages[index].ID != message.ID {
			continue
		}
		session.Messages[index] = message
		return session
	}
	return appendMessage(session, message)
}

func (s *system) completeRun(ctx context.Context, record *runRecord, session types.Session) {
	if err := s.saveRunSession(ctx, record, types.RunStatusCompleted); err != nil {
		reason, payload := runFailureFromError(err, "save session failed: "+err.Error())
		state, _ := s.updateRunWithError(record.runID, types.RunStatusFailed, reason, payload)
		s.publish(record.runID, "run_failed", state)
		return
	}
	state, err := s.updateRun(record.runID, types.RunStatusCompleted, "")
	if err != nil {
		return
	}
	s.publishAssistantMessageUpdate(record)
	s.publish(record.runID, "run_completed", state)
}

func (s *system) failRun(ctx context.Context, record *runRecord, session types.Session, err error) {
	reason, payload := runFailureFromError(err, "")
	s.failRunWithPayload(ctx, record, session, reason, payload)
}

func (s *system) failRunMessage(ctx context.Context, record *runRecord, session types.Session, reason string) {
	reason, payload := runFailureFromError(nil, reason)
	s.failRunWithPayload(ctx, record, session, reason, payload)
}

func (s *system) failRunWithPayload(ctx context.Context, record *runRecord, session types.Session, reason string, payload *types.ErrorPayload) {
	state, err := s.updateRunWithError(record.runID, types.RunStatusFailed, reason, payload)
	if err != nil {
		if session.ID != "" {
			session = markRunFailureMessage(record, session, payload)
			_ = s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID)
			_ = s.saveRunSession(ctx, record, types.RunStatus(session.Status))
		}
		s.publish(record.runID, "run_failed", types.RunState{ID: record.runID, InputMessageID: record.inputMessageID, LastMessageID: record.lastMessageID, Status: types.RunStatusFailed, Reason: reason, Error: cloneErrorPayload(payload)})
		return
	}
	if session.ID != "" {
		session = markRunFailureMessage(record, session, payload)
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err == nil {
			if next, ok := s.getRunState(record.runID); ok {
				state = next
			}
		}
		messagesSaved, saveErr := s.saveRunSessionWithStatusFallback(ctx, record, types.RunStatusFailed)
		if saveErr != nil {
			saveReason, savePayload := runFailureFromError(saveErr, "save session failed: "+saveErr.Error())
			s.publish(record.runID, "run_failed", types.RunState{ID: record.runID, Status: types.RunStatusFailed, Reason: saveReason, Error: cloneErrorPayload(savePayload)})
			return
		}
		if messagesSaved {
			s.publishAssistantMessageUpdate(record)
		}
	} else {
		s.publishAssistantMessageUpdate(record)
	}
	s.publish(record.runID, "run_failed", state)
}

func (s *system) failRunStateOnly(record *runRecord, err error) {
	reason, payload := runFailureFromError(err, "")
	state, updateErr := s.updateRunWithError(record.runID, types.RunStatusFailed, reason, payload)
	if updateErr != nil {
		s.publish(record.runID, "run_failed", types.RunState{ID: record.runID, Status: types.RunStatusFailed, Reason: reason, Error: cloneErrorPayload(payload)})
		return
	}
	s.publish(record.runID, "run_failed", state)
}

func (s *system) cancelRunRecord(ctx context.Context, record *runRecord, session types.Session) {
	state, err := s.updateRun(record.runID, types.RunStatusCancelled, "cancelled")
	if err != nil {
		return
	}
	if cancelRunToolParts(record, "cancelled by user") {
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err == nil {
			if next, ok := s.getRunState(record.runID); ok {
				state = next
			}
		}
	}
	messagesSaved := false
	if session.ID != "" {
		var saveErr error
		messagesSaved, saveErr = s.saveRunSessionWithStatusFallback(ctx, record, types.RunStatusCancelled)
		if saveErr != nil {
			s.publish(record.runID, "run_failed", types.RunState{ID: record.runID, Status: types.RunStatusFailed, Reason: "save session failed: " + saveErr.Error()})
			return
		}
	}
	if messagesSaved || session.ID == "" {
		s.publishAssistantMessageUpdate(record)
	}
	s.publish(record.runID, "run_cancelled", state)
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
