package agentruntime

import (
	"context"
	"fmt"
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
	compactRun := isCompactRunRequest(request)
	stream := (request.Stream == nil || *request.Stream) && !compactRun
	runCtx, cancel := context.WithCancel(context.Background())
	now := nowUTC()
	state := types.RunState{ID: utils.NewID("run"), RoleID: request.RoleID, GroupID: strings.TrimSpace(request.GroupID), WorkspaceID: strings.TrimSpace(request.WorkspaceID), SessionID: request.SessionID, Stream: stream, Status: types.RunStatusCreated, CreatedAt: now, UpdatedAt: now}
	modelOverride, _ := types.NormalizeModelOverrideCoordinate(modelOverrideFromRunRequest(request))
	record := &runRecord{runID: state.ID, roleID: request.RoleID, groupID: state.GroupID, workspaceID: state.WorkspaceID, state: state, stream: stream, streamInput: request.Stream, isCompactRun: compactRun, modelOverride: modelOverride, reasoningEffort: types.TrimReasoningEffort(request.ReasoningEffort), hookPromptSelection: types.NormalizeHookPromptSelection(request.HookPromptMode, request.HookPromptPresetID), hookPromptSelectionInput: hasHookPromptSelectionInput(request), cancel: cancel, inbox: newRunInbox()}
	if compactRun {
		record.commandName = compactCommandName
	}
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
	if compactRun {
		go s.continueCompactRun(runCtx, record, contextSession)
	} else {
		go s.continueRun(runCtx, record, contextSession)
	}
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

// CancelRun 只发出取消信号并登记停止意图：
// 消息收尾、状态落定、保存与终态通知都由该运行自己的处理队列完成，
// 接口本身不修改会话状态、不广播终态。
func (s *system) CancelRun(ctx context.Context, runID string) error {
	s.mu.Lock()
	record, ok := s.runs[runID]
	if !ok {
		s.mu.Unlock()
		return runtimeNotFound("run was not found", nil)
	}
	if !isActiveRunStatus(record.state.Status) {
		s.mu.Unlock()
		return runtimeStateInvalid("invalid run state transition", nil)
	}
	record.cancel()
	enqueueRunEvent(record, runEvent{kind: runEventStop})
	s.mu.Unlock()
	return nil
}

func (s *system) startRun(ctx context.Context, record *runRecord, request types.RunRequest) (types.RunState, types.Session, error) {
	origin := runOriginFromRequest(request)
	record.origin = origin
	state, err := s.updateRun(record.runID, types.RunStatusRunning, "")
	if err != nil {
		return state, types.Session{}, err
	}
	s.publish(eventSourceFromRecord(record), "run_started", state)
	compactRun := record.commandName == compactCommandName
	session, err := s.loadOrCreateSession(ctx, request)
	if err != nil {
		return state, types.Session{}, err
	}
	if compactRun && strings.TrimSpace(request.SessionID) == "" {
		return state, types.Session{}, runtimeInvalid("当前没有可压缩的会话", nil)
	}
	applyRunReasoningEffort(record, &session)
	applyRunModelOverride(record, &session)
	applyRunHookPromptPreset(record, &session)
	applyRunStreamPreference(record, &session)
	if record.state.SessionID == "" {
		if err := s.setRunSessionID(record.runID, session.ID); err != nil {
			return state, types.Session{}, err
		}
	}
	var contextSession types.Session
	var assistantParent types.Message
	if compactRun {
		contextSession, assistantParent, err = prepareCompactRunSession(session, request)
	} else {
		session, contextSession, assistantParent, err = s.prepareRunSession(ctx, session, request)
		contextSession = compactedContextSession(contextSession)
	}
	if err != nil {
		return state, types.Session{}, err
	}
	record.session = session
	record.messageParent = assistantParent
	record.anchorMessageID = assistantParent.ID
	if !compactRun && runOriginMarksInputMessage(origin) {
		markRunInputMessage(record, assistantParent)
	}
	markRunDependencyMessages(record, contextSession.Messages)
	if compactRun && s.hasActiveRunInDependencyPath(record) {
		return state, types.Session{}, runtimeStateInvalid("当前分支路径仍有运行中的任务，请完成后再压缩", nil)
	}
	// 「从会话末条」起点的语义是接在当前活动分支最新之后、不分叉：
	// 残余并发由分支槽冲突兜住，因此不再以「同锚点有活动运行」为由另起分支。
	record.forceBranchReply = origin != types.RunOriginSessionTail && (shouldForceRunBranchReply(session, assistantParent, origin) || s.hasActiveRunAtAnchor(record))
	record.forceNewAssistantReply = runOriginForcesNewAssistantReply(origin)
	lastMessageID := assistantParent.ID
	inputMessageID := assistantParent.ID
	if compactRun {
		inputMessageID = ""
	}
	if err := s.setRunMessageIDs(record.runID, inputMessageID, lastMessageID); err != nil {
		return state, types.Session{}, err
	}
	if !compactRun {
		if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
			return state, types.Session{}, err
		}
	}
	state, _ = s.getRunState(record.runID)
	return state, contextSession, nil
}

func applyRunHookPromptPreset(record *runRecord, session *types.Session) {
	if record == nil || session == nil {
		return
	}
	current := types.HookPromptSelectionFromSessionMetadata(session.Metadata)
	if !record.hookPromptSelectionInput {
		record.hookPromptSelection = current
		return
	}
	next := types.NormalizeHookPromptSelection(record.hookPromptSelection.Mode, record.hookPromptSelection.PresetID)
	record.hookPromptSelection = next
	if !types.SameHookPromptSelection(current, next) {
		record.hookPromptPersistPending = true
	}
	session.Metadata = types.PutHookPromptSessionMetadata(session.Metadata, next)
}

// applyRunStreamPreference 流式缺省归会话事实：请求未显式指定时读取会话的流式标记。
func applyRunStreamPreference(record *runRecord, session *types.Session) {
	if record == nil || session == nil || record.streamInput != nil {
		return
	}
	stream := types.StreamEnabledFromSessionMetadata(session.Metadata)
	if record.isCompactRun {
		stream = false
	}
	record.stream = stream
	record.state.Stream = stream
}

func hasHookPromptSelectionInput(request types.RunRequest) bool {
	return strings.TrimSpace(request.HookPromptMode) != "" || strings.TrimSpace(request.HookPromptPresetID) != ""
}

func (s *system) continueRun(ctx context.Context, record *runRecord, contextSession types.Session) {
	// 主脑协程顶层兜底：任何未预期崩溃都按既有失败收口路径进入失败终态，绝不把运行留在「运行中」。
	defer func() {
		if recovered := recover(); recovered != nil {
			s.failRun(context.Background(), record, record.session, fmt.Errorf("agent run panicked: %v", recovered))
		}
	}()
	for attempt := 0; ; attempt++ {
		retry, err := s.runConversation(ctx, record, &contextSession)
		if err == nil {
			return
		}
		if !retry || attempt >= maxSessionTailRunRetries {
			s.failRun(context.Background(), record, record.session, err)
			return
		}
		next, resetErr := s.resetSessionTailRun(ctx, record)
		if resetErr != nil {
			s.failRun(context.Background(), record, record.session, resetErr)
			return
		}
		contextSession = next
	}
}

// maxSessionTailRunRetries 是「从会话末条」起点因并发分支槽冲突而重解析起点的最大重试次数。
const maxSessionTailRunRetries = 3

// runConversation 推进一次会话循环。返回 (retry, err)：err 非空且 retry 为真表示遭遇
// 可重试的分支槽冲突，调用方应重新载入会话、重解析起点后重试；
// 其余终态出口都在内部收口并返回 (false, nil)。
func (s *system) runConversation(ctx context.Context, record *runRecord, contextSession *types.Session) (bool, error) {
	assistantParent := record.messageParent
	for {
		if s.drainRunInbox(record) || ctx.Err() != nil {
			s.cancelRunRecord(context.Background(), record, record.session)
			return false, nil
		}
		flushedAsyncToolResults, err := s.flushAsyncToolResults(ctx, record, contextSession)
		if err != nil {
			if s.retryableSessionTailConflict(record, err) {
				return true, err
			}
			s.failRun(context.Background(), record, record.session, err)
			return false, nil
		}
		if flushedAsyncToolResults {
			continue
		}
		roleContext, err := s.buildRoleContext(ctx, record.roleID, *contextSession)
		if err != nil {
			s.failRun(context.Background(), record, record.session, err)
			return false, nil
		}
		modelResponse, err := s.callModel(ctx, record, roleContext)
		if err != nil {
			if ctx.Err() != nil {
				s.cancelRunRecord(context.Background(), record, record.session)
				return false, nil
			}
			s.failRun(context.Background(), record, record.session, err)
			return false, nil
		}
		if err := ctx.Err(); err != nil {
			s.cancelRunRecord(context.Background(), record, record.session)
			return false, nil
		}
		assistantOutputRecorded := shouldRecordAssistantOutput(modelResponse)
		if assistantOutputRecorded {
			if record.forceNewAssistantReply || record.messageParent.Type != "assistant" {
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
		if assistantOutputRecorded {
			applyRunModelTiming(record)
		}
		assistantParent = record.messageParent
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, assistantParent.ID); err != nil {
			s.failRun(context.Background(), record, record.session, err)
			return false, nil
		}
		if assistantOutputRecorded {
			*contextSession = appendMessage(*contextSession, assistantParent)
		}
		s.publish(eventSourceFromRecord(record), "model_output", modelResponse)
		if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
			if s.retryableSessionTailConflict(record, err) {
				return true, err
			}
			s.failRun(context.Background(), record, record.session, err)
			return false, nil
		}
		if assistantOutputRecorded {
			s.publishAssistantMessageUpdate(record)
		}
		if len(modelResponse.ToolIntents) == 0 {
			flushedAsyncToolResults, err := s.flushAsyncToolResults(ctx, record, contextSession)
			if err != nil {
				if s.retryableSessionTailConflict(record, err) {
					return true, err
				}
				s.failRun(context.Background(), record, record.session, err)
				return false, nil
			}
			if flushedAsyncToolResults {
				continue
			}
			s.completeRun(context.Background(), record, record.session)
			return false, nil
		}
		_, err = s.handleToolIntents(ctx, record, contextSession, modelResponse.ToolIntents)
		if err != nil {
			if ctx.Err() != nil {
				s.cancelRunRecord(context.Background(), record, record.session)
				return false, nil
			}
			if s.retryableSessionTailConflict(record, err) {
				return true, err
			}
			s.failRun(context.Background(), record, record.session, err)
			return false, nil
		}
		// 工具批可能在等待期间就地回灌过异步结果；回复锚点统一取会话当前末条，
		// 让紧随其后的模型回复接在最新事实上。
		assistantParent = lastSessionMessage(record.session)
		record.messageParent = assistantParent
		*contextSession = upsertSessionMessage(*contextSession, assistantParent)
		record.activeAssistantID = ""
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, assistantParent.ID); err != nil {
			s.failRun(context.Background(), record, record.session, err)
			return false, nil
		}
		if err := ctx.Err(); err != nil {
			s.cancelRunRecord(context.Background(), record, record.session)
			return false, nil
		}
		if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
			if s.retryableSessionTailConflict(record, err) {
				return true, err
			}
			s.failRun(context.Background(), record, record.session, err)
			return false, nil
		}
	}
}

// retryableSessionTailConflict 判定一次落盘冲突是否应触发「从会话末条」起点重解析重试。
// 只在运行尚未落定自有助手回复时重试，避免把已提交的回复重新生成一遍；
// 异步结果等非助手产出已落盘不影响重试，它们幂等且不会重复。
func (s *system) retryableSessionTailConflict(record *runRecord, err error) bool {
	if record == nil || record.origin != types.RunOriginSessionTail || record.persistedAssistantOutput {
		return false
	}
	return isStorageConflict(err)
}

// resetSessionTailRun 在「从会话末条」起点遭遇并发分支槽冲突后，重新载入会话、重解析起点，
// 丢弃本次未落定的运行副本，让运行从最新事实上重试。
func (s *system) resetSessionTailRun(ctx context.Context, record *runRecord) (types.Session, error) {
	request := types.RunRequest{RoleID: record.roleID, GroupID: record.groupID, WorkspaceID: record.workspaceID, SessionID: record.session.ID, Origin: types.RunOriginSessionTail}
	session, err := s.loadOrCreateSession(ctx, request)
	if err != nil {
		return types.Session{}, err
	}
	applyRunReasoningEffort(record, &session)
	applyRunModelOverride(record, &session)
	applyRunHookPromptPreset(record, &session)
	applyRunStreamPreference(record, &session)
	session, contextSession, assistantParent, err := s.prepareRunSession(ctx, session, request)
	if err != nil {
		return types.Session{}, err
	}
	contextSession = compactedContextSession(contextSession)
	record.session = session
	record.messageParent = assistantParent
	record.anchorMessageID = assistantParent.ID
	record.activeAssistantID = ""
	record.ownedMessageIDs = nil
	record.deletedMessageIDs = nil
	record.dependencyIDs = nil
	record.messageSnapshots = nil
	s.discardToolConfirmations(record)
	record.forceBranchReply = false
	record.forceNewAssistantReply = true
	markRunDependencyMessages(record, contextSession.Messages)
	if err := s.setRunMessageIDs(record.runID, assistantParent.ID, assistantParent.ID); err != nil {
		return types.Session{}, err
	}
	return contextSession, nil
}

func shouldRecordAssistantOutput(response types.ModelResponse) bool {
	return strings.TrimSpace(response.Content) != "" || strings.TrimSpace(response.Reasoning) != "" || strings.TrimSpace(response.ReasoningSignature) != "" || strings.TrimSpace(response.ReasoningData) != "" || len(response.ToolIntents) == 0
}

func validateRunRequest(ctx context.Context, request types.RunRequest) error {
	if err := ctx.Err(); err != nil {
		return runtimeInvalid("start run context is cancelled", err)
	}
	if strings.TrimSpace(request.RoleID) == "" {
		return runtimeInvalid("role id is required", nil)
	}
	if strings.TrimSpace(request.GroupID) != "" {
		if _, err := cleanRuntimeID(request.GroupID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(request.WorkspaceID) != "" {
		if _, err := cleanRuntimeID(request.WorkspaceID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(request.GroupID) != "" && strings.TrimSpace(request.WorkspaceID) != "" {
		return runtimeInvalid("groupId cannot be combined with workspaceId", nil)
	}
	if effort := types.TrimReasoningEffort(request.ReasoningEffort); effort != "" && !types.IsReasoningEffort(effort) {
		return runtimeInvalid("reasoningEffort is invalid", nil)
	}
	if override := modelOverrideFromRunRequest(request); hasModelOverrideInput(override) {
		if _, ok := types.NormalizeModelOverrideCoordinate(override); !ok {
			return runtimeInvalid("modelOverride is invalid", nil)
		}
	}
	if runOriginFromRequest(request) == types.RunOriginSessionTail {
		return validateSessionTailRunRequest(request)
	}
	hasAttachments := len(request.Attachments) > 0
	hasMessage := strings.TrimSpace(request.Message) != "" || hasAttachments
	hasUserMessageID := strings.TrimSpace(request.UserMessageID) != ""
	hasContextMessageID := strings.TrimSpace(request.ContextMessageID) != ""
	if runInputCount(hasMessage, hasUserMessageID, hasContextMessageID) != 1 {
		return runtimeInvalid("exactly one of message, userMessageId, or contextMessageId is required", nil)
	}
	if err := validateRunSlashCommand(request); err != nil {
		return err
	}
	if (hasUserMessageID || hasContextMessageID) && strings.TrimSpace(request.ParentMessageID) != "" {
		return runtimeInvalid("parentMessageId cannot be combined with userMessageId or contextMessageId", nil)
	}
	if (hasUserMessageID || hasContextMessageID) && hasAttachments {
		return runtimeInvalid("attachments cannot be combined with userMessageId or contextMessageId", nil)
	}
	if (hasUserMessageID || hasContextMessageID) && strings.TrimSpace(request.SessionID) == "" {
		return runtimeInvalid("session id is required when userMessageId or contextMessageId is provided", nil)
	}
	if strings.TrimSpace(request.ParentMessageID) != "" && strings.TrimSpace(request.SessionID) == "" {
		return runtimeInvalid("session id is required when parentMessageId is provided", nil)
	}
	return nil
}

// validateSessionTailRunRequest 校验「从会话末条」起点：它只声明会话，不携带任何消息输入。
func validateSessionTailRunRequest(request types.RunRequest) error {
	if strings.TrimSpace(request.SessionID) == "" {
		return runtimeInvalid("session id is required when starting from session tail", nil)
	}
	if strings.TrimSpace(request.Message) != "" || len(request.Attachments) > 0 || strings.TrimSpace(request.UserMessageID) != "" || strings.TrimSpace(request.ContextMessageID) != "" || strings.TrimSpace(request.ParentMessageID) != "" {
		return runtimeInvalid("session tail run must not carry message input", nil)
	}
	return nil
}

func cleanRuntimeID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", runtimeInvalid("id is required", nil)
	}
	if strings.Contains(value, "/") || strings.Contains(value, "\\") || value == "." || value == ".." {
		return "", runtimeInvalid("id is invalid", nil)
	}
	return value, nil
}

func modelOverrideFromRunRequest(request types.RunRequest) types.ModelCoordinate {
	if request.ModelOverride == nil {
		return types.ModelCoordinate{}
	}
	return *request.ModelOverride
}

func hasModelOverrideInput(coordinate types.ModelCoordinate) bool {
	return strings.TrimSpace(coordinate.Kind) != "" || strings.TrimSpace(coordinate.ProviderID) != "" || strings.TrimSpace(coordinate.GroupID) != "" || strings.TrimSpace(coordinate.ModelID) != ""
}

func runInputCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func applyRunReasoningEffort(record *runRecord, session *types.Session) {
	if record == nil || session == nil {
		return
	}
	const key = types.SessionMetadataReasoningEffort
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

func applyRunModelOverride(record *runRecord, session *types.Session) {
	if record == nil || session == nil {
		return
	}
	if normalized, ok := types.NormalizeModelOverrideCoordinate(record.modelOverride); ok {
		record.modelOverride = normalized
		if current, currentOK := types.ModelOverrideFromSessionMetadata(session.Metadata); !currentOK || !types.SameModelOverrideCoordinate(current, normalized) {
			record.modelOverridePersistPending = true
		}
		session.Metadata = types.PutModelOverrideSessionMetadata(session.Metadata, normalized)
		return
	}
	if current, ok := types.ModelOverrideFromSessionMetadata(session.Metadata); ok {
		record.modelOverride = current
	}
}

func (s *system) prepareRunSession(ctx context.Context, session types.Session, request types.RunRequest) (types.Session, types.Session, types.Message, error) {
	switch runOriginFromRequest(request) {
	case types.RunOriginSessionTail:
		// 锚点在本函数的同一次会话读取内解析，调用方不预计算、不传入锚点消息 ID。
		parent := activeSessionBranchTail(session)
		if strings.TrimSpace(parent.ID) == "" {
			return session, types.Session{}, types.Message{}, runtimeInvalid("session has no message to continue from", nil)
		}
		contextSession, _, err := sessionContextThroughMessage(session, parent.ID)
		if err != nil {
			return session, types.Session{}, types.Message{}, err
		}
		return session, contextSession, parent, nil
	case types.RunOriginContextMessage:
		contextSession, parent, err := sessionContextThroughMessage(session, request.ContextMessageID)
		if err != nil {
			return session, types.Session{}, types.Message{}, err
		}
		return session, contextSession, parent, nil
	case types.RunOriginUserMessage:
		contextSession, parent, err := sessionContextThroughUserMessage(session, request.UserMessageID)
		if err != nil {
			return session, types.Session{}, types.Message{}, err
		}
		return session, contextSession, parent, nil
	default:
		session, err := s.appendUserMessageForRun(ctx, session, request)
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
}

func shouldForceRunBranchReply(session types.Session, parent types.Message, origin types.RunOrigin) bool {
	if origin == types.RunOriginContextMessage {
		return hasAnyChild(session.Messages, parent.ID)
	}
	return shouldForceBranchReply(session, parent)
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
	defer s.finalizeRun(record)
	s.settleRunTerminal(record, runTerminalIntent{status: types.RunStatusCompleted}, func() (string, *types.ErrorPayload) {
		if err := s.saveRunSession(ctx, record, types.RunStatusCompleted); err != nil {
			return runFailureFromError(err, "save session failed: "+err.Error())
		}
		s.publishAssistantMessageUpdate(record)
		return "", nil
	})
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
	defer s.finalizeRun(record)
	s.settleRunTerminal(record, runTerminalIntent{status: types.RunStatusFailed, reason: reason, payload: payload}, func() (string, *types.ErrorPayload) {
		if session.ID == "" {
			s.publishAssistantMessageUpdate(record)
			return "", nil
		}
		// 运行失败即收口：未决工具统一落定为失败，不留在未决状态。
		failRunToolParts(record, reason)
		session = markRunFailureMessage(record, session, payload)
		_ = s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID)
		messagesSaved, saveErr := s.saveRunSessionWithStatusFallback(ctx, record, types.RunStatusFailed)
		if saveErr != nil {
			return runFailureFromError(saveErr, "save session failed: "+saveErr.Error())
		}
		if messagesSaved {
			s.publishAssistantMessageUpdate(record)
		}
		return "", nil
	})
}

func (s *system) failRunStateOnly(record *runRecord, err error) {
	defer s.finalizeRun(record)
	reason, payload := runFailureFromError(err, "")
	s.settleRunTerminal(record, runTerminalIntent{status: types.RunStatusFailed, reason: reason, payload: payload}, nil)
}

func (s *system) cancelRunRecord(ctx context.Context, record *runRecord, session types.Session) {
	defer s.finalizeRun(record)
	s.settleRunTerminal(record, runTerminalIntent{status: types.RunStatusCancelled, reason: "cancelled"}, func() (string, *types.ErrorPayload) {
		if cancelRunToolParts(record, "cancelled by user") {
			_ = s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID)
		}
		messagesSaved := false
		if session.ID != "" {
			var saveErr error
			messagesSaved, saveErr = s.saveRunSessionWithStatusFallback(ctx, record, types.RunStatusCancelled)
			if saveErr != nil {
				return runFailureFromError(saveErr, "save session failed: "+saveErr.Error())
			}
		}
		if messagesSaved || session.ID == "" {
			s.publishAssistantMessageUpdate(record)
		}
		return "", nil
	})
}

// terminalRunEventType 由已落定状态唯一派生终态事件名；调用方不得自行指定事件名。
func terminalRunEventType(status types.RunStatus) string {
	switch status {
	case types.RunStatusCompleted:
		return "run_completed"
	case types.RunStatusCancelled:
		return "run_cancelled"
	default:
		return "run_failed"
	}
}

// runTerminalIntent 是运行终态的意图：状态在收尾前先定案，终态事件名由该状态唯一派生，
// 调用方不得自行指定事件名。
type runTerminalIntent struct {
	status  types.RunStatus
	reason  string
	payload *types.ErrorPayload
}

// settleRunTerminal 是运行终态的唯一收口权威：以「运行仍处于活动态」为认领闸门做一次性认领，
// 只有第一个认领成功的调用者负责落定终态、执行收尾落盘并派生发布终态事件；后续到达的收口请求
// 静默吸收，绝不发布第二条终态事件。终态意图在收尾前先定案，收尾落盘失败不改变已定终态，
// 失败原因随终态事件携带。认领成功后终态事件必达：即便收尾途中崩溃，也在展开时补发这一条。
func (s *system) settleRunTerminal(record *runRecord, intent runTerminalIntent, wrapUp func() (string, *types.ErrorPayload)) {
	if record == nil {
		return
	}
	s.mu.Lock()
	if !isActiveRunStatus(record.state.Status) {
		s.mu.Unlock()
		return
	}
	record.state.Status = intent.status
	record.state.Reason = intent.reason
	record.state.Retry = nil
	record.state.Error = cloneErrorPayload(intent.payload)
	record.state.UpdatedAt = nowUTC()
	s.mu.Unlock()
	// 认领一旦成功，这条终态事件就由本调用者负责发出：收尾正常返回则带最终状态发出，
	// 收尾途中崩溃也在栈展开时补发，保证同一条运行恰好一条终态事件。
	defer func() {
		state, _ := s.getRunState(record.runID)
		s.publish(eventSourceFromRecord(record), terminalRunEventType(intent.status), state)
	}()
	if wrapUp != nil {
		if reason, payload := wrapUp(); reason != "" || payload != nil {
			s.mu.Lock()
			record.state.Reason = reason
			record.state.Error = cloneErrorPayload(payload)
			record.state.UpdatedAt = nowUTC()
			s.mu.Unlock()
		}
	}
}

// terminalRunRetention 是运行进入终态后的内存保留窗口：
// 终态运行记录不再参与调度，只在窗口内保留供短时查询，超界由既有锁内的惰性回收删除。
const terminalRunRetention = 10 * time.Minute

// finalizeRun 是运行收口的统一终结步骤：在所有终态出口完成「终态落定、会话落盘、终态事件发布」
// 之后调用，释放本运行的运行期派生内存，并让运行记录进入有界保留。
// 它只删内存，绝不动会话；不新增回收器、定时器或后台扫描。
func (s *system) finalizeRun(record *runRecord) {
	if record == nil {
		return
	}
	s.publishedMu.Lock()
	delete(s.publishedMessages, record.runID)
	s.publishedMu.Unlock()
	s.mu.Lock()
	// 运行终态统一清账：注销确认决策台账并回绝未落定的提交，不让投递方挂死。
	s.clearConfirmationsLocked(record)
	record.terminalAt = nowUTC()
	s.reclaimTerminalRunsLocked()
	s.mu.Unlock()
}

// reclaimTerminalRunsLocked 在既有锁内惰性回收超出保留窗口的终态运行记录。
func (s *system) reclaimTerminalRunsLocked() {
	now := nowUTC()
	for runID, record := range s.runs {
		if record == nil || isActiveRunStatus(record.state.Status) || record.terminalAt.IsZero() {
			continue
		}
		if now.Sub(record.terminalAt) >= terminalRunRetention {
			delete(s.runs, runID)
		}
	}
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
