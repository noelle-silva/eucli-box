package agentruntime

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) saveSession(ctx context.Context, session types.Session, status types.RunStatus) error {
	session.Status = string(status)
	if err := s.storage.SaveSession(ctx, session); err != nil {
		return runtimeStorageFailed("failed to save session", err)
	}
	return nil
}

func (s *system) updateRun(runID string, status types.RunStatus, reason string) (types.RunState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.runs[runID]
	if !ok {
		return types.RunState{}, runtimeNotFound("run was not found", nil)
	}
	if !canTransition(record.state.Status, status) {
		return types.RunState{}, runtimeStateInvalid("invalid run state transition", nil)
	}
	record.state.Status = status
	record.state.Reason = reason
	record.state.UpdatedAt = nowUTC()
	return record.state, nil
}

func (s *system) setRunSessionID(runID string, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.runs[runID]
	if !ok {
		return runtimeNotFound("run was not found", nil)
	}
	record.state.SessionID = sessionID
	record.state.UpdatedAt = nowUTC()
	return nil
}

func (s *system) setRunMessageIDs(runID string, inputMessageID string, lastMessageID string) error {
	inputMessageID = strings.TrimSpace(inputMessageID)
	lastMessageID = strings.TrimSpace(lastMessageID)
	if inputMessageID == "" && lastMessageID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.runs[runID]
	if !ok {
		return runtimeNotFound("run was not found", nil)
	}
	if inputMessageID != "" {
		record.inputMessageID = inputMessageID
		record.state.InputMessageID = inputMessageID
	}
	if lastMessageID != "" {
		record.lastMessageID = lastMessageID
		record.state.LastMessageID = lastMessageID
	}
	record.state.UpdatedAt = nowUTC()
	return nil
}

func canTransition(from types.RunStatus, to types.RunStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case types.RunStatusCreated:
		return to == types.RunStatusRunning || to == types.RunStatusFailed || to == types.RunStatusCancelled
	case types.RunStatusRunning:
		return to == types.RunStatusWaitingConfirmation || to == types.RunStatusCompleted || to == types.RunStatusFailed || to == types.RunStatusCancelled
	case types.RunStatusWaitingConfirmation:
		return to == types.RunStatusRunning || to == types.RunStatusFailed || to == types.RunStatusCancelled
	default:
		return false
	}
}
