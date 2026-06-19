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

type sessionScopeKind string

const (
	sessionScopeRole      sessionScopeKind = "role"
	sessionScopeGroup     sessionScopeKind = "group"
	sessionScopeWorkspace sessionScopeKind = "workspace"
)

type sessionScope struct {
	Kind        sessionScopeKind
	ID          string
	WorkspaceID string
	RoleID      string
}

func roleSessionScope(roleID string) sessionScope {
	return sessionScope{Kind: sessionScopeRole, ID: strings.TrimSpace(roleID)}
}

func groupSessionScope(groupID string) sessionScope {
	return sessionScope{Kind: sessionScopeGroup, ID: strings.TrimSpace(groupID)}
}

func workspaceSessionScope(workspaceID string, roleID string) sessionScope {
	workspaceID = strings.TrimSpace(workspaceID)
	roleID = strings.TrimSpace(roleID)
	return sessionScope{Kind: sessionScopeWorkspace, ID: workspaceID + "/" + roleID, WorkspaceID: workspaceID, RoleID: roleID}
}

func sessionScopeFromSession(session types.Session) (sessionScope, error) {
	groupID := strings.TrimSpace(session.GroupID)
	workspaceID := strings.TrimSpace(session.WorkspaceID)
	roleID := strings.TrimSpace(session.RoleID)
	if groupID != "" {
		if roleID != "" {
			return sessionScope{}, storageInvalid("session cannot belong to both role and group", nil)
		}
		if workspaceID != "" {
			return sessionScope{}, storageInvalid("session cannot belong to both group and workspace", nil)
		}
		return cleanSessionScope(groupSessionScope(groupID))
	}
	if workspaceID != "" {
		if roleID == "" {
			return sessionScope{}, storageInvalid("workspace session roleId is required", nil)
		}
		if _, err := cleanID(roleID); err != nil {
			return sessionScope{}, err
		}
		return cleanSessionScope(workspaceSessionScope(workspaceID, roleID))
	}
	return cleanSessionScope(roleSessionScope(roleID))
}

func cleanSessionScope(scope sessionScope) (sessionScope, error) {
	scope.ID = strings.TrimSpace(scope.ID)
	scope.WorkspaceID = strings.TrimSpace(scope.WorkspaceID)
	scope.RoleID = strings.TrimSpace(scope.RoleID)
	if scope.Kind != sessionScopeRole && scope.Kind != sessionScopeGroup && scope.Kind != sessionScopeWorkspace {
		return sessionScope{}, storageInvalid("session scope is invalid", nil)
	}
	if scope.Kind == sessionScopeWorkspace {
		if _, err := cleanID(scope.WorkspaceID); err != nil {
			return sessionScope{}, err
		}
		if _, err := cleanID(scope.RoleID); err != nil {
			return sessionScope{}, err
		}
		scope.ID = scope.WorkspaceID + "/" + scope.RoleID
		return scope, nil
	}
	if _, err := cleanID(scope.ID); err != nil {
		return sessionScope{}, err
	}
	return scope, nil
}

func (s *system) sessionDataFile(scope sessionScope, sessionID string) (string, error) {
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return "", err
	}
	if scope.Kind == sessionScopeGroup {
		return s.paths.groupSessionDataFile(scope.ID, sessionID)
	}
	if scope.Kind == sessionScopeWorkspace {
		return s.paths.workspaceSessionDataFile(scope.WorkspaceID, scope.RoleID, sessionID)
	}
	return s.paths.sessionDataFile(scope.ID, sessionID)
}

func (s *system) sessionDir(scope sessionScope, sessionID string) (string, error) {
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return "", err
	}
	if scope.Kind == sessionScopeGroup {
		return s.paths.groupSessionDir(scope.ID, sessionID)
	}
	if scope.Kind == sessionScopeWorkspace {
		return s.paths.workspaceSessionDir(scope.WorkspaceID, scope.RoleID, sessionID)
	}
	return s.paths.sessionDir(scope.ID, sessionID)
}

func (s *system) sessionAttachmentsDir(scope sessionScope, sessionID string) (string, error) {
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return "", err
	}
	if scope.Kind == sessionScopeGroup {
		return s.paths.groupSessionAttachmentsDir(scope.ID, sessionID)
	}
	if scope.Kind == sessionScopeWorkspace {
		return s.paths.workspaceSessionAttachmentsDir(scope.WorkspaceID, scope.RoleID, sessionID)
	}
	return s.paths.sessionAttachmentsDir(scope.ID, sessionID)
}

func (s *system) sessionScopeDir(scope sessionScope) (string, error) {
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return "", err
	}
	if scope.Kind == sessionScopeGroup {
		return s.paths.sessionGroupDir(scope.ID)
	}
	if scope.Kind == sessionScopeWorkspace {
		return s.paths.workspaceRoleSessionsDir(scope.WorkspaceID, scope.RoleID)
	}
	return s.paths.sessionRoleDir(scope.ID)
}

func (s *system) CreateSession(ctx context.Context, roleID string, title string) (types.Session, error) {
	scope, err := cleanSessionScope(roleSessionScope(roleID))
	if err != nil {
		return types.Session{}, err
	}
	return s.createSession(ctx, scope, title)
}

func (s *system) CreateGroupSession(ctx context.Context, groupID string, title string) (types.Session, error) {
	scope, err := cleanSessionScope(groupSessionScope(groupID))
	if err != nil {
		return types.Session{}, err
	}
	return s.createSession(ctx, scope, title)
}

func (s *system) CreateWorkspaceSession(ctx context.Context, workspaceID string, roleID string, title string) (types.Session, error) {
	scope, err := cleanSessionScope(workspaceSessionScope(workspaceID, roleID))
	if err != nil {
		return types.Session{}, err
	}
	if _, err := cleanID(roleID); err != nil {
		return types.Session{}, err
	}
	sessionTitle := strings.TrimSpace(title)
	if sessionTitle == "" {
		sessionTitle = types.DefaultSessionTitle
	}
	now := time.Now().UTC()
	session := types.Session{ID: utils.NewID("session"), WorkspaceID: scope.ID, RoleID: strings.TrimSpace(roleID), Title: normalizeSessionTitle(sessionTitle), Status: string(types.RunStatusCreated), Messages: []types.Message{}, CreatedAt: now, UpdatedAt: now, LastActive: now}
	if err := s.SaveSession(ctx, session); err != nil {
		return types.Session{}, err
	}
	return session, nil
}

func (s *system) createSession(ctx context.Context, scope sessionScope, title string) (types.Session, error) {
	sessionTitle := strings.TrimSpace(title)
	if sessionTitle == "" {
		sessionTitle = types.DefaultSessionTitle
	}
	now := time.Now().UTC()
	session := types.Session{
		ID:         utils.NewID("session"),
		Title:      normalizeSessionTitle(sessionTitle),
		Status:     string(types.RunStatusCreated),
		Messages:   []types.Message{},
		CreatedAt:  now,
		UpdatedAt:  now,
		LastActive: now,
	}
	if scope.Kind == sessionScopeGroup {
		session.GroupID = scope.ID
	} else if scope.Kind == sessionScopeWorkspace {
		session.WorkspaceID = scope.ID
	} else {
		session.RoleID = scope.ID
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
	scope, err := sessionScopeFromSession(session)
	if err != nil {
		return types.Session{}, err
	}
	if _, err := cleanID(session.ID); err != nil {
		return types.Session{}, err
	}
	target, err := s.sessionDataFile(scope, session.ID)
	if err != nil {
		return types.Session{}, err
	}
	if err := writeJSON(ctx, target, toSessionStorageDocument(session)); err != nil {
		return types.Session{}, err
	}
	attachmentsDir, err := s.sessionAttachmentsDir(scope, session.ID)
	if err != nil {
		return types.Session{}, err
	}
	if err := ensureDirs(attachmentsDir); err != nil {
		return types.Session{}, storageWriteFailed("failed to create session attachments directory", err)
	}
	return session, nil
}

func (s *system) LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error) {
	return s.loadSession(ctx, roleSessionScope(roleID), sessionID)
}

func (s *system) LoadGroupSession(ctx context.Context, groupID string, sessionID string) (types.Session, error) {
	return s.loadSession(ctx, groupSessionScope(groupID), sessionID)
}

func (s *system) LoadWorkspaceSession(ctx context.Context, workspaceID string, roleID string, sessionID string) (types.Session, error) {
	return s.loadSession(ctx, workspaceSessionScope(workspaceID, roleID), sessionID)
}

func (s *system) loadSession(ctx context.Context, scope sessionScope, sessionID string) (types.Session, error) {
	target, err := s.sessionDataFile(scope, sessionID)
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
	if _, err := sessionScopeFromSession(session); err != nil {
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
	merged.Metadata = applySessionMetadataPatch(merged.Metadata, save.MetadataPatch)
	merged.AsyncToolTasks = mergeAsyncToolTasks(merged.AsyncToolTasks, session.AsyncToolTasks)
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

func mergeAsyncToolTasks(current []types.AsyncToolTask, incoming []types.AsyncToolTask) []types.AsyncToolTask {
	if len(current) == 0 && len(incoming) == 0 {
		return nil
	}
	byID := map[string]types.AsyncToolTask{}
	order := []string{}
	add := func(task types.AsyncToolTask) {
		id := strings.TrimSpace(task.ID)
		if id == "" {
			return
		}
		if _, exists := byID[id]; !exists {
			order = append(order, id)
		}
		byID[id] = task
	}
	for _, task := range current {
		add(task)
	}
	for _, task := range incoming {
		add(task)
	}
	result := make([]types.AsyncToolTask, 0, len(order))
	for _, id := range order {
		result = append(result, byID[id])
	}
	return result
}

func (s *system) readSessionForMessageSave(ctx context.Context, fallback types.Session, now time.Time) (types.Session, bool, error) {
	scope, err := sessionScopeFromSession(fallback)
	if err != nil {
		return types.Session{}, false, err
	}
	loaded, err := s.loadSession(ctx, scope, fallback.ID)
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

func applySessionMetadataPatch(current map[string]string, patch map[string]string) map[string]string {
	if len(patch) == 0 {
		return current
	}
	out := make(map[string]string, len(current)+len(patch))
	for key, value := range current {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	for key, value := range patch {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			delete(out, key)
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
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
	return s.listSessions(ctx, roleSessionScope(roleID))
}

func (s *system) ListGroupSessions(ctx context.Context, groupID string) ([]types.SessionSummary, error) {
	return s.listSessions(ctx, groupSessionScope(groupID))
}

func (s *system) ListWorkspaceSessions(ctx context.Context, workspaceID string, roleID string) ([]types.SessionSummary, error) {
	return s.listSessions(ctx, workspaceSessionScope(workspaceID, roleID))
}

func (s *system) listSessions(ctx context.Context, scope sessionScope) ([]types.SessionSummary, error) {
	scopeDir, err := s.sessionScopeDir(scope)
	if err != nil {
		return nil, err
	}
	sessions, err := readObjects[types.Session](ctx, scopeDir)
	if err != nil {
		return nil, err
	}
	summaries := make([]types.SessionSummary, 0, len(sessions))
	for _, session := range sessions {
		session = normalizeSessionForStorage(session, time.Now().UTC())
		summaries = append(summaries, types.SessionSummary{ID: session.ID, RoleID: session.RoleID, GroupID: session.GroupID, WorkspaceID: session.WorkspaceID, Title: session.Title, Status: session.Status, UpdatedAt: session.UpdatedAt, LastActive: session.LastActive})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastActive.After(summaries[j].LastActive)
	})
	return summaries, nil
}

func (s *system) DeleteSession(ctx context.Context, roleID string, sessionID string) error {
	return s.deleteSession(ctx, roleSessionScope(roleID), sessionID)
}

func (s *system) DeleteGroupSession(ctx context.Context, groupID string, sessionID string) error {
	return s.deleteSession(ctx, groupSessionScope(groupID), sessionID)
}

func (s *system) DeleteWorkspaceSession(ctx context.Context, workspaceID string, roleID string, sessionID string) error {
	return s.deleteSession(ctx, workspaceSessionScope(workspaceID, roleID), sessionID)
}

func (s *system) deleteSession(ctx context.Context, scope sessionScope, sessionID string) error {
	dir, err := s.sessionDir(scope, sessionID)
	if err != nil {
		return err
	}
	recycleID := sessionID
	if scope.Kind == sessionScopeGroup {
		recycleID = "groups-" + scope.ID + "-" + sessionID
	} else if scope.Kind == sessionScopeWorkspace {
		recycleID = "workspaces-" + scope.WorkspaceID + "-" + scope.RoleID + "-" + sessionID
	}
	if err := moveToRecycle(ctx, s.paths, types.StorageItemSession, recycleID, dir); err != nil {
		return err
	}
	return s.rebuildAllSessionIndexes(ctx)
}
