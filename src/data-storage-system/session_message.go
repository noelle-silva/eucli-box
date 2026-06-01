package datastorage

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

const defaultSessionBranchID = "main"

func normalizeSessionForStorage(session types.Session, now time.Time) types.Session {
	session.ID = strings.TrimSpace(session.ID)
	session.RoleID = strings.TrimSpace(session.RoleID)
	session.Title = normalizeSessionTitle(session.Title)
	if session.Status == "" {
		session.Status = string(types.RunStatusCreated)
	}
	baseline := firstNonZeroTime(session.CreatedAt, session.LastActive, session.UpdatedAt, now)
	if session.CreatedAt.IsZero() {
		session.CreatedAt = baseline
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = baseline
	}
	if session.LastActive.IsZero() {
		session.LastActive = baseline
	}
	session.Messages = normalizeSessionMessages(session.Messages, now)
	for _, message := range session.Messages {
		if message.CreatedAt.After(session.LastActive) {
			session.LastActive = message.CreatedAt
		}
		if message.UpdatedAt.After(session.UpdatedAt) {
			session.UpdatedAt = message.UpdatedAt
		}
	}
	if session.LastActive.After(session.UpdatedAt) {
		session.UpdatedAt = session.LastActive
	}
	return session
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Now().UTC()
}

func normalizeSessionMessages(messages []types.Message, now time.Time) []types.Message {
	result := make([]types.Message, 0, len(messages))
	seen := map[string]struct{}{}
	lastMessageID := ""
	lastByBranch := map[string]string{}

	for _, message := range messages {
		message.ID = strings.TrimSpace(message.ID)
		if message.ID == "" {
			message.ID = utils.NewID("message")
		}
		if _, ok := seen[message.ID]; ok {
			message.ID = utils.NewID("message")
		}
		seen[message.ID] = struct{}{}

		message.Type = normalizeMessageType(message.Type)
		message.BranchID = normalizeBranchID(message.BranchID)
		message.ParentMessageID = strings.TrimSpace(message.ParentMessageID)
		if message.ParentMessageID == "" {
			message.ParentMessageID = lastByBranch[message.BranchID]
			if message.ParentMessageID == "" {
				message.ParentMessageID = lastMessageID
			}
		}
		if message.CreatedAt.IsZero() {
			message.CreatedAt = now
		}
		if message.UpdatedAt.IsZero() {
			message.UpdatedAt = message.CreatedAt
		}

		result = append(result, message)
		lastMessageID = message.ID
		lastByBranch[message.BranchID] = message.ID
	}

	return result
}

func normalizeSessionTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return "新聊天"
	}
	runes := []rune(title)
	if len(runes) > 80 {
		return strings.TrimSpace(string(runes[:80]))
	}
	return title
}

func normalizeMessageType(messageType string) string {
	switch strings.TrimSpace(messageType) {
	case "assistant":
		return "assistant"
	case "tool":
		return "tool"
	case "tool_request":
		return "tool_request"
	case "tool_confirmation":
		return "tool_confirmation"
	case "failure":
		return "failure"
	default:
		return "user"
	}
}

func normalizeBranchID(branchID string) string {
	branchID = strings.TrimSpace(branchID)
	if branchID == "" {
		return defaultSessionBranchID
	}
	return branchID
}

func (s *system) UpdateSessionTitle(ctx context.Context, roleID string, sessionID string, title string) (types.Session, error) {
	session, err := s.LoadSession(ctx, roleID, sessionID)
	if err != nil {
		return types.Session{}, err
	}
	now := time.Now().UTC()
	session.Title = normalizeSessionTitle(title)
	session.UpdatedAt = now
	if session.LastActive.Before(now) {
		session.LastActive = now
	}
	if err := s.SaveSession(ctx, session); err != nil {
		return types.Session{}, err
	}
	return s.LoadSession(ctx, roleID, sessionID)
}

func (s *system) UpdateSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string, content string) (types.Session, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return types.Session{}, storageInvalid("message id is required", nil)
	}
	session, err := s.LoadSession(ctx, roleID, sessionID)
	if err != nil {
		return types.Session{}, err
	}
	now := time.Now().UTC()
	session = normalizeSessionForStorage(session, now)
	updated := false
	for i := range session.Messages {
		if session.Messages[i].ID != messageID {
			continue
		}
		session.Messages[i].Content = content
		session.Messages[i].UpdatedAt = now
		updated = true
		break
	}
	if !updated {
		return types.Session{}, storageNotFound("message was not found", nil)
	}
	session.UpdatedAt = now
	if session.LastActive.Before(now) {
		session.LastActive = now
	}
	if err := s.SaveSession(ctx, session); err != nil {
		return types.Session{}, err
	}
	return s.LoadSession(ctx, roleID, sessionID)
}

func (s *system) DeleteSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string) (types.Session, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return types.Session{}, storageInvalid("message id is required", nil)
	}
	session, err := s.LoadSession(ctx, roleID, sessionID)
	if err != nil {
		return types.Session{}, err
	}
	now := time.Now().UTC()
	session = normalizeSessionForStorage(session, now)
	parentID := ""
	found := false
	for _, message := range session.Messages {
		if message.ID == messageID {
			parentID = message.ParentMessageID
			found = true
			break
		}
	}
	if !found {
		return types.Session{}, storageNotFound("message was not found", nil)
	}

	next := make([]types.Message, 0, len(session.Messages)-1)
	for _, message := range session.Messages {
		if message.ID == messageID {
			continue
		}
		if message.ParentMessageID == messageID {
			message.ParentMessageID = parentID
			message.UpdatedAt = now
		}
		next = append(next, message)
	}
	session.Messages = next
	session.UpdatedAt = now
	if session.LastActive.Before(now) {
		session.LastActive = now
	}
	if err := s.SaveSession(ctx, session); err != nil {
		return types.Session{}, err
	}
	return s.LoadSession(ctx, roleID, sessionID)
}

func (s *system) DeleteSessionMessageSubtree(ctx context.Context, roleID string, sessionID string, messageID string) (types.Session, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return types.Session{}, storageInvalid("message id is required", nil)
	}
	session, err := s.LoadSession(ctx, roleID, sessionID)
	if err != nil {
		return types.Session{}, err
	}
	now := time.Now().UTC()
	session = normalizeSessionForStorage(session, now)
	children := map[string][]string{}
	found := false
	for _, message := range session.Messages {
		if message.ID == messageID {
			found = true
		}
		if message.ParentMessageID == "" {
			continue
		}
		children[message.ParentMessageID] = append(children[message.ParentMessageID], message.ID)
	}
	if !found {
		return types.Session{}, storageNotFound("message was not found", nil)
	}

	deleted := map[string]struct{}{}
	stack := []string{messageID}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == "" {
			continue
		}
		if _, ok := deleted[current]; ok {
			continue
		}
		deleted[current] = struct{}{}
		stack = append(stack, children[current]...)
	}

	next := make([]types.Message, 0, len(session.Messages)-len(deleted))
	for _, message := range session.Messages {
		if _, ok := deleted[message.ID]; ok {
			continue
		}
		next = append(next, message)
	}
	session.Messages = next
	session.UpdatedAt = now
	if session.LastActive.Before(now) {
		session.LastActive = now
	}
	if err := s.SaveSession(ctx, session); err != nil {
		return types.Session{}, err
	}
	return s.LoadSession(ctx, roleID, sessionID)
}
