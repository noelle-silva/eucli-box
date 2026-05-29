package agentruntime

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func (s *system) loadOrCreateSession(ctx context.Context, request types.RunRequest) (types.Session, error) {
	if strings.TrimSpace(request.SessionID) != "" {
		session, err := s.storage.LoadSession(ctx, request.RoleID, request.SessionID)
		if err != nil {
			return types.Session{}, runtimeStorageFailed("failed to load session", err)
		}
		return session, nil
	}
	now := time.Now().UTC()
	return types.Session{ID: utils.NewID("session"), RoleID: request.RoleID, Title: firstTitle(request.Message), Status: string(types.RunStatusCreated), Messages: []types.Message{}, CreatedAt: now, LastActive: now}, nil
}

func appendMessage(session types.Session, message types.Message) types.Session {
	session.Messages = append(session.Messages, message)
	session.LastActive = message.CreatedAt
	return session
}

func userMessage(content string) types.Message {
	return types.Message{ID: utils.NewID("message"), Type: "user", Content: content, CreatedAt: time.Now().UTC()}
}

func assistantMessage(content string) types.Message {
	return types.Message{ID: utils.NewID("message"), Type: "assistant", Content: content, CreatedAt: time.Now().UTC()}
}

func toolMessage(result types.ToolResult) types.Message {
	content := result.Content
	if content == "" {
		content = result.Error
	}
	return types.Message{ID: utils.NewID("message"), Type: "tool", Content: content, ToolName: result.ToolName, Reason: result.Error, CreatedAt: time.Now().UTC()}
}

func failureMessage(reason string) types.Message {
	return types.Message{ID: utils.NewID("message"), Type: "failure", Reason: reason, Content: reason, CreatedAt: time.Now().UTC()}
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
