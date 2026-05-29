package datastorage

import (
	"context"
	"sort"

	"eucli-box/pkg/types"
)

func (s *system) SaveSession(ctx context.Context, session types.Session) error {
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
	return s.rebuildAllSessionIndexes(ctx)
}

func (s *system) LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error) {
	target, err := s.paths.sessionDataFile(roleID, sessionID)
	if err != nil {
		return types.Session{}, err
	}
	return readJSON[types.Session](ctx, target)
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
		summaries = append(summaries, types.SessionSummary{ID: session.ID, RoleID: session.RoleID, Title: session.Title, Status: session.Status, LastActive: session.LastActive})
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
