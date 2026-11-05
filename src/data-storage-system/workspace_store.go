package datastorage

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/workspaceprompt"
)

func (s *system) SaveWorkspace(ctx context.Context, workspace types.Workspace) error {
	now := time.Now().UTC()
	workspace, err := s.normalizeWorkspaceForStorage(workspace, now)
	if err != nil {
		return err
	}
	target, err := s.paths.workspaceDataFile(workspace.ID)
	if err != nil {
		return err
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	isNew := !dataFileExists(target)
	orderedIDs, summaries, err := s.workspaceCollectionForSave(ctx, workspace, isNew, now)
	if err != nil {
		return err
	}
	tx := newFileTransaction()
	defer tx.Discard()
	if err := tx.StageJSON(ctx, target, workspace); err != nil {
		return err
	}
	if err := tx.StageJSON(ctx, s.paths.workspaceOrderFile(), orderFile{IDs: orderedIDs}); err != nil {
		return err
	}
	indexTarget := filepath.Join(s.paths.workspacesRoot(), "index.json")
	if err := tx.StageJSON(ctx, indexTarget, rootIndex[types.WorkspaceSummary]{Items: summaries}); err != nil {
		return err
	}
	return tx.Commit()
}

// workspaceCollectionForSave 在内存中推导保存后的集合顺序与索引摘要：
// 实体数据、顺序事实与派生索引必须来自同一份待提交快照，
// 不能等实体落盘后再各自回读，否则失败时会留下半吊子记录。
func (s *system) workspaceCollectionForSave(ctx context.Context, workspace types.Workspace, isNew bool, now time.Time) ([]string, []types.WorkspaceSummary, error) {
	workspaces, err := readObjects[types.Workspace](ctx, s.paths.workspacesRoot())
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[string]types.WorkspaceSummary, len(workspaces)+1)
	for _, existing := range workspaces {
		normalized, err := s.normalizeWorkspaceForStorage(existing, now)
		if err != nil {
			return nil, nil, err
		}
		byID[normalized.ID] = workspaceSummary(normalized)
	}
	byID[workspace.ID] = workspaceSummary(workspace)

	existingIDs, err := listObjectIDs(ctx, s.paths.workspacesRoot())
	if err != nil {
		return nil, nil, err
	}
	if isNew {
		existingIDs = append(existingIDs, workspace.ID)
	}
	order, err := readOrderIDs(ctx, s.paths.workspaceOrderFile())
	if err != nil {
		return nil, nil, err
	}
	if isNew {
		order = append([]string{workspace.ID}, order...)
	}
	orderedIDs := mergeOrderIDs(order, existingIDs)
	summaries := make([]types.WorkspaceSummary, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if summary, ok := byID[id]; ok {
			summaries = append(summaries, summary)
		}
	}
	return orderedIDs, summaries, nil
}

func workspaceSummary(workspace types.Workspace) types.WorkspaceSummary {
	return types.WorkspaceSummary{ID: workspace.ID, Name: workspace.Name, UpdatedAt: workspace.UpdatedAt}
}

func (s *system) LoadWorkspace(ctx context.Context, workspaceID string) (types.Workspace, error) {
	target, err := s.paths.workspaceDataFile(workspaceID)
	if err != nil {
		return types.Workspace{}, err
	}
	workspace, err := readJSON[types.Workspace](ctx, target)
	if err != nil {
		return types.Workspace{}, err
	}
	return s.normalizeWorkspaceForStorage(workspace, time.Now().UTC())
}

func (s *system) ListWorkspaces(ctx context.Context) ([]types.WorkspaceSummary, error) {
	workspaces, err := readObjects[types.Workspace](ctx, s.paths.workspacesRoot())
	if err != nil {
		return nil, err
	}
	byID := make(map[string]types.WorkspaceSummary, len(workspaces))
	for _, workspace := range workspaces {
		workspace, err = s.normalizeWorkspaceForStorage(workspace, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		byID[workspace.ID] = workspaceSummary(workspace)
	}
	ids, err := orderedObjectIDs(ctx, s.paths.workspacesRoot(), s.paths.workspaceOrderFile())
	if err != nil {
		return nil, err
	}
	summaries := make([]types.WorkspaceSummary, 0, len(ids))
	for _, id := range ids {
		if summary, ok := byID[id]; ok {
			summaries = append(summaries, summary)
		}
	}
	return summaries, nil
}

func (s *system) PreviewWorkspacePrompt(ctx context.Context, workspace types.Workspace) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	workspace, err := s.normalizeWorkspaceForStorage(workspace, time.Now().UTC())
	if err != nil {
		return "", err
	}
	return workspaceprompt.Content(workspace), nil
}

func (s *system) ReorderWorkspaces(ctx context.Context, workspaceIDs []string) error {
	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	if err := s.reorderObjects(ctx, s.paths.workspacesRoot(), s.paths.workspaceOrderFile(), workspaceIDs); err != nil {
		return err
	}
	return s.rebuildWorkspaceIndex(ctx)
}

func (s *system) DeleteWorkspace(ctx context.Context, workspaceID string) error {
	dir, err := s.paths.workspaceDir(workspaceID)
	if err != nil {
		return err
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	if err := s.moveToRecycle(ctx, types.StorageItemWorkspace, workspaceID, dir); err != nil {
		return err
	}
	if err := s.syncOrderAfterDelete(ctx, s.paths.workspacesRoot(), s.paths.workspaceOrderFile()); err != nil {
		return err
	}
	return s.rebuildWorkspaceIndex(ctx)
}

func (s *system) rebuildWorkspaceIndex(ctx context.Context) error {
	workspaces, err := s.ListWorkspaces(ctx)
	if err != nil {
		return err
	}
	return writeIndex(ctx, filepath.Join(s.paths.workspacesRoot(), "index.json"), rootIndex[types.WorkspaceSummary]{Items: workspaces})
}

func (s *system) normalizeWorkspaceForStorage(workspace types.Workspace, now time.Time) (types.Workspace, error) {
	workspace.ID = strings.TrimSpace(workspace.ID)
	if _, err := cleanID(workspace.ID); err != nil {
		return types.Workspace{}, err
	}
	workspace.Name = strings.Join(strings.Fields(workspace.Name), " ")
	if workspace.Name == "" {
		workspace.Name = "未命名工作区"
	}
	directories, err := normalizeWorkspaceDirectories(workspace.Directories)
	if err != nil {
		return types.Workspace{}, err
	}
	workspace.Directories = directories
	workspace.Prompt = strings.TrimSpace(workspace.Prompt)
	workspace.RegisteredRoleIDs = s.normalizeWorkspaceRegisteredRoleIDs(workspace.RegisteredRoleIDs)
	baseline := firstNonZeroTime(workspace.CreatedAt, workspace.UpdatedAt, now)
	if workspace.CreatedAt.IsZero() {
		workspace.CreatedAt = baseline
	}
	if workspace.UpdatedAt.IsZero() || workspace.UpdatedAt.Before(workspace.CreatedAt) {
		workspace.UpdatedAt = baseline
	}
	return workspace, nil
}

// normalizeWorkspaceRegisteredRoleIDs 只保留真实存在的角色并去重：
// 工作区清单里不保留已不存在的角色，同一角色只出现一次。
func (s *system) normalizeWorkspaceRegisteredRoleIDs(roleIDs []string) []string {
	result := make([]string, 0, len(roleIDs))
	seen := make(map[string]struct{}, len(roleIDs))
	for _, rawRoleID := range roleIDs {
		roleID := strings.TrimSpace(rawRoleID)
		if roleID == "" {
			continue
		}
		if _, ok := seen[roleID]; ok {
			continue
		}
		target, err := s.paths.roleDataFile(roleID)
		if err != nil || !dataFileExists(target) {
			continue
		}
		seen[roleID] = struct{}{}
		result = append(result, roleID)
	}
	return result
}

func normalizeWorkspaceDirectories(directories []types.WorkspaceDirectory) ([]types.WorkspaceDirectory, error) {
	result := make([]types.WorkspaceDirectory, 0, len(directories))
	seenPath := map[string]struct{}{}
	seenAlias := map[string]struct{}{}
	for _, directory := range directories {
		normalized, err := normalizeWorkspaceDirectory(directory)
		if err != nil {
			return nil, err
		}
		if _, ok := seenPath[normalized.Path]; ok {
			continue
		}
		aliasKey := strings.ToLower(normalized.Alias)
		if _, ok := seenAlias[aliasKey]; ok {
			return nil, storageInvalid("workspace directory alias is duplicated", nil)
		}
		seenPath[normalized.Path] = struct{}{}
		seenAlias[aliasKey] = struct{}{}
		result = append(result, normalized)
	}
	return result, nil
}

func normalizeWorkspaceDirectory(directory types.WorkspaceDirectory) (types.WorkspaceDirectory, error) {
	rawPath := strings.TrimSpace(directory.Path)
	if rawPath == "" {
		return types.WorkspaceDirectory{}, storageInvalid("workspace directory path is required", nil)
	}
	if !filepath.IsAbs(rawPath) {
		return types.WorkspaceDirectory{}, storageInvalid("workspace directory path must be absolute", nil)
	}
	absPath, err := filepath.Abs(rawPath)
	if err != nil {
		return types.WorkspaceDirectory{}, storageInvalid("failed to resolve workspace directory path", err)
	}
	absPath = filepath.Clean(absPath)
	// 这里只校验登记事实（路径字符串/绝对路径/别名），不探测磁盘：
	// 目录移动、改名或拔盘属于运行时健康，由 workspacehealth 单独报告。
	alias := strings.Join(strings.Fields(directory.Alias), " ")
	if alias == "" {
		alias = filepath.Base(absPath)
	}
	if alias == "." || alias == string(filepath.Separator) {
		alias = "目录"
	}
	return types.WorkspaceDirectory{Path: absPath, Alias: alias, Description: strings.TrimSpace(directory.Description)}, nil
}
