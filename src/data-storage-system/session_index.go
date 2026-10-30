package datastorage

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

const (
	// sessionIndexSchemaVersion 是会话索引文档的结构版本。
	// 旧索引（缺字段）一律判为无效并按数据目录重建，开发阶段不做迁移。
	sessionIndexSchemaVersion = 2
	// sessionIndexSort 是索引与列表共用的排序契约：
	// lastActive 降序，同刻按 id 升序，保证结果与目录枚举顺序无关。
	sessionIndexSort = "lastActiveDescIdAsc"
)

// sessionIndexDocument 是单个 scope 的派生索引：
// data.json 是唯一权威，本文件只承载列表投影，可随时按数据目录重建。
// Dirty 是崩溃意图位：写序为 dirty=true → 写数据 → dirty=false，
// 崩在任意位置都能在下次读取时被识别并自愈，无需额外 pending 文件。
type sessionIndexDocument struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Dirty         bool                   `json:"dirty"`
	Rev           uint64                 `json:"rev"`
	Sort          string                 `json:"sort"`
	Sessions      []types.SessionSummary `json:"sessions"`
}

func newSessionIndexDocument(rev uint64, sessions []types.SessionSummary) sessionIndexDocument {
	return sessionIndexDocument{
		SchemaVersion: sessionIndexSchemaVersion,
		Dirty:         false,
		Rev:           rev,
		Sort:          sessionIndexSort,
		Sessions:      sortSessionSummaries(sessions),
	}
}

func (s *system) scopeIndexValid(document sessionIndexDocument, scope sessionScope) bool {
	if document.SchemaVersion != sessionIndexSchemaVersion || document.Dirty || document.Sort != sessionIndexSort {
		return false
	}
	source := scope.fingerprint()
	for _, summary := range document.Sessions {
		if strings.TrimSpace(summary.ID) == "" || summary.Source != source {
			return false
		}
	}
	return true
}

// sessionSummaryFromStored 是纯函数：只依赖已落盘的会话事实，
// 不再用 time.Now 补时间戳，因此同一份数据永远投影出同一份摘要。
func sessionSummaryFromStored(session types.Session, scope sessionScope) types.SessionSummary {
	return types.SessionSummary{
		ID:                 session.ID,
		RoleID:             session.RoleID,
		GroupID:            session.GroupID,
		WorkspaceID:        session.WorkspaceID,
		Title:              session.Title,
		LastMessagePreview: sessionLastMessagePreview(session.Messages),
		Status:             session.Status,
		CreatedAt:          session.CreatedAt,
		UpdatedAt:          session.UpdatedAt,
		LastActive:         session.LastActive,
		MessageCount:       len(session.Messages),
		Source:             scope.fingerprint(),
	}
}

func sortSessionSummaries(summaries []types.SessionSummary) []types.SessionSummary {
	sort.Slice(summaries, func(i, j int) bool {
		if !summaries[i].LastActive.Equal(summaries[j].LastActive) {
			return summaries[i].LastActive.After(summaries[j].LastActive)
		}
		return summaries[i].ID < summaries[j].ID
	})
	return summaries
}

func upsertSessionSummary(summaries []types.SessionSummary, summary types.SessionSummary) []types.SessionSummary {
	for index := range summaries {
		if summaries[index].ID == summary.ID {
			summaries[index] = summary
			return summaries
		}
	}
	return append(summaries, summary)
}

func removeSessionSummary(summaries []types.SessionSummary, sessionID string) []types.SessionSummary {
	next := make([]types.SessionSummary, 0, len(summaries))
	for _, summary := range summaries {
		if summary.ID == sessionID {
			continue
		}
		next = append(next, summary)
	}
	return next
}

// readScopeIndex 只做「读字节 → 锁外解码」：
// 索引缺失、损坏、结构版本不符、脏位未清、来源指纹不符，一律判为无效，
// 由调用方按数据目录重建。
func (s *system) readScopeIndex(ctx context.Context, scope sessionScope, lock *scopeLock) (sessionIndexDocument, bool) {
	indexFile, err := s.sessionIndexFile(scope)
	if err != nil {
		return sessionIndexDocument{}, false
	}
	lock.mu.RLock()
	payload, readErr := readFileBytes(ctx, indexFile)
	lock.mu.RUnlock()
	if readErr != nil {
		return sessionIndexDocument{}, false
	}
	document, decodeErr := decodeJSON[sessionIndexDocument](payload)
	if decodeErr != nil || !s.scopeIndexValid(document, scope) {
		return sessionIndexDocument{}, false
	}
	s.sessionMetrics.indexReads.Add(1)
	s.sessionMetrics.indexBytes.Add(uint64(len(payload)))
	return document, true
}

// readScopeIndexForWriteLocked 在写者锁内取得可信索引；
// 索引不可信时先按数据目录重建，绝不把「只含本次会话」的半份索引写回。
func (s *system) readScopeIndexForWriteLocked(ctx context.Context, scope sessionScope, lock *scopeLock) (sessionIndexDocument, error) {
	if document, ok := s.readScopeIndex(ctx, scope, lock); ok {
		return document, nil
	}
	scopeDir, err := s.sessionScopeDir(scope)
	if err != nil {
		return sessionIndexDocument{}, err
	}
	ids, err := scopeSessionIDs(scopeDir)
	if err != nil {
		return sessionIndexDocument{}, err
	}
	// 全新 scope 没有既有会话：空索引就是可信事实，无需扫描，也不算重建。
	if len(ids) == 0 {
		return newSessionIndexDocument(0, nil), nil
	}
	summaries, err := s.rebuildScopeIndexLocked(ctx, scope, lock, true)
	if err != nil {
		return sessionIndexDocument{}, err
	}
	// 重建已经把新索引（含递增后的 Rev）写回，这里直接按同一事实组装，避免再次读盘。
	rev := uint64(1)
	if current, ok := s.readScopeIndex(ctx, scope, lock); ok {
		rev = current.Rev
	}
	return newSessionIndexDocument(rev, summaries), nil
}

// writeScopeIndexLocked 把编码与 fsync 放在锁外，只在 rename 期间持写锁。
func (s *system) writeScopeIndexLocked(ctx context.Context, scope sessionScope, lock *scopeLock, document sessionIndexDocument) error {
	indexFile, err := s.sessionIndexFile(scope)
	if err != nil {
		return err
	}
	temp, err := prepareJSONDerived(ctx, indexFile, document)
	if err != nil {
		return storageIndexFailed("failed to prepare session index", err)
	}
	defer func() { _ = os.Remove(temp) }()
	lock.mu.Lock()
	err = commitJSON(temp, indexFile)
	lock.mu.Unlock()
	if err != nil {
		s.sessionMetrics.indexFailures.Add(1)
		return storageIndexFailed("failed to write session index", err)
	}
	s.sessionMetrics.indexWrites.Add(1)
	return nil
}

// scanScopeSummaries 是索引重建的唯一数据源：逐个读取本 scope 的 data.json。
func (s *system) scanScopeSummaries(ctx context.Context, scope sessionScope) ([]types.SessionSummary, error) {
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
		session = normalizeSessionStored(session)
		summaries = append(summaries, sessionSummaryFromStored(session, scope))
	}
	return sortSessionSummaries(summaries), nil
}

// rebuildScopeIndexLocked 假定调用方已持有 scope 写者锁。
func (s *system) rebuildScopeIndexLocked(ctx context.Context, scope sessionScope, lock *scopeLock, strict bool) ([]types.SessionSummary, error) {
	summaries, err := s.scanScopeSummaries(ctx, scope)
	if err != nil {
		return nil, err
	}
	scopeDir, err := s.sessionScopeDir(scope)
	if err != nil {
		return nil, err
	}
	// 读取一个从未存在过的 scope 时不制造空目录与空索引：列表结果为空即事实。
	if len(summaries) == 0 && !directoryExists(scopeDir) {
		return summaries, nil
	}
	rev := uint64(0)
	if current, ok := s.readScopeIndex(ctx, scope, lock); ok {
		rev = current.Rev
	}
	s.sessionMetrics.indexRebuilds.Add(1)
	document := newSessionIndexDocument(rev+1, summaries)
	if err := s.writeScopeIndexLocked(ctx, scope, lock, document); err != nil {
		if strict {
			return nil, err
		}
		s.sessionMetrics.indexFailures.Add(1)
		log.Printf("data-storage-system: 会话索引自愈写入失败（读取继续，下次读取会重试）：scope=%s err=%v", scope.key(), err)
	}
	return summaries, nil
}

// rebuildScopeIndex 取得写者锁后重建单个 scope 索引。
func (s *system) rebuildScopeIndex(ctx context.Context, scope sessionScope, lock *scopeLock, strict bool) ([]types.SessionSummary, error) {
	lock.writers.Lock()
	defer lock.writers.Unlock()
	return s.rebuildScopeIndexLocked(ctx, scope, lock, strict)
}

func scopeSessionIDs(scopeDir string) (map[string]struct{}, error) {
	entries, err := os.ReadDir(scopeDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]struct{}{}, nil
		}
		return nil, storageReadFailed("failed to scan session scope directory", err)
	}
	ids := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ids[entry.Name()] = struct{}{}
	}
	return ids, nil
}

func hasDanglingSummary(summaries []types.SessionSummary, ids map[string]struct{}) bool {
	for _, summary := range summaries {
		if _, ok := ids[summary.ID]; !ok {
			return true
		}
	}
	return false
}

// ensureScopeCatalog 保证 scope 目录存在，并在新 scope 出现时增量登记目录树。
// 目录已存在时零写入、零 catalog 锁，跨 scope 写因此保持并行。
func (s *system) ensureScopeCatalog(ctx context.Context, scope sessionScope) error {
	scopeDir, err := s.sessionScopeDir(scope)
	if err != nil {
		return err
	}
	if directoryExists(scopeDir) {
		return nil
	}
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if directoryExists(scopeDir) {
		return nil
	}
	if err := ensureDirs(scopeDir); err != nil {
		return storageWriteFailed("failed to create session scope directory", err)
	}
	switch scope.Kind {
	case sessionScopeGroup:
		return s.upsertCatalogFolder(ctx, s.paths.sessionGroupsRoot(), scope.ID)
	case sessionScopeWorkspace:
		workspaceDir, err := s.workspaceDir(scope.WorkspaceID)
		if err != nil {
			return err
		}
		if err := s.upsertWorkspaceRoleFolder(ctx, workspaceDir, scope.RoleID); err != nil {
			return err
		}
		return s.upsertCatalogFolder(ctx, s.paths.sessionWorkspacesRoot(), scope.WorkspaceID)
	default:
		return s.upsertCatalogFolder(ctx, s.paths.sessionRolesRoot(), scope.ID)
	}
}

func (s *system) upsertCatalogFolder(ctx context.Context, root string, folderID string) error {
	indexFile := filepath.Join(root, "index.json")
	ids, err := readCatalogFolderIDs(ctx, root, indexFile)
	if err != nil {
		return err
	}
	ids = upsertFolderID(ids, folderID)
	if err := writeIndex(ctx, indexFile, sessionRoleIndex{Folders: folderIDs(ids)}); err != nil {
		return err
	}
	s.sessionMetrics.catalogUpdates.Add(1)
	return nil
}

func (s *system) upsertWorkspaceRoleFolder(ctx context.Context, workspaceDir string, roleID string) error {
	indexFile := filepath.Join(workspaceDir, "index.json")
	ids, err := readWorkspaceRoleIDs(ctx, workspaceDir, indexFile)
	if err != nil {
		return err
	}
	ids = upsertFolderID(ids, roleID)
	if err := writeIndex(ctx, indexFile, workspaceRoleSessionIndex{Roles: folderIDs(ids)}); err != nil {
		return err
	}
	s.sessionMetrics.catalogUpdates.Add(1)
	return nil
}

func readCatalogFolderIDs(ctx context.Context, root string, indexFile string) ([]string, error) {
	if document, err := readJSON[sessionRoleIndex](ctx, indexFile); err == nil {
		return folderIDsFromFolders(document.Folders), nil
	}
	return scanDirectoryIDs(root)
}

func readWorkspaceRoleIDs(ctx context.Context, workspaceDir string, indexFile string) ([]string, error) {
	if document, err := readJSON[workspaceRoleSessionIndex](ctx, indexFile); err == nil {
		return folderIDsFromFolders(document.Roles), nil
	}
	return scanDirectoryIDs(workspaceDir)
}

func scanDirectoryIDs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, storageIndexFailed("failed to scan index directory", err)
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ids = append(ids, entry.Name())
	}
	sort.Strings(ids)
	return ids, nil
}

func folderIDsFromFolders(folders []roleFolder) []string {
	ids := make([]string, 0, len(folders))
	for _, folder := range folders {
		if id := strings.TrimSpace(folder.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func folderIDs(ids []string) []roleFolder {
	folders := make([]roleFolder, 0, len(ids))
	for _, id := range ids {
		folders = append(folders, roleFolder{ID: id})
	}
	return folders
}

func upsertFolderID(ids []string, folderID string) []string {
	folderID = strings.TrimSpace(folderID)
	for _, id := range ids {
		if id == folderID {
			sort.Strings(ids)
			return ids
		}
	}
	ids = append(ids, folderID)
	sort.Strings(ids)
	return ids
}

// rebuildSessionIndexes 是全量重建入口（启动维护与显式修复）：
// 逐 scope 重建索引，再重写目录树根索引。
func (s *system) rebuildSessionIndexes(ctx context.Context) error {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if err := s.rebuildRoleSessionIndexes(ctx); err != nil {
		return err
	}
	if err := s.rebuildGroupSessionIndexes(ctx); err != nil {
		return err
	}
	return s.rebuildWorkspaceSessionIndexes(ctx)
}

func (s *system) rebuildRoleSessionIndexes(ctx context.Context) error {
	ids, err := scanDirectoryIDs(s.paths.sessionRolesRoot())
	if err != nil {
		return err
	}
	for _, roleID := range ids {
		scope, err := cleanSessionScope(roleSessionScope(roleID))
		if err != nil {
			return err
		}
		lock, err := s.scopeLock(scope)
		if err != nil {
			return err
		}
		if _, err := s.rebuildScopeIndex(ctx, scope, lock, true); err != nil {
			return err
		}
	}
	return writeIndex(ctx, filepath.Join(s.paths.sessionRolesRoot(), "index.json"), sessionRoleIndex{Folders: folderIDs(ids)})
}

func (s *system) rebuildGroupSessionIndexes(ctx context.Context) error {
	ids, err := scanDirectoryIDs(s.paths.sessionGroupsRoot())
	if err != nil {
		return err
	}
	for _, groupID := range ids {
		scope, err := cleanSessionScope(groupSessionScope(groupID))
		if err != nil {
			return err
		}
		lock, err := s.scopeLock(scope)
		if err != nil {
			return err
		}
		if _, err := s.rebuildScopeIndex(ctx, scope, lock, true); err != nil {
			return err
		}
	}
	return writeIndex(ctx, filepath.Join(s.paths.sessionGroupsRoot(), "index.json"), sessionRoleIndex{Folders: folderIDs(ids)})
}

func (s *system) rebuildWorkspaceSessionIndexes(ctx context.Context) error {
	workspaceIDs, err := scanDirectoryIDs(s.paths.sessionWorkspacesRoot())
	if err != nil {
		return err
	}
	for _, workspaceID := range workspaceIDs {
		workspaceDir, err := s.workspaceDir(workspaceID)
		if err != nil {
			return err
		}
		roleIDs, err := scanDirectoryIDs(workspaceDir)
		if err != nil {
			return err
		}
		for _, roleID := range roleIDs {
			scope, err := cleanSessionScope(workspaceSessionScope(workspaceID, roleID))
			if err != nil {
				return err
			}
			lock, err := s.scopeLock(scope)
			if err != nil {
				return err
			}
			if _, err := s.rebuildScopeIndex(ctx, scope, lock, true); err != nil {
				return err
			}
		}
		if err := writeIndex(ctx, filepath.Join(workspaceDir, "index.json"), workspaceRoleSessionIndex{Roles: folderIDs(roleIDs)}); err != nil {
			return err
		}
	}
	return writeIndex(ctx, filepath.Join(s.paths.sessionWorkspacesRoot(), "index.json"), sessionRoleIndex{Folders: folderIDs(workspaceIDs)})
}
