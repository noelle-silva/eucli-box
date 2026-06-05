package agentruntime

import (
	"context"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) saveRunSession(ctx context.Context, record *runRecord, status types.RunStatus) error {
	record.session.Status = string(status)
	save := runSessionMessageSave(record, status)
	if err := s.storage.SaveSessionMessages(ctx, save); err != nil {
		return runtimeStorageFailed("failed to save session", err)
	}
	markRunSessionSaveAccepted(record, save)
	return nil
}

func (s *system) saveRunSessionStatus(ctx context.Context, record *runRecord, status types.RunStatus) error {
	record.session.Status = string(status)
	if err := s.storage.SaveSessionMessages(ctx, types.SessionMessageSave{Session: record.session, Status: status}); err != nil {
		return runtimeStorageFailed("failed to save session status", err)
	}
	return nil
}

func (s *system) saveRunSessionWithStatusFallback(ctx context.Context, record *runRecord, status types.RunStatus) (bool, error) {
	err := s.saveRunSession(ctx, record, status)
	if err == nil {
		return true, nil
	}
	if !isStorageConflict(err) {
		return false, err
	}
	if err := s.saveRunSessionStatus(ctx, record, status); err != nil {
		return false, err
	}
	return false, nil
}

func runSessionMessageSave(record *runRecord, status types.RunStatus) types.SessionMessageSave {
	return types.SessionMessageSave{Session: record.session, Writes: runMessageWrites(record), Deletes: runMessageDeletes(record), Conditions: runMessageConditions(record), Status: status}
}

func runMessageWrites(record *runRecord) []types.SessionMessageWrite {
	if record == nil || len(record.ownedMessageIDs) == 0 {
		return nil
	}
	byID := messagesByID(record.session.Messages)
	writes := make([]types.SessionMessageWrite, 0, len(record.ownedMessageIDs))
	for _, messageID := range sortedRecordIDs(record.ownedMessageIDs) {
		message, ok := byID[messageID]
		if !ok {
			continue
		}
		write := types.SessionMessageWrite{Message: cloneRunMessageSnapshot(message)}
		if expected, ok := record.messageSnapshots[messageID]; ok {
			snapshot := cloneRunMessageSnapshot(expected)
			write.Expected = &snapshot
		}
		writes = append(writes, write)
	}
	return writes
}

func runMessageDeletes(record *runRecord) []types.SessionMessageDelete {
	if record == nil || len(record.deletedMessageIDs) == 0 {
		return nil
	}
	deletes := make([]types.SessionMessageDelete, 0, len(record.deletedMessageIDs))
	for _, messageID := range sortedRecordIDs(record.deletedMessageIDs) {
		expected, ok := record.messageSnapshots[messageID]
		if !ok {
			continue
		}
		snapshot := cloneRunMessageSnapshot(expected)
		deletes = append(deletes, types.SessionMessageDelete{MessageID: messageID, Expected: &snapshot})
	}
	return deletes
}

func runMessageConditions(record *runRecord) []types.SessionMessageCondition {
	if record == nil || len(record.dependencyIDs) == 0 {
		return nil
	}
	conditions := make([]types.SessionMessageCondition, 0, len(record.dependencyIDs))
	for _, messageID := range sortedRecordIDs(record.dependencyIDs) {
		if isRunOwnedMessage(record, messageID) {
			continue
		}
		expected, ok := record.messageSnapshots[messageID]
		if !ok {
			continue
		}
		snapshot := cloneRunMessageSnapshot(expected)
		conditions = append(conditions, types.SessionMessageCondition{MessageID: messageID, Expected: &snapshot})
	}
	return conditions
}

func markRunSessionSaveAccepted(record *runRecord, save types.SessionMessageSave) {
	if record == nil {
		return
	}
	if record.messageSnapshots == nil {
		record.messageSnapshots = map[string]types.Message{}
	}
	for _, write := range save.Writes {
		messageID := strings.TrimSpace(write.Message.ID)
		if messageID == "" {
			continue
		}
		record.messageSnapshots[messageID] = cloneRunMessageSnapshot(write.Message)
	}
	for _, removed := range save.Deletes {
		messageID := strings.TrimSpace(removed.MessageID)
		if messageID == "" {
			continue
		}
		delete(record.deletedMessageIDs, messageID)
		delete(record.ownedMessageIDs, messageID)
		delete(record.dependencyIDs, messageID)
		delete(record.messageSnapshots, messageID)
	}
}

func sortedRecordIDs(ids map[string]struct{}) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func (s *system) updateRun(runID string, status types.RunStatus, reason string) (types.RunState, error) {
	return s.updateRunWithError(runID, status, reason, nil)
}

func (s *system) updateRunWithError(runID string, status types.RunStatus, reason string, errPayload *types.ErrorPayload) (types.RunState, error) {
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
	record.state.Error = cloneErrorPayload(errPayload)
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

func (s *system) hasActiveRunAtAnchor(record *runRecord) bool {
	if record == nil || strings.TrimSpace(record.anchorMessageID) == "" || strings.TrimSpace(record.session.ID) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, other := range s.runs {
		if other == nil || other.runID == record.runID {
			continue
		}
		if strings.TrimSpace(other.roleID) != strings.TrimSpace(record.roleID) {
			continue
		}
		if strings.TrimSpace(other.state.SessionID) != strings.TrimSpace(record.session.ID) {
			continue
		}
		if strings.TrimSpace(other.anchorMessageID) != strings.TrimSpace(record.anchorMessageID) {
			continue
		}
		if other.state.Status == types.RunStatusRunning || other.state.Status == types.RunStatusWaitingConfirmation || other.state.Status == types.RunStatusCreated {
			return true
		}
	}
	return false
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
