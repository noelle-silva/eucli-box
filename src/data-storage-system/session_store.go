package datastorage

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

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
	now := time.Now().UTC()
	session := types.Session{ID: utils.NewID("session"), WorkspaceID: scope.WorkspaceID, RoleID: scope.RoleID, Title: types.NormalizeSessionTitle(title), Status: string(types.RunStatusCreated), Messages: []types.Message{}, CreatedAt: now, UpdatedAt: now, LastActive: now}
	if err := s.SaveSession(ctx, session); err != nil {
		return types.Session{}, err
	}
	return session, nil
}

func (s *system) createSession(ctx context.Context, scope sessionScope, title string) (types.Session, error) {
	now := time.Now().UTC()
	session := types.Session{
		ID:         utils.NewID("session"),
		Title:      types.NormalizeSessionTitle(title),
		Status:     string(types.RunStatusCreated),
		Messages:   []types.Message{},
		CreatedAt:  now,
		UpdatedAt:  now,
		LastActive: now,
	}
	if scope.Kind == sessionScopeGroup {
		session.GroupID = scope.ID
	} else if scope.Kind == sessionScopeWorkspace {
		session.WorkspaceID = scope.WorkspaceID
	} else {
		session.RoleID = scope.ID
	}
	if err := s.SaveSession(ctx, session); err != nil {
		return types.Session{}, err
	}
	return session, nil
}

func (s *system) SaveSession(ctx context.Context, session types.Session) error {
	_, err := s.persistSession(ctx, session, time.Now().UTC())
	return err
}

// persistSession 是整份会话覆盖写：写 data.json，并把本 scope 索引里的单条摘要 upsert。
// 它不读回旧会话，也不触发任何全库重建。
func (s *system) persistSession(ctx context.Context, session types.Session, now time.Time) (types.Session, error) {
	session = normalizeSessionForWrite(session, now)
	scope, err := sessionScopeFromSession(session)
	if err != nil {
		return types.Session{}, err
	}
	if _, err := cleanID(session.ID); err != nil {
		return types.Session{}, err
	}
	if err := s.ensureScopeCatalog(ctx, scope); err != nil {
		return types.Session{}, err
	}
	lock, err := s.scopeLock(scope)
	if err != nil {
		return types.Session{}, err
	}
	lock.writers.Lock()
	defer lock.writers.Unlock()
	if err := s.commitSessionLocked(ctx, scope, session, lock); err != nil {
		return types.Session{}, err
	}
	return session, nil
}

// commitSessionLocked 假定调用方已持有 scope 写者锁。
// 写序严格为：索引 dirty=true → 数据 → 索引 dirty=false，
// 三份文档的编码与 fsync 都在锁外，临界区内只有三次 rename。
func (s *system) commitSessionLocked(ctx context.Context, scope sessionScope, session types.Session, lock *scopeLock) error {
	indexFile, err := s.sessionIndexFile(scope)
	if err != nil {
		return err
	}
	dataFile, err := s.sessionDataFile(scope, session.ID)
	if err != nil {
		return err
	}
	current, err := s.readScopeIndexForWriteLocked(ctx, scope, lock)
	if err != nil {
		return err
	}
	next := current
	next.SchemaVersion = sessionIndexSchemaVersion
	next.Sort = sessionIndexSort
	next.Dirty = false
	next.Rev = current.Rev + 1
	next.Sessions = sortSessionSummaries(upsertSessionSummary(append([]types.SessionSummary(nil), current.Sessions...), sessionSummaryFromStored(session, scope)))

	dirty := current
	dirty.SchemaVersion = sessionIndexSchemaVersion
	dirty.Sort = sessionIndexSort
	dirty.Dirty = true

	dirtyTemp, err := prepareJSON(ctx, indexFile, dirty)
	if err != nil {
		return s.failSessionWrite("failed to prepare session index dirty marker", err)
	}
	defer func() { _ = os.Remove(dirtyTemp) }()
	dataTemp, err := prepareJSON(ctx, dataFile, toSessionStorageDocument(session))
	if err != nil {
		return storageWriteFailed("failed to prepare session data", err)
	}
	defer func() { _ = os.Remove(dataTemp) }()
	// dirty 位必须先于数据持久化，否则崩溃后「数据已写、脏位丢失」会变成静默过期；
	// 最终索引是派生缓存，不 fsync：丢了会被 dirty 位或结构校验识别并重建。
	nextTemp, err := prepareJSONDerived(ctx, indexFile, next)
	if err != nil {
		return s.failSessionWrite("failed to prepare session index", err)
	}
	defer func() { _ = os.Remove(nextTemp) }()

	lock.mu.Lock()
	if err := commitJSON(dirtyTemp, indexFile); err != nil {
		lock.mu.Unlock()
		return s.failSessionWrite("failed to write session index dirty marker", err)
	}
	if err := commitJSON(dataTemp, dataFile); err != nil {
		lock.mu.Unlock()
		return storageWriteFailed("failed to replace session data", err)
	}
	if err := commitJSON(nextTemp, indexFile); err != nil {
		lock.mu.Unlock()
		return s.failSessionWrite("failed to write session index", err)
	}
	lock.mu.Unlock()

	s.sessionMetrics.indexWrites.Add(1)
	s.sessionMetrics.dataWrites.Add(1)
	attachmentsDir, err := s.sessionAttachmentsDir(scope, session.ID)
	if err != nil {
		return err
	}
	if err := ensureDirs(attachmentsDir); err != nil {
		return storageWriteFailed("failed to create session attachments directory", err)
	}
	return nil
}

// failSessionWrite 统一索引写失败的出口：绝不吞错、绝不降级为只写数据，
// 计数并记日志后如实返回 storage.index_failed，调用方可整文档重试（幂等）。
func (s *system) failSessionWrite(message string, err error) error {
	s.sessionMetrics.indexFailures.Add(1)
	log.Printf("data-storage-system: %s: %v", message, err)
	return storageIndexFailed(message, err)
}

func (s *system) SaveSessionMessages(ctx context.Context, save types.SessionMessageSave) error {
	scope, err := sessionScopeFromSession(save.Session)
	if err != nil {
		return err
	}
	if _, err := cleanID(save.Session.ID); err != nil {
		return err
	}
	if err := s.ensureScopeCatalog(ctx, scope); err != nil {
		return err
	}
	lock, err := s.scopeLock(scope)
	if err != nil {
		return err
	}
	lock.writers.Lock()
	defer lock.writers.Unlock()

	now := time.Now().UTC()
	stored, exists, err := s.readSessionIfExistsLocked(ctx, scope, save.Session.ID, lock)
	if err != nil {
		return err
	}
	merged, err := s.mergeSessionMessageSave(save, stored, exists, now)
	if err != nil {
		return err
	}
	return s.commitSessionLocked(ctx, scope, normalizeSessionForWrite(merged, now), lock)
}

func (s *system) UpdateSessionSettings(ctx context.Context, roleID string, sessionID string, patch types.SessionSettingsPatch) (types.Session, error) {
	return s.updateSessionSettings(ctx, roleSessionScope(roleID), sessionID, patch)
}

func (s *system) UpdateGroupSessionSettings(ctx context.Context, groupID string, sessionID string, patch types.SessionSettingsPatch) (types.Session, error) {
	return s.updateSessionSettings(ctx, groupSessionScope(groupID), sessionID, patch)
}

func (s *system) UpdateWorkspaceSessionSettings(ctx context.Context, workspaceID string, roleID string, sessionID string, patch types.SessionSettingsPatch) (types.Session, error) {
	return s.updateSessionSettings(ctx, workspaceSessionScope(workspaceID, roleID), sessionID, patch)
}

func (s *system) updateSessionSettings(ctx context.Context, scope sessionScope, sessionID string, patch types.SessionSettingsPatch) (types.Session, error) {
	return s.updateSessionAtomically(ctx, scope, sessionID, func(session *types.Session, _ time.Time) error {
		if patch.StreamEnabled != nil {
			session.Metadata = types.PutStreamEnabledSessionMetadata(session.Metadata, *patch.StreamEnabled)
		}
		if patch.ReasoningEffort != nil {
			effort := types.TrimReasoningEffort(types.ReasoningEffort(*patch.ReasoningEffort))
			if effort != "" && !types.IsReasoningEffort(effort) {
				return storageInvalid("reasoning effort is invalid", nil)
			}
			if len(session.Metadata) == 0 {
				session.Metadata = map[string]string{}
			}
			delete(session.Metadata, types.SessionMetadataReasoningEffort)
			if effort != "" {
				session.Metadata[types.SessionMetadataReasoningEffort] = string(effort)
			}
		}
		if patch.ModelOverride != nil {
			if strings.TrimSpace(patch.ModelOverride.ModelID) == "" {
				session.Metadata = types.ClearModelOverrideSessionMetadata(session.Metadata)
			} else {
				normalized, ok := types.NormalizeModelOverrideCoordinate(*patch.ModelOverride)
				if !ok {
					return storageInvalid("session model override is incomplete", nil)
				}
				session.Metadata = types.PutModelOverrideSessionMetadata(session.Metadata, normalized)
			}
		}
		return nil
	})
}

func (s *system) UpdateSessionHookPrompt(ctx context.Context, roleID string, sessionID string, selection types.HookPromptSelection) (types.Session, error) {
	return s.updateSessionHookPrompt(ctx, roleSessionScope(roleID), sessionID, selection)
}

func (s *system) UpdateGroupSessionHookPrompt(ctx context.Context, groupID string, sessionID string, selection types.HookPromptSelection) (types.Session, error) {
	return s.updateSessionHookPrompt(ctx, groupSessionScope(groupID), sessionID, selection)
}

func (s *system) UpdateWorkspaceSessionHookPrompt(ctx context.Context, workspaceID string, roleID string, sessionID string, selection types.HookPromptSelection) (types.Session, error) {
	return s.updateSessionHookPrompt(ctx, workspaceSessionScope(workspaceID, roleID), sessionID, selection)
}

func (s *system) updateSessionHookPrompt(ctx context.Context, scope sessionScope, sessionID string, selection types.HookPromptSelection) (types.Session, error) {
	return s.updateSessionAtomically(ctx, scope, sessionID, func(session *types.Session, _ time.Time) error {
		session.Metadata = types.PutHookPromptSessionMetadata(session.Metadata, selection)
		return nil
	})
}

func (s *system) UpdateSessionToolAuthorization(ctx context.Context, roleID string, sessionID string, toolID string, authorized bool) (types.Session, error) {
	return s.updateSessionToolAuthorization(ctx, roleSessionScope(roleID), sessionID, toolID, authorized)
}

func (s *system) UpdateGroupSessionToolAuthorization(ctx context.Context, groupID string, sessionID string, toolID string, authorized bool) (types.Session, error) {
	return s.updateSessionToolAuthorization(ctx, groupSessionScope(groupID), sessionID, toolID, authorized)
}

func (s *system) UpdateWorkspaceSessionToolAuthorization(ctx context.Context, workspaceID string, roleID string, sessionID string, toolID string, authorized bool) (types.Session, error) {
	return s.updateSessionToolAuthorization(ctx, workspaceSessionScope(workspaceID, roleID), sessionID, toolID, authorized)
}

func (s *system) updateSessionToolAuthorization(ctx context.Context, scope sessionScope, sessionID string, toolID string, authorized bool) (types.Session, error) {
	if strings.TrimSpace(toolID) == "" {
		return types.Session{}, storageInvalid("tool id is required", nil)
	}
	return s.updateSessionAtomically(ctx, scope, sessionID, func(session *types.Session, _ time.Time) error {
		session.Metadata = types.PutSessionToolAuthorization(session.Metadata, toolID, authorized)
		return nil
	})
}

func (s *system) updateSessionAtomically(ctx context.Context, scope sessionScope, sessionID string, mutate func(*types.Session, time.Time) error) (types.Session, error) {
	session, _, err := s.mutateSession(ctx, scope, sessionID, func(current *types.Session, now time.Time) (bool, error) {
		if err := mutate(current, now); err != nil {
			return false, err
		}
		return true, nil
	})
	return session, err
}

// mutateSession 是「读快照 → 内存变更 → 落名」的唯一写原语。
// 写者锁覆盖整段以保证读改写不丢更新；编解码与 fsync 全在 mu 之外。
func (s *system) mutateSession(ctx context.Context, scope sessionScope, sessionID string, mutate func(*types.Session, time.Time) (bool, error)) (types.Session, bool, error) {
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return types.Session{}, false, err
	}
	if _, err := cleanID(sessionID); err != nil {
		return types.Session{}, false, err
	}
	lock, err := s.scopeLock(scope)
	if err != nil {
		return types.Session{}, false, err
	}
	lock.writers.Lock()
	defer lock.writers.Unlock()

	current, err := s.readSessionLocked(ctx, scope, sessionID, lock)
	if err != nil {
		return types.Session{}, false, err
	}
	now := time.Now().UTC()
	write, err := mutate(&current, now)
	if err != nil {
		return types.Session{}, false, err
	}
	if !write {
		return current, false, nil
	}
	current.UpdatedAt = now
	if current.LastActive.Before(now) {
		current.LastActive = now
	}
	next := normalizeSessionForWrite(current, now)
	if err := s.commitSessionLocked(ctx, scope, next, lock); err != nil {
		return types.Session{}, false, err
	}
	return next, true, nil
}

// 公开会话读取只持 scope 读锁做一次 ReadFile，解码与归一化全在锁外。
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
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return types.Session{}, err
	}
	lock, err := s.scopeLock(scope)
	if err != nil {
		return types.Session{}, err
	}
	return s.readSessionLocked(ctx, scope, sessionID, lock)
}

func (s *system) readSessionLocked(ctx context.Context, scope sessionScope, sessionID string, lock *scopeLock) (types.Session, error) {
	target, err := s.sessionDataFile(scope, sessionID)
	if err != nil {
		return types.Session{}, err
	}
	lock.mu.RLock()
	payload, readErr := readFileBytes(ctx, target)
	lock.mu.RUnlock()
	if readErr != nil {
		return types.Session{}, readErr
	}
	session, decodeErr := decodeJSON[types.Session](payload)
	if decodeErr != nil {
		return types.Session{}, decodeErr
	}
	s.sessionMetrics.dataReads.Add(1)
	return normalizeSessionStored(session), nil
}

func (s *system) readSessionIfExistsLocked(ctx context.Context, scope sessionScope, sessionID string, lock *scopeLock) (types.Session, bool, error) {
	session, err := s.readSessionLocked(ctx, scope, sessionID, lock)
	if err == nil {
		return session, true, nil
	}
	if !isMissingSessionData(err) {
		return types.Session{}, false, err
	}
	return types.Session{}, false, nil
}

func (s *system) mergeSessionMessageSave(save types.SessionMessageSave, stored types.Session, storedExists bool, now time.Time) (types.Session, error) {
	session := save.Session
	merged := stored
	loaded := storedExists
	if !loaded {
		session.Messages = nil
		merged = normalizeSessionForWrite(session, now)
	}
	merged.Status = string(save.Status)
	if loaded {
		merged.Metadata = mergeSessionMetadataPreservingCurrent(merged.Metadata, session.Metadata)
	} else {
		merged.Metadata = mergeSessionMetadata(session.Metadata, merged.Metadata)
	}
	merged.Metadata = applySessionMetadataPatch(merged.Metadata, save.MetadataPatch)
	merged.AsyncToolTasks = types.MergeAsyncToolTasks(merged.AsyncToolTasks, session.AsyncToolTasks)
	// 会话级用量真值随会话一并持久化：以本次会话携带的用量为准（未拿到真值即为无），
	// 一次会话只留最新一条，绝不保留上次旧值。
	merged.TokenUsage = session.TokenUsage
	// 会话级缓存率同理：以本次会话携带的缓存率为准，新值整体覆盖，无真值即为无。
	merged.CacheRate = session.CacheRate
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

// listSessions 只读 scope 索引：索引不可信时按数据目录重建，索引条目悬空时过滤并修复。
// 列表路径不再逐个读取 data.json。
func (s *system) listSessions(ctx context.Context, scope sessionScope) ([]types.SessionSummary, error) {
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return nil, err
	}
	lock, err := s.scopeLock(scope)
	if err != nil {
		return nil, err
	}
	document, ok := s.readScopeIndex(ctx, scope, lock)
	if !ok {
		return s.rebuildScopeIndex(ctx, scope, lock, false)
	}
	scopeDir, err := s.sessionScopeDir(scope)
	if err != nil {
		return nil, err
	}
	ids, err := scopeSessionIDs(scopeDir)
	if err != nil {
		return nil, err
	}
	if !hasDanglingSummary(document.Sessions, ids) {
		return sortSessionSummaries(document.Sessions), nil
	}
	// 悬空条目意味着索引与目录事实已经分叉：走一次权威重建，
	// 而不是在无写者锁的情况下改写索引（那会覆盖并发写者的新条目）。
	s.sessionMetrics.indexRepairs.Add(1)
	log.Printf("data-storage-system: 会话索引存在悬空条目，按数据目录重建：scope=%s", scope.key())
	return s.rebuildScopeIndex(ctx, scope, lock, false)
}

func (s *system) DeleteSession(ctx context.Context, roleID string, sessionID string) error {
	return s.removeSession(ctx, roleSessionScope(roleID), sessionID)
}

func (s *system) DeleteGroupSession(ctx context.Context, groupID string, sessionID string) error {
	return s.removeSession(ctx, groupSessionScope(groupID), sessionID)
}

func (s *system) DeleteWorkspaceSession(ctx context.Context, workspaceID string, roleID string, sessionID string) error {
	return s.removeSession(ctx, workspaceSessionScope(workspaceID, roleID), sessionID)
}

// removeSession 是删除的唯一原语，与保存共用同一个 scope 写者锁，写序为：
// 索引 dirty=true → rename 目录到回收站 → 回收站元数据 → 从索引移除条目。
func (s *system) removeSession(ctx context.Context, scope sessionScope, sessionID string) error {
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return err
	}
	if _, err := cleanID(sessionID); err != nil {
		return err
	}
	dir, err := s.sessionDir(scope, sessionID)
	if err != nil {
		return err
	}
	indexFile, err := s.sessionIndexFile(scope)
	if err != nil {
		return err
	}
	lock, err := s.scopeLock(scope)
	if err != nil {
		return err
	}
	lock.writers.Lock()
	defer lock.writers.Unlock()

	document, err := s.readScopeIndexForWriteLocked(ctx, scope, lock)
	if err != nil {
		return err
	}
	relocation, err := s.prepareRecycleRelocation(ctx, types.StorageItemSession, recycleOriginalID(scope, sessionID), dir)
	if err != nil {
		return err
	}
	dirty := document
	dirty.SchemaVersion = sessionIndexSchemaVersion
	dirty.Sort = sessionIndexSort
	dirty.Dirty = true
	dirtyTemp, err := prepareJSON(ctx, indexFile, dirty)
	if err != nil {
		return s.failSessionWrite("failed to prepare session index dirty marker", err)
	}
	defer func() { _ = os.Remove(dirtyTemp) }()

	lock.mu.Lock()
	if err := commitJSON(dirtyTemp, indexFile); err != nil {
		lock.mu.Unlock()
		return s.failSessionWrite("failed to write session index dirty marker", err)
	}
	relocateErr := relocation.commit()
	lock.mu.Unlock()
	if relocateErr != nil {
		return relocateErr
	}

	if err := s.writeRecycleMetadata(ctx, relocation); err != nil {
		return s.rollbackSessionDelete(ctx, scope, relocation, document, lock, err)
	}

	next := newSessionIndexDocument(document.Rev+1, removeSessionSummary(document.Sessions, sessionID))
	nextTemp, err := prepareJSONDerived(ctx, indexFile, next)
	if err != nil {
		return s.failSessionWrite("failed to prepare session index", err)
	}
	defer func() { _ = os.Remove(nextTemp) }()
	lock.mu.Lock()
	err = commitJSON(nextTemp, indexFile)
	lock.mu.Unlock()
	if err != nil {
		return s.failSessionWrite("failed to write session index", err)
	}
	s.sessionMetrics.indexWrites.Add(1)
	return nil
}

// rollbackSessionDelete 在回收站元数据失败时把目录搬回原位并恢复索引。
func (s *system) rollbackSessionDelete(ctx context.Context, scope sessionScope, relocation recycleRelocation, document sessionIndexDocument, lock *scopeLock, cause error) error {
	restored := document
	restored.SchemaVersion = sessionIndexSchemaVersion
	restored.Sort = sessionIndexSort
	restored.Dirty = false
	indexFile, err := s.sessionIndexFile(scope)
	if err != nil {
		return cause
	}
	restoreTemp, prepareErr := prepareJSON(ctx, indexFile, restored)
	if prepareErr != nil {
		return cause
	}
	defer func() { _ = os.Remove(restoreTemp) }()
	lock.mu.Lock()
	defer lock.mu.Unlock()
	if err := relocation.rollback(); err != nil {
		return storageDeleteFailed("failed to rollback session delete", err)
	}
	if err := commitJSON(restoreTemp, indexFile); err != nil {
		s.sessionMetrics.indexFailures.Add(1)
		log.Printf("data-storage-system: 会话删除回滚后索引恢复失败（dirty 位会在下次读取时自愈）：scope=%s err=%v", scope.key(), err)
	}
	return cause
}

func recycleOriginalID(scope sessionScope, sessionID string) string {
	switch scope.Kind {
	case sessionScopeGroup:
		return "groups-" + scope.ID + "-" + sessionID
	case sessionScopeWorkspace:
		return "workspaces-" + scope.WorkspaceID + "-" + scope.RoleID + "-" + sessionID
	default:
		return sessionID
	}
}
