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
		message.Error = normalizeErrorPayload(message.Error)
		message.Attachments = normalizeSessionMessageAttachments(message.Attachments)
		if message.Content == "" {
			message.Content = textProjectionFromParts(message.Parts)
		}
		message.Parts = normalizeSessionMessageParts(message, now)
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

func normalizeErrorPayload(errorPayload *types.ErrorPayload) *types.ErrorPayload {
	if errorPayload == nil {
		return nil
	}
	errorPayload.Code = strings.TrimSpace(errorPayload.Code)
	errorPayload.Message = strings.TrimSpace(errorPayload.Message)
	errorPayload.System = strings.TrimSpace(errorPayload.System)
	if errorPayload.Message == "" {
		return nil
	}
	return errorPayload
}

func normalizeSessionMessageParts(message types.Message, now time.Time) []types.MessagePart {
	parts := make([]types.MessagePart, 0, len(message.Parts)+1)
	seen := map[string]struct{}{}
	if (message.Type == "user" || message.Type == "assistant") && message.Content != "" {
		parts = append(parts, normalizeTextPart(firstPartOfType(message.Parts, "text"), message.Content, now, seen))
	}
	for _, part := range message.Parts {
		if strings.TrimSpace(part.Type) != "tool" {
			continue
		}
		parts = append(parts, normalizeToolPart(part, now, seen))
	}
	return parts
}

func firstPartOfType(parts []types.MessagePart, partType string) types.MessagePart {
	for _, part := range parts {
		if strings.TrimSpace(part.Type) == partType {
			return part
		}
	}
	return types.MessagePart{}
}

func normalizeTextPart(part types.MessagePart, text string, now time.Time, seen map[string]struct{}) types.MessagePart {
	part.Type = "text"
	part.Text = text
	part.ID = normalizeMessagePartID(part.ID, seen)
	if part.CreatedAt.IsZero() {
		part.CreatedAt = now
	}
	if part.UpdatedAt.IsZero() {
		part.UpdatedAt = part.CreatedAt
	}
	part.CallID = ""
	part.ToolName = ""
	part.Source = ""
	part.Raw = ""
	part.Input = nil
	part.State = ""
	part.Decision = nil
	part.Result = nil
	return part
}

func normalizeToolPart(part types.MessagePart, now time.Time, seen map[string]struct{}) types.MessagePart {
	part.Type = "tool"
	part.ID = normalizeMessagePartID(part.ID, seen)
	part.Source = strings.TrimSpace(part.Source)
	part.CallID = strings.TrimSpace(part.CallID)
	part.ToolName = strings.TrimSpace(part.ToolName)
	part.State = strings.TrimSpace(part.State)
	part.Text = ""
	if part.Input == nil {
		part.Input = map[string]any{}
	}
	if part.Decision != nil {
		part.Decision.ID = strings.TrimSpace(part.Decision.ID)
		part.Decision.ActionID = strings.TrimSpace(part.Decision.ActionID)
		part.Decision.ToolName = strings.TrimSpace(part.Decision.ToolName)
		part.Decision.Status = strings.TrimSpace(part.Decision.Status)
		part.Decision.Reason = strings.TrimSpace(part.Decision.Reason)
	}
	if part.Result != nil {
		part.Result.ID = strings.TrimSpace(part.Result.ID)
		part.Result.ActionID = strings.TrimSpace(part.Result.ActionID)
		part.Result.ToolName = strings.TrimSpace(part.Result.ToolName)
		part.Result.Error = strings.TrimSpace(part.Result.Error)
		if part.Result.Metadata == nil {
			part.Result.Metadata = map[string]any{}
		}
	}
	if len(part.Display) == 0 {
		part.Display = nil
	}
	if part.CreatedAt.IsZero() {
		part.CreatedAt = now
	}
	if part.UpdatedAt.IsZero() {
		part.UpdatedAt = part.CreatedAt
	}
	return part
}

func normalizeMessagePartID(id string, seen map[string]struct{}) string {
	id = strings.TrimSpace(id)
	if id == "" {
		id = utils.NewID("part")
	}
	if _, ok := seen[id]; ok {
		id = utils.NewID("part")
	}
	seen[id] = struct{}{}
	return id
}

func textProjectionFromParts(parts []types.MessagePart) string {
	blocks := []string{}
	for _, part := range parts {
		if strings.TrimSpace(part.Type) != "text" || part.Text == "" {
			continue
		}
		blocks = append(blocks, part.Text)
	}
	return strings.Join(blocks, "\n\n")
}

func normalizeSessionTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return types.DefaultSessionTitle
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

func normalizeSessionMessageAttachments(attachments []types.MessageAttachment) []types.MessageAttachment {
	result := make([]types.MessageAttachment, 0, len(attachments))
	seen := map[string]struct{}{}
	for _, attachment := range attachments {
		attachment.ID = strings.TrimSpace(attachment.ID)
		if attachment.ID == "" {
			attachment.ID = utils.NewID("att")
		}
		if _, ok := seen[attachment.ID]; ok {
			attachment.ID = utils.NewID("att")
		}
		seen[attachment.ID] = struct{}{}

		kind := normalizeMessageAttachmentKind(attachment.Kind)
		if kind == "image" {
			attachment.Path = filepathToSlashTrimmed(attachment.Path)
			if attachment.Path == "" {
				continue
			}
			attachment.Kind = "image"
			attachment.Name = normalizeAttachmentName(attachment.Name, "图片")
			attachment.Mime = strings.TrimSpace(attachment.Mime)
			attachment.Lang = ""
			attachment.Text = ""
			attachment.FullLen = 0
			attachment.SendLen = 0
			attachment.SendPct = 0
			result = append(result, attachment)
			continue
		}

		attachment.Text = strings.TrimSpace(attachment.Text)
		if attachment.Text == "" {
			continue
		}
		attachment.Kind = kind
		attachment.Name = normalizeAttachmentName(attachment.Name, "文件")
		attachment.Mime = strings.TrimSpace(attachment.Mime)
		attachment.Path = ""
		attachment.Lang = normalizeAttachmentLang(attachment.Lang, kind)
		textLen := len([]rune(attachment.Text))
		if attachment.FullLen <= 0 {
			attachment.FullLen = textLen
		}
		if attachment.SendLen <= 0 || attachment.SendLen > attachment.FullLen {
			attachment.SendLen = textLen
		}
		if attachment.SendPct <= 0 {
			attachment.SendPct = 100
		}
		if attachment.SendPct > 100 {
			attachment.SendPct = 100
		}
		result = append(result, attachment)
	}
	return result
}

func filepathToSlashTrimmed(path string) string {
	return strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
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

func (s *system) UpdateSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string, patch types.SessionMessagePatch) (types.Message, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return types.Message{}, storageInvalid("message id is required", nil)
	}
	if patch.Content == nil && patch.Parts == nil {
		return types.Message{}, storageInvalid("message patch is empty", nil)
	}
	session, err := s.LoadSession(ctx, roleID, sessionID)
	if err != nil {
		return types.Message{}, err
	}
	now := time.Now().UTC()
	session = normalizeSessionForStorage(session, now)
	updatedIndex := -1
	for i := range session.Messages {
		if session.Messages[i].ID != messageID {
			continue
		}
		if patch.Content != nil {
			session.Messages[i].Content = *patch.Content
		}
		if patch.Parts != nil {
			session.Messages[i].Parts = append([]types.MessagePart(nil), (*patch.Parts)...)
		}
		session.Messages[i].UpdatedAt = now
		updatedIndex = i
		break
	}
	if updatedIndex < 0 {
		return types.Message{}, storageNotFound("message was not found", nil)
	}
	session.UpdatedAt = now
	if session.LastActive.Before(now) {
		session.LastActive = now
	}
	session = normalizeSessionForStorage(session, now)
	updatedMessage := types.Message{}
	for _, message := range session.Messages {
		if message.ID == messageID {
			updatedMessage = message
			break
		}
	}
	if updatedMessage.ID == "" {
		return types.Message{}, storageNotFound("message was not found", nil)
	}
	if _, err := s.writeSessionData(ctx, session, now); err != nil {
		return types.Message{}, err
	}
	if err := s.rebuildSessionIndexes(ctx, roleID); err != nil {
		return types.Message{}, err
	}
	return updatedMessage, nil
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
