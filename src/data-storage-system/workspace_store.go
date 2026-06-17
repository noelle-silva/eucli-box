package datastorage

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/workspaceprompt"
)

func (s *system) SaveWorkspace(ctx context.Context, workspace types.Workspace) error {
	workspace, err := normalizeWorkspaceForStorage(workspace, time.Now().UTC())
	if err != nil {
		return err
	}
	target, err := s.paths.workspaceDataFile(workspace.ID)
	if err != nil {
		return err
	}
	if err := writeJSON(ctx, target, workspace); err != nil {
		return err
	}
	return s.rebuildWorkspaceIndex(ctx)
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
	return normalizeWorkspaceForStorage(workspace, time.Now().UTC())
}

func (s *system) ListWorkspaces(ctx context.Context) ([]types.WorkspaceSummary, error) {
	workspaces, err := readObjects[types.Workspace](ctx, s.paths.workspacesRoot())
	if err != nil {
		return nil, err
	}
	summaries := make([]types.WorkspaceSummary, 0, len(workspaces))
	for _, workspace := range workspaces {
		workspace, err = normalizeWorkspaceForStorage(workspace, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, types.WorkspaceSummary{ID: workspace.ID, Name: workspace.Name, UpdatedAt: workspace.UpdatedAt})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })
	return summaries, nil
}

func (s *system) PreviewWorkspacePrompt(ctx context.Context, workspace types.Workspace) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	workspace, err := normalizeWorkspaceForStorage(workspace, time.Now().UTC())
	if err != nil {
		return "", err
	}
	return workspaceprompt.Content(workspace), nil
}

func (s *system) DeleteWorkspace(ctx context.Context, workspaceID string) error {
	dir, err := s.paths.workspaceDir(workspaceID)
	if err != nil {
		return err
	}
	if err := moveToRecycle(ctx, s.paths, types.StorageItemWorkspace, workspaceID, dir); err != nil {
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

func normalizeWorkspaceForStorage(workspace types.Workspace, now time.Time) (types.Workspace, error) {
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
	baseline := firstNonZeroTime(workspace.CreatedAt, workspace.UpdatedAt, now)
	if workspace.CreatedAt.IsZero() {
		workspace.CreatedAt = baseline
	}
	if workspace.UpdatedAt.IsZero() || workspace.UpdatedAt.Before(workspace.CreatedAt) {
		workspace.UpdatedAt = baseline
	}
	return workspace, nil
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
	info, err := os.Stat(absPath)
	if err != nil {
		return types.WorkspaceDirectory{}, storageInvalid("workspace directory path must exist", err)
	}
	if !info.IsDir() {
		return types.WorkspaceDirectory{}, storageInvalid("workspace directory path must be a directory", nil)
	}
	alias := strings.Join(strings.Fields(directory.Alias), " ")
	if alias == "" {
		alias = filepath.Base(absPath)
	}
	if alias == "." || alias == string(filepath.Separator) {
		alias = "目录"
	}
	return types.WorkspaceDirectory{Path: absPath, Alias: alias, Description: strings.TrimSpace(directory.Description)}, nil
}
