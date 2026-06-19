package agentruntime

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

const defaultRuntimeBranchID = "main"

func (s *system) loadOrCreateSession(ctx context.Context, request types.RunRequest) (types.Session, error) {
	groupID := strings.TrimSpace(request.GroupID)
	workspaceID := strings.TrimSpace(request.WorkspaceID)
	if strings.TrimSpace(request.SessionID) != "" {
		if groupID != "" {
			session, err := s.storage.LoadGroupSession(ctx, groupID, request.SessionID)
			if err != nil {
				return types.Session{}, runtimeStorageFailed("failed to load group session", err)
			}
			return s.recoverAsyncToolTasks(session), nil
		}
		if workspaceID != "" {
			session, err := s.storage.LoadWorkspaceSession(ctx, workspaceID, request.RoleID, request.SessionID)
			if err != nil {
				return types.Session{}, runtimeStorageFailed("failed to load workspace session", err)
			}
			return s.recoverAsyncToolTasks(session), nil
		}
		session, err := s.storage.LoadSession(ctx, request.RoleID, request.SessionID)
		if err != nil {
			return types.Session{}, runtimeStorageFailed("failed to load session", err)
		}
		return s.recoverAsyncToolTasks(session), nil
	}
	now := time.Now().UTC()
	if groupID != "" {
		return types.Session{ID: utils.NewID("session"), GroupID: groupID, Title: types.DefaultSessionTitle, Status: string(types.RunStatusCreated), Messages: []types.Message{}, CreatedAt: now, UpdatedAt: now, LastActive: now}, nil
	}
	if workspaceID != "" {
		return types.Session{ID: utils.NewID("session"), WorkspaceID: workspaceID, RoleID: request.RoleID, Title: types.DefaultSessionTitle, Status: string(types.RunStatusCreated), Messages: []types.Message{}, CreatedAt: now, UpdatedAt: now, LastActive: now}, nil
	}
	return types.Session{ID: utils.NewID("session"), RoleID: request.RoleID, Title: types.DefaultSessionTitle, Status: string(types.RunStatusCreated), Messages: []types.Message{}, CreatedAt: now, UpdatedAt: now, LastActive: now}, nil
}

func (s *system) recoverAsyncToolTasks(session types.Session) types.Session {
	if len(session.AsyncToolTasks) == 0 {
		return session
	}
	now := time.Now().UTC()
	for index := range session.AsyncToolTasks {
		if runtimeTask, ok := s.asyncToolTaskSnapshot(session.AsyncToolTasks[index]); ok {
			session.AsyncToolTasks[index] = runtimeTask
			continue
		}
		status := session.AsyncToolTasks[index].Status
		if status != types.AsyncToolTaskStatusPending && status != types.AsyncToolTaskStatusRunning {
			continue
		}
		session.AsyncToolTasks[index].Status = types.AsyncToolTaskStatusFailed
		session.AsyncToolTasks[index].FinishedAt = now
		session.AsyncToolTasks[index].Error = "异步任务所在进程已结束，任务未完成。"
		result := failedToolResult(session.AsyncToolTasks[index].Action, session.AsyncToolTasks[index].Error)
		session.AsyncToolTasks[index].Result = &result
	}
	return session
}

func (s *system) asyncToolTaskSnapshot(task types.AsyncToolTask) (types.AsyncToolTask, bool) {
	id := strings.TrimSpace(task.ID)
	if id == "" {
		return types.AsyncToolTask{}, false
	}
	s.mu.Lock()
	runtimeTask, ok := s.asyncTasks[id]
	s.mu.Unlock()
	if !ok || strings.TrimSpace(runtimeTask.SessionID) != strings.TrimSpace(task.SessionID) {
		return types.AsyncToolTask{}, false
	}
	return runtimeTask, true
}

func appendMessage(session types.Session, message types.Message) types.Session {
	now := time.Now().UTC()
	if strings.TrimSpace(message.ID) == "" {
		message.ID = utils.NewID("message")
	}
	message.BranchID = strings.TrimSpace(message.BranchID)
	if message.BranchID == "" {
		message.BranchID = activeSessionBranchID(session)
	}
	message.ParentMessageID = strings.TrimSpace(message.ParentMessageID)
	if message.ParentMessageID == "" {
		message.ParentMessageID = lastMessageIDInBranch(session.Messages, message.BranchID)
	}
	if message.ParentMessageID == "" && len(session.Messages) > 0 {
		message.ParentMessageID = session.Messages[len(session.Messages)-1].ID
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	if message.UpdatedAt.IsZero() {
		message.UpdatedAt = message.CreatedAt
	}
	session.Messages = append(session.Messages, message)
	session.LastActive = message.CreatedAt
	if session.UpdatedAt.Before(message.UpdatedAt) {
		session.UpdatedAt = message.UpdatedAt
	}
	return session
}

func markRunOwnedMessage(record *runRecord, messageID string) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return
	}
	if record.ownedMessageIDs == nil {
		record.ownedMessageIDs = map[string]struct{}{}
	}
	record.ownedMessageIDs[messageID] = struct{}{}
}

func isRunOwnedMessage(record *runRecord, messageID string) bool {
	if record == nil || len(record.ownedMessageIDs) == 0 {
		return false
	}
	_, ok := record.ownedMessageIDs[strings.TrimSpace(messageID)]
	return ok
}

func markRunDeletedMessage(record *runRecord, message types.Message) {
	messageID := strings.TrimSpace(message.ID)
	if messageID == "" || record == nil {
		return
	}
	if record.deletedMessageIDs == nil {
		record.deletedMessageIDs = map[string]struct{}{}
	}
	record.deletedMessageIDs[messageID] = struct{}{}
	if record.messageSnapshots == nil {
		record.messageSnapshots = map[string]types.Message{}
	}
	if _, ok := record.messageSnapshots[messageID]; !ok {
		record.messageSnapshots[messageID] = cloneRunMessageSnapshot(message)
	}
	delete(record.ownedMessageIDs, messageID)
}

func markRunDependencyMessage(record *runRecord, message types.Message) {
	messageID := strings.TrimSpace(message.ID)
	if messageID == "" || record == nil || isRunOwnedMessage(record, messageID) {
		return
	}
	if record.dependencyIDs == nil {
		record.dependencyIDs = map[string]struct{}{}
	}
	record.dependencyIDs[messageID] = struct{}{}
	if record.messageSnapshots == nil {
		record.messageSnapshots = map[string]types.Message{}
	}
	if _, ok := record.messageSnapshots[messageID]; !ok {
		record.messageSnapshots[messageID] = cloneRunMessageSnapshot(message)
	}
}

func markRunDependencyMessages(record *runRecord, messages []types.Message) {
	for _, message := range messages {
		markRunDependencyMessage(record, message)
	}
}

func (s *system) appendUserMessageForRun(ctx context.Context, session types.Session, request types.RunRequest) (types.Session, error) {
	attachments, err := s.saveRunAttachments(ctx, session, request.Attachments)
	if err != nil {
		return session, err
	}
	message := userMessage(request.Message)
	message.Attachments = attachments
	parentMessageID := strings.TrimSpace(request.ParentMessageID)
	if parentMessageID == "" {
		next := appendMessage(session, message)
		return next, nil
	}
	parent, ok := messageByID(session.Messages, parentMessageID)
	if !ok {
		return session, runtimeNotFound("parent message was not found", nil)
	}
	message.BranchID = userMessageBranchID(session, parent, message.ID)
	return appendChildMessage(session, message, parent), nil
}

func (s *system) saveRunAttachments(ctx context.Context, session types.Session, attachments []types.RunAttachment) ([]types.MessageAttachment, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	stored := make([]types.MessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		var saved types.MessageAttachment
		var err error
		if strings.TrimSpace(session.GroupID) != "" {
			saved, err = s.storage.SaveGroupSessionMessageAttachment(ctx, session.GroupID, session.ID, attachment)
		} else if strings.TrimSpace(session.WorkspaceID) != "" {
			saved, err = s.storage.SaveWorkspaceSessionMessageAttachment(ctx, session.WorkspaceID, session.RoleID, session.ID, attachment)
		} else {
			saved, err = s.storage.SaveSessionMessageAttachment(ctx, session.RoleID, session.ID, attachment)
		}
		if err != nil {
			return nil, runtimeStorageFailed("failed to save message attachment", err)
		}
		stored = append(stored, saved)
	}
	return stored, nil
}

func appendAssistantReply(session types.Session, content string, parent types.Message) types.Session {
	message := assistantMessage(content)
	message.BranchID = assistantReplyBranchID(session, parent, message.ID)
	return appendChildMessage(session, message, parent)
}

func appendAssistantReplyForRun(record *runRecord, content string) types.Session {
	message := assistantMessage(content)
	if strings.TrimSpace(record.groupID) != "" {
		message.SpeakerRoleID = record.roleID
	}
	if record.forceBranchReply {
		message.BranchID = branchIDFromMessageID(message.ID)
	} else {
		message.BranchID = assistantReplyBranchID(record.session, record.messageParent, message.ID)
	}
	return appendChildMessage(record.session, message, record.messageParent)
}

func appendChildMessage(session types.Session, message types.Message, parent types.Message) types.Session {
	message.ParentMessageID = parent.ID
	if strings.TrimSpace(message.BranchID) == "" {
		message.BranchID = parent.BranchID
	}
	return appendMessage(session, message)
}

func appendRunAssistantReply(record *runRecord, content string) {
	if strings.TrimSpace(record.messageParent.ID) == "" {
		appendRunMessage(record, assistantMessage(content))
		record.activeAssistantID = record.messageParent.ID
		return
	}
	record.session = appendAssistantReplyForRun(record, content)
	record.messageParent = lastSessionMessage(record.session)
	record.lastMessageID = record.messageParent.ID
	record.activeAssistantID = record.messageParent.ID
	record.forceNewAssistantReply = false
	markRunOwnedMessage(record, record.messageParent.ID)
}

func ensureRunAssistantMessage(record *runRecord) {
	if assistant, ok := activeRunAssistant(record); ok {
		record.messageParent = assistant
		record.lastMessageID = assistant.ID
		return
	}
	appendRunAssistantReply(record, "")
}

func updateRunAssistantContent(record *runRecord, content string) {
	if _, ok := activeRunAssistant(record); !ok {
		appendRunAssistantReply(record, content)
		return
	}
	now := time.Now().UTC()
	messageID := strings.TrimSpace(record.activeAssistantID)
	if messageID == "" {
		messageID = record.messageParent.ID
	}
	record.messageParent.Content = content
	record.messageParent.UpdatedAt = now
	setMessageTextPart(&record.messageParent, content, now)
	for index := range record.session.Messages {
		if record.session.Messages[index].ID != messageID {
			continue
		}
		record.session.Messages[index].Content = content
		record.session.Messages[index].UpdatedAt = now
		setMessageTextPart(&record.session.Messages[index], content, now)
		record.messageParent = record.session.Messages[index]
		break
	}
	record.session.UpdatedAt = now
	record.session.LastActive = now
	record.lastMessageID = record.messageParent.ID
}

func updateRunAssistantReasoning(record *runRecord, reasoning string, source string, signature string, data string) {
	if _, ok := activeRunAssistant(record); !ok {
		appendRunAssistantReply(record, "")
	}
	now := time.Now().UTC()
	messageID := strings.TrimSpace(record.activeAssistantID)
	if messageID == "" {
		messageID = record.messageParent.ID
	}
	setMessageReasoningPart(&record.messageParent, reasoning, source, signature, data, now)
	record.messageParent.UpdatedAt = now
	for index := range record.session.Messages {
		if record.session.Messages[index].ID != messageID {
			continue
		}
		setMessageReasoningPart(&record.session.Messages[index], reasoning, source, signature, data, now)
		record.session.Messages[index].UpdatedAt = now
		record.messageParent = record.session.Messages[index]
		break
	}
	record.session.UpdatedAt = now
	record.session.LastActive = now
	record.lastMessageID = record.messageParent.ID
}

func dropEmptyAssistantOutput(record *runRecord) {
	if record.messageParent.Type != "assistant" || strings.TrimSpace(record.messageParent.ID) == "" || strings.TrimSpace(record.messageParent.Content) != "" || hasToolParts(record.messageParent) || hasReasoningParts(record.messageParent) {
		return
	}
	if len(record.session.Messages) == 0 || record.session.Messages[len(record.session.Messages)-1].ID != record.messageParent.ID {
		return
	}
	markRunDeletedMessage(record, record.messageParent)
	parentID := strings.TrimSpace(record.messageParent.ParentMessageID)
	record.session.Messages = record.session.Messages[:len(record.session.Messages)-1]
	if parentID != "" {
		if parent, ok := messageByID(record.session.Messages, parentID); ok {
			record.messageParent = parent
			record.lastMessageID = parent.ID
			record.activeAssistantID = ""
			return
		}
	}
	record.messageParent = lastSessionMessage(record.session)
	record.lastMessageID = record.messageParent.ID
	record.activeAssistantID = ""
}

func appendRunMessage(record *runRecord, message types.Message) {
	if strings.TrimSpace(record.groupID) != "" && message.Type == "assistant" {
		message.SpeakerRoleID = record.roleID
	}
	record.session = appendChildMessage(record.session, message, record.messageParent)
	record.messageParent = lastSessionMessage(record.session)
	record.lastMessageID = record.messageParent.ID
	markRunOwnedMessage(record, record.messageParent.ID)
}

func markRunInputMessage(record *runRecord, message types.Message) {
	markRunOwnedMessage(record, message.ID)
}

func markRunFailureMessage(record *runRecord, session types.Session, payload *types.ErrorPayload) types.Session {
	if payload == nil || strings.TrimSpace(payload.Message) == "" {
		return session
	}
	record.session = session
	if _, ok := activeRunAssistant(record); !ok {
		appendRunAssistantReply(record, "")
	}
	messageID := strings.TrimSpace(record.activeAssistantID)
	if messageID == "" {
		return record.session
	}
	now := time.Now().UTC()
	for index := range record.session.Messages {
		if record.session.Messages[index].ID != messageID {
			continue
		}
		record.session.Messages[index].Error = cloneErrorPayload(payload)
		record.session.Messages[index].UpdatedAt = now
		record.messageParent = record.session.Messages[index]
		record.lastMessageID = record.messageParent.ID
		record.session.UpdatedAt = now
		record.session.LastActive = now
		return record.session
	}
	return record.session
}

func cloneErrorPayload(payload *types.ErrorPayload) *types.ErrorPayload {
	if payload == nil {
		return nil
	}
	return &types.ErrorPayload{Code: payload.Code, Message: payload.Message, System: payload.System, Details: payload.Details, Cause: cloneErrorPayload(payload.Cause), Causes: cloneErrorPayloads(payload.Causes)}
}

func cloneErrorPayloads(payloads []*types.ErrorPayload) []*types.ErrorPayload {
	if len(payloads) == 0 {
		return nil
	}
	cloned := make([]*types.ErrorPayload, 0, len(payloads))
	for _, payload := range payloads {
		if clonedPayload := cloneErrorPayload(payload); clonedPayload != nil {
			cloned = append(cloned, clonedPayload)
		}
	}
	return cloned
}

func cloneRunRetryInfo(retry *types.RunRetryInfo) *types.RunRetryInfo {
	if retry == nil {
		return nil
	}
	cloned := *retry
	cloned.Failure = cloneErrorPayload(retry.Failure)
	return &cloned
}

func assistantReplyBranchID(session types.Session, parent types.Message, assistantID string) string {
	parentBranchID := strings.TrimSpace(parent.BranchID)
	if parentBranchID == "" {
		parentBranchID = defaultRuntimeBranchID
	}
	if parent.Type != "user" || !hasAssistantChild(session.Messages, parent.ID) {
		return parentBranchID
	}
	return branchIDFromMessageID(assistantID)
}

func shouldForceBranchReply(session types.Session, parent types.Message) bool {
	parentID := strings.TrimSpace(parent.ID)
	if parentID == "" || parent.Type != "user" {
		return false
	}
	return hasAssistantChild(session.Messages, parentID)
}

func userMessageBranchID(session types.Session, parent types.Message, userMessageID string) string {
	parentBranchID := strings.TrimSpace(parent.BranchID)
	if parentBranchID == "" {
		parentBranchID = defaultRuntimeBranchID
	}
	if !hasAnyChild(session.Messages, parent.ID) {
		return parentBranchID
	}
	return branchIDFromMessageID(userMessageID)
}

func hasAnyChild(messages []types.Message, parentID string) bool {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return false
	}
	for _, message := range messages {
		if strings.TrimSpace(message.ParentMessageID) == parentID {
			return true
		}
	}
	return false
}

func hasAssistantChild(messages []types.Message, parentID string) bool {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return false
	}
	for _, message := range messages {
		if message.Type == "assistant" && strings.TrimSpace(message.ParentMessageID) == parentID {
			return true
		}
	}
	return false
}

func branchIDFromMessageID(messageID string) string {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return defaultRuntimeBranchID
	}
	return "branch-" + messageID
}

func sessionContextThroughMessage(session types.Session, messageID string) (types.Session, types.Message, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return types.Session{}, types.Message{}, runtimeInvalid("message id is required", nil)
	}
	byID := messagesByID(session.Messages)
	target, ok := byID[messageID]
	if !ok {
		return types.Session{}, types.Message{}, runtimeNotFound("message was not found", nil)
	}

	chain := []types.Message{}
	seen := map[string]struct{}{}
	current := target
	for {
		if current.ID == "" {
			break
		}
		if _, ok := seen[current.ID]; ok {
			return types.Session{}, types.Message{}, runtimeInvalid("message parent chain contains a cycle", nil)
		}
		seen[current.ID] = struct{}{}
		chain = append(chain, current)
		parentID := strings.TrimSpace(current.ParentMessageID)
		if parentID == "" {
			break
		}
		parent, ok := byID[parentID]
		if !ok {
			break
		}
		current = parent
	}

	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	contextSession := session
	contextSession.Messages = chain
	return contextSession, target, nil
}

func sessionContextThroughUserMessage(session types.Session, messageID string) (types.Session, types.Message, error) {
	contextSession, target, err := sessionContextThroughMessage(session, messageID)
	if err != nil {
		return types.Session{}, types.Message{}, err
	}
	if target.Type != "user" {
		return types.Session{}, types.Message{}, runtimeInvalid("userMessageId must reference a user message", nil)
	}
	return contextSession, target, nil
}

func messagesByID(messages []types.Message) map[string]types.Message {
	byID := map[string]types.Message{}
	for _, message := range messages {
		if strings.TrimSpace(message.ID) == "" {
			continue
		}
		byID[message.ID] = message
	}
	return byID
}

func messageByID(messages []types.Message, messageID string) (types.Message, bool) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return types.Message{}, false
	}
	message, ok := messagesByID(messages)[messageID]
	return message, ok
}

func activeSessionBranchID(session types.Session) string {
	if session.Metadata != nil {
		branchID := strings.TrimSpace(session.Metadata["activeBranchId"])
		if branchID != "" {
			return branchID
		}
	}
	return defaultRuntimeBranchID
}

func lastMessageIDInBranch(messages []types.Message, branchID string) string {
	branchID = strings.TrimSpace(branchID)
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		messageBranchID := strings.TrimSpace(message.BranchID)
		if messageBranchID == "" {
			messageBranchID = defaultRuntimeBranchID
		}
		if messageBranchID == branchID && strings.TrimSpace(message.ID) != "" {
			return message.ID
		}
	}
	return ""
}

func userMessage(content string) types.Message {
	now := time.Now().UTC()
	return types.Message{ID: utils.NewID("message"), Type: "user", Content: content, BranchID: defaultRuntimeBranchID, CreatedAt: now, UpdatedAt: now}
}

func asyncToolResultMessage(task types.AsyncToolTask) types.Message {
	now := time.Now().UTC()
	toolID := ""
	if task.Result != nil {
		toolID = strings.TrimSpace(task.Result.ID)
	}
	return types.Message{ID: utils.NewID("message"), Type: types.MessageTypeAsyncToolResult, Content: asyncToolResultContent(task), BranchID: defaultRuntimeBranchID, ToolID: toolID, ToolName: asyncToolResultToolName(task), CreatedAt: now, UpdatedAt: now}
}

func assistantMessage(content string) types.Message {
	now := time.Now().UTC()
	return types.Message{ID: utils.NewID("message"), Type: "assistant", Content: content, BranchID: defaultRuntimeBranchID, CreatedAt: now, UpdatedAt: now}
}

func toolMessage(result types.ToolResult) types.Message {
	content := result.Content
	if content == "" {
		content = result.Error
	}
	now := time.Now().UTC()
	return types.Message{ID: utils.NewID("message"), Type: "tool", Content: content, ToolID: result.ID, ToolName: result.ToolName, Reason: result.Error, CreatedAt: now, UpdatedAt: now}
}

func toolRequestMessage(action types.ToolAction) types.Message {
	now := time.Now().UTC()
	return types.Message{ID: utils.NewID("message"), Type: "tool_request", Content: action.ToolName, ToolName: action.ToolName, CreatedAt: now, UpdatedAt: now}
}

func toolConfirmationMessage(decision types.PermissionDecision) types.Message {
	content := decision.Status
	if decision.Status == types.PermissionStatusAllowed {
		content = "tool approved by user"
	} else {
		content = "tool rejected by user"
	}
	now := time.Now().UTC()
	return types.Message{ID: utils.NewID("message"), Type: "tool_confirmation", Content: content, ToolName: decision.ToolName, Reason: decision.Reason, CreatedAt: now, UpdatedAt: now}
}
