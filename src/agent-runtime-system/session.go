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
	if strings.TrimSpace(request.SessionID) != "" {
		session, err := s.storage.LoadSession(ctx, request.RoleID, request.SessionID)
		if err != nil {
			return types.Session{}, runtimeStorageFailed("failed to load session", err)
		}
		return session, nil
	}
	now := time.Now().UTC()
	return types.Session{ID: utils.NewID("session"), RoleID: request.RoleID, Title: firstTitle(request.Message), Status: string(types.RunStatusCreated), Messages: []types.Message{}, CreatedAt: now, UpdatedAt: now, LastActive: now}, nil
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

func appendUserMessageForRun(session types.Session, content string, parentMessageID string) (types.Session, error) {
	message := userMessage(content)
	parentMessageID = strings.TrimSpace(parentMessageID)
	if parentMessageID == "" {
		return appendMessage(session, message), nil
	}
	parent, ok := messageByID(session.Messages, parentMessageID)
	if !ok {
		return session, runtimeNotFound("parent message was not found", nil)
	}
	message.BranchID = userMessageBranchID(session, parent, message.ID)
	return appendChildMessage(session, message, parent), nil
}

func appendAssistantReply(session types.Session, content string, parent types.Message) types.Session {
	message := assistantMessage(content)
	message.BranchID = assistantReplyBranchID(session, parent, message.ID)
	return appendChildMessage(session, message, parent)
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
		return
	}
	record.session = appendAssistantReply(record.session, content, record.messageParent)
	record.messageParent = lastSessionMessage(record.session)
	record.lastMessageID = record.messageParent.ID
}

func ensureRunAssistantMessage(record *runRecord) {
	if record.messageParent.Type == "assistant" && strings.TrimSpace(record.messageParent.ID) != "" {
		return
	}
	appendRunAssistantReply(record, "")
}

func updateRunAssistantContent(record *runRecord, content string) {
	if record.messageParent.Type != "assistant" || strings.TrimSpace(record.messageParent.ID) == "" {
		appendRunAssistantReply(record, content)
		return
	}
	now := time.Now().UTC()
	record.messageParent.Content = content
	record.messageParent.UpdatedAt = now
	for index := range record.session.Messages {
		if record.session.Messages[index].ID != record.messageParent.ID {
			continue
		}
		record.session.Messages[index].Content = content
		record.session.Messages[index].UpdatedAt = now
		break
	}
	record.session.UpdatedAt = now
	record.session.LastActive = now
	record.lastMessageID = record.messageParent.ID
}

func appendRunMessage(record *runRecord, message types.Message) {
	record.session = appendChildMessage(record.session, message, record.messageParent)
	record.messageParent = lastSessionMessage(record.session)
	record.lastMessageID = record.messageParent.ID
}

func appendRunFailureMessage(record *runRecord, session types.Session, reason string) types.Session {
	if strings.TrimSpace(record.messageParent.ID) == "" {
		session = appendMessage(session, failureMessage(reason))
		record.session = session
		record.lastMessageID = lastSessionMessage(session).ID
		return session
	}
	record.session = session
	appendRunMessage(record, failureMessage(reason))
	return record.session
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
		return types.Session{}, types.Message{}, runtimeInvalid("userMessageId is required", nil)
	}
	byID := messagesByID(session.Messages)
	target, ok := byID[messageID]
	if !ok {
		return types.Session{}, types.Message{}, runtimeNotFound("user message was not found", nil)
	}
	if target.Type != "user" {
		return types.Session{}, types.Message{}, runtimeInvalid("userMessageId must reference a user message", nil)
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

func failureMessage(reason string) types.Message {
	now := time.Now().UTC()
	return types.Message{ID: utils.NewID("message"), Type: "failure", Content: reason, Reason: reason, CreatedAt: now, UpdatedAt: now}
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

func firstTitle(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "New session"
	}
	runes := []rune(message)
	if len(runes) > 48 {
		return string(runes[:48])
	}
	return message
}
