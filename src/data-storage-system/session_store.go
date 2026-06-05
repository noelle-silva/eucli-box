package datastorage

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func (s *system) CreateSession(ctx context.Context, roleID string, title string) (types.Session, error) {
	roleID = strings.TrimSpace(roleID)
	if _, err := cleanID(roleID); err != nil {
		return types.Session{}, err
	}
	sessionTitle := strings.TrimSpace(title)
	if sessionTitle == "" {
		sessionTitle = types.DefaultSessionTitle
	}
	now := time.Now().UTC()
	session := types.Session{
		ID:         utils.NewID("session"),
		RoleID:     roleID,
		Title:      normalizeSessionTitle(sessionTitle),
		Status:     string(types.RunStatusCreated),
		Messages:   []types.Message{},
		CreatedAt:  now,
		UpdatedAt:  now,
		LastActive: now,
	}
	if err := s.SaveSession(ctx, session); err != nil {
		return types.Session{}, err
	}
	return session, nil
}

func (s *system) SaveSession(ctx context.Context, session types.Session) error {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if _, err := s.writeSessionData(ctx, session, time.Now().UTC()); err != nil {
		return err
	}
	return s.rebuildAllSessionIndexes(ctx)
}

func (s *system) SaveSessionMessages(ctx context.Context, save types.SessionMessageSave) error {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	now := time.Now().UTC()
	merged, err := s.mergeSessionMessageSave(ctx, save, now)
	if err != nil {
		return err
	}
	if _, err := s.writeSessionData(ctx, merged, now); err != nil {
		return err
	}
	return s.rebuildAllSessionIndexes(ctx)
}

func (s *system) writeSessionData(ctx context.Context, session types.Session, now time.Time) (types.Session, error) {
	session = normalizeSessionForStorage(session, now)
	if _, err := cleanID(session.RoleID); err != nil {
		return types.Session{}, err
	}
	if _, err := cleanID(session.ID); err != nil {
		return types.Session{}, err
	}
	target, err := s.paths.sessionDataFile(session.RoleID, session.ID)
	if err != nil {
		return types.Session{}, err
	}
	if err := writeJSON(ctx, target, toSessionStorageDocument(session)); err != nil {
		return types.Session{}, err
	}
	attachmentsDir, err := s.paths.sessionAttachmentsDir(session.RoleID, session.ID)
	if err != nil {
		return types.Session{}, err
	}
	if err := ensureDirs(attachmentsDir); err != nil {
		return types.Session{}, storageWriteFailed("failed to create session attachments directory", err)
	}
	return session, nil
}

func (s *system) LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error) {
	target, err := s.paths.sessionDataFile(roleID, sessionID)
	if err != nil {
		return types.Session{}, err
	}
	session, err := readJSON[types.Session](ctx, target)
	if err != nil {
		return types.Session{}, err
	}
	return normalizeSessionForStorage(session, time.Now().UTC()), nil
}

func (s *system) mergeSessionMessageSave(ctx context.Context, save types.SessionMessageSave, now time.Time) (types.Session, error) {
	session := save.Session
	if _, err := cleanID(session.RoleID); err != nil {
		return types.Session{}, err
	}
	if _, err := cleanID(session.ID); err != nil {
		return types.Session{}, err
	}
	merged, loaded, err := s.readSessionForMessageSave(ctx, session, now)
	if err != nil {
		return types.Session{}, err
	}
	merged.Status = string(save.Status)
	if loaded {
		merged.Metadata = mergeSessionMetadataPreservingCurrent(merged.Metadata, session.Metadata)
	} else {
		merged.Metadata = mergeSessionMetadata(session.Metadata, merged.Metadata)
	}
	if merged.Title == "" || merged.Title == types.DefaultSessionTitle {
		merged.Title = session.Title
	}
	if merged.CreatedAt.IsZero() || (!session.CreatedAt.IsZero() && session.CreatedAt.Before(merged.CreatedAt)) {
		merged.CreatedAt = session.CreatedAt
	}
	if session.UpdatedAt.After(merged.UpdatedAt) {
		merged.UpdatedAt = session.UpdatedAt
	}
	if session.LastActive.After(merged.LastActive) {
		merged.LastActive = session.LastActive
	}
	if err := validateSessionMessageConditions(merged, save.Conditions); err != nil {
		return types.Session{}, err
	}
	for _, delete := range save.Deletes {
		if err := validateSessionMessageDelete(merged, delete); err != nil {
			return types.Session{}, err
		}
	}
	validated := merged
	for _, delete := range save.Deletes {
		validated = removeStoredSessionMessage(validated, delete.MessageID)
	}
	for _, write := range save.Writes {
		if err := validateSessionMessageWrite(validated, write); err != nil {
			return types.Session{}, err
		}
		validated = upsertStoredSessionMessage(validated, write.Message)
	}
	merged = validated
	if !now.IsZero() && merged.UpdatedAt.Before(now) && (len(save.Writes) > 0 || len(save.Deletes) > 0) {
		merged.UpdatedAt = now
	}
	return merged, nil
}

func (s *system) readSessionForMessageSave(ctx context.Context, fallback types.Session, now time.Time) (types.Session, bool, error) {
	loaded, err := s.LoadSession(ctx, fallback.RoleID, fallback.ID)
	if err == nil {
		return loaded, true, nil
	}
	if !isMissingSessionData(err) {
		return types.Session{}, false, err
	}
	fallback.Messages = nil
	return normalizeSessionForStorage(fallback, now), false, nil
}

func isMissingSessionData(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, os.ErrNotExist)
}

func mergeSessionMetadata(base map[string]string, incoming map[string]string) map[string]string {
	if len(base) == 0 && len(incoming) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(incoming))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range incoming {
		out[key] = value
	}
	return out
}

func mergeSessionMetadataPreservingCurrent(current map[string]string, incoming map[string]string) map[string]string {
	if len(current) == 0 && len(incoming) == 0 {
		return nil
	}
	out := make(map[string]string, len(current)+len(incoming))
	for key, value := range incoming {
		out[key] = value
	}
	for key, value := range current {
		out[key] = value
	}
	return out
}

func validateSessionMessageConditions(session types.Session, conditions []types.SessionMessageCondition) error {
	for _, condition := range conditions {
		messageID := strings.TrimSpace(condition.MessageID)
		if messageID == "" && condition.Expected != nil {
			messageID = strings.TrimSpace(condition.Expected.ID)
		}
		if err := validateStoredMessageExpected(session, messageID, condition.Expected); err != nil {
			return err
		}
	}
	return nil
}

func validateSessionMessageWrite(session types.Session, write types.SessionMessageWrite) error {
	messageID := strings.TrimSpace(write.Message.ID)
	if messageID == "" {
		return storageInvalid("message id is required", nil)
	}
	if err := validateStoredMessageExpected(session, messageID, write.Expected); err != nil {
		return err
	}
	if shouldValidateMessageBranchSlot(session, write.Message) {
		return validateMessageBranchSlotAvailable(session, write.Message)
	}
	return nil
}

func validateSessionMessageDelete(session types.Session, delete types.SessionMessageDelete) error {
	messageID := strings.TrimSpace(delete.MessageID)
	if messageID == "" && delete.Expected != nil {
		messageID = strings.TrimSpace(delete.Expected.ID)
	}
	if messageID == "" {
		return storageInvalid("message id is required", nil)
	}
	return validateStoredMessageExpected(session, messageID, delete.Expected)
}

func validateStoredMessageExpected(session types.Session, messageID string, expected *types.Message) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return storageInvalid("message id is required", nil)
	}
	current, ok := storedSessionMessageByID(session, messageID)
	if expected == nil {
		if ok {
			return storageConflict("message already exists", nil)
		}
		return nil
	}
	if !ok {
		return storageConflict("message was changed or deleted", nil)
	}
	if !storedMessageMatchesExpected(current, *expected) {
		return storageConflict("message was changed or deleted", nil)
	}
	return nil
}

func storedMessageMatchesExpected(current types.Message, expected types.Message) bool {
	if strings.TrimSpace(current.ID) == "" || strings.TrimSpace(current.ID) != strings.TrimSpace(expected.ID) {
		return false
	}
	if current.Type != expected.Type || strings.TrimSpace(current.ParentMessageID) != strings.TrimSpace(expected.ParentMessageID) || normalizeBranchID(current.BranchID) != normalizeBranchID(expected.BranchID) {
		return false
	}
	if !current.UpdatedAt.IsZero() || !expected.UpdatedAt.IsZero() {
		return current.UpdatedAt.Equal(expected.UpdatedAt)
	}
	return current.CreatedAt.Equal(expected.CreatedAt) && current.Content == expected.Content
}

func shouldValidateMessageBranchSlot(session types.Session, message types.Message) bool {
	current, ok := storedSessionMessageByID(session, message.ID)
	if !ok {
		return true
	}
	return !sameMessageBranchSlot(current, message)
}

func validateMessageBranchSlotAvailable(session types.Session, message types.Message) error {
	messageID := strings.TrimSpace(message.ID)
	if messageID == "" {
		return storageInvalid("message id is required", nil)
	}
	for _, current := range session.Messages {
		if strings.TrimSpace(current.ID) == messageID {
			continue
		}
		if sameMessageBranchSlot(current, message) {
			return storageConflict("message branch slot is already occupied", nil)
		}
	}
	return nil
}

func sameMessageBranchSlot(left types.Message, right types.Message) bool {
	return strings.TrimSpace(left.ParentMessageID) == strings.TrimSpace(right.ParentMessageID) && normalizeBranchID(left.BranchID) == normalizeBranchID(right.BranchID)
}

func storedSessionMessageByID(session types.Session, messageID string) (types.Message, bool) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return types.Message{}, false
	}
	for _, message := range session.Messages {
		if strings.TrimSpace(message.ID) == messageID {
			return message, true
		}
	}
	return types.Message{}, false
}

func removeStoredSessionMessage(session types.Session, messageID string) types.Session {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return session
	}
	next := make([]types.Message, 0, len(session.Messages))
	removed := false
	for _, message := range session.Messages {
		if strings.TrimSpace(message.ID) == messageID {
			removed = true
			continue
		}
		next = append(next, message)
	}
	if removed {
		session.Messages = next
	}
	return session
}

func upsertStoredSessionMessage(session types.Session, message types.Message) types.Session {
	messageID := strings.TrimSpace(message.ID)
	if messageID == "" {
		return session
	}
	for index := range session.Messages {
		if session.Messages[index].ID != messageID {
			continue
		}
		session.Messages[index] = message
		if message.UpdatedAt.After(session.UpdatedAt) {
			session.UpdatedAt = message.UpdatedAt
		}
		if message.CreatedAt.After(session.LastActive) {
			session.LastActive = message.CreatedAt
		}
		return session
	}
	session.Messages = append(session.Messages, message)
	if message.UpdatedAt.After(session.UpdatedAt) {
		session.UpdatedAt = message.UpdatedAt
	}
	if message.CreatedAt.After(session.LastActive) {
		session.LastActive = message.CreatedAt
	}
	return session
}

func (s *system) ListSessions(ctx context.Context, roleID string) ([]types.SessionSummary, error) {
	roleDir, err := s.paths.sessionRoleDir(roleID)
	if err != nil {
		return nil, err
	}
	sessions, err := readObjects[types.Session](ctx, roleDir)
	if err != nil {
		return nil, err
	}
	summaries := make([]types.SessionSummary, 0, len(sessions))
	for _, session := range sessions {
		session = normalizeSessionForStorage(session, time.Now().UTC())
		summaries = append(summaries, types.SessionSummary{ID: session.ID, RoleID: session.RoleID, Title: session.Title, Status: session.Status, UpdatedAt: session.UpdatedAt, LastActive: session.LastActive})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastActive.After(summaries[j].LastActive)
	})
	return summaries, nil
}

func (s *system) DeleteSession(ctx context.Context, roleID string, sessionID string) error {
	dir, err := s.paths.sessionDir(roleID, sessionID)
	if err != nil {
		return err
	}
	if err := moveToRecycle(ctx, s.paths, types.StorageItemSession, sessionID, dir); err != nil {
		return err
	}
	return s.rebuildAllSessionIndexes(ctx)
}
