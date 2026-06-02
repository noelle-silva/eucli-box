package datastorage

import (
	"context"
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
		sessionTitle = "新聊天"
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
	session = normalizeSessionForStorage(session, time.Now().UTC())
	if _, err := cleanID(session.RoleID); err != nil {
		return err
	}
	if _, err := cleanID(session.ID); err != nil {
		return err
	}
	target, err := s.paths.sessionDataFile(session.RoleID, session.ID)
	if err != nil {
		return err
	}
	if err := writeJSON(ctx, target, session); err != nil {
		return err
	}
	attachmentsDir, err := s.paths.sessionAttachmentsDir(session.RoleID, session.ID)
	if err != nil {
		return err
	}
	if err := ensureDirs(attachmentsDir); err != nil {
		return storageWriteFailed("failed to create session attachments directory", err)
	}
	return s.rebuildAllSessionIndexes(ctx)
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
