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
	return types.Message{ID: utils.NewID("message"), Type: "tool", Content: content, BranchID: defaultRuntimeBranchID, ToolID: result.ID, ToolName: result.ToolName, Reason: result.Error, CreatedAt: now, UpdatedAt: now}
}

func failureMessage(reason string) types.Message {
	now := time.Now().UTC()
	return types.Message{ID: utils.NewID("message"), Type: "failure", Content: reason, BranchID: defaultRuntimeBranchID, Reason: reason, CreatedAt: now, UpdatedAt: now}
}

func toolRequestMessage(action types.ToolAction) types.Message {
	now := time.Now().UTC()
	return types.Message{ID: utils.NewID("message"), Type: "tool_request", Content: action.ToolName, BranchID: defaultRuntimeBranchID, ToolName: action.ToolName, CreatedAt: now, UpdatedAt: now}
}

func toolConfirmationMessage(decision types.PermissionDecision) types.Message {
	content := decision.Status
	if decision.Status == types.PermissionStatusAllowed {
		content = "tool approved by user"
	} else {
		content = "tool rejected by user"
	}
	now := time.Now().UTC()
	return types.Message{ID: utils.NewID("message"), Type: "tool_confirmation", Content: content, BranchID: defaultRuntimeBranchID, ToolName: decision.ToolName, Reason: decision.Reason, CreatedAt: now, UpdatedAt: now}
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
