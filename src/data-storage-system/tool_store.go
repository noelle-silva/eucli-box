package datastorage

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) SaveTool(ctx context.Context, tool types.ToolDefinition) error {
	if _, err := cleanID(tool.ID); err != nil {
		return err
	}
	target, err := s.paths.toolDataFile(tool.ID)
	if err != nil {
		return err
	}
	if err := writeJSON(ctx, target, tool); err != nil {
		return err
	}
	toolDir, err := s.paths.toolDir(tool.ID)
	if err != nil {
		return err
	}
	if err := ensureDirs(filepath.Join(toolDir, "binary")); err != nil {
		return storageWriteFailed("failed to create tool binary directory", err)
	}
	return s.rebuildToolIndex(ctx)
}

func (s *system) LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error) {
	target, err := s.paths.toolDataFile(toolID)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	tool, err := readJSON[types.ToolDefinition](ctx, target)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	directory, err := s.resolveToolDirectory(tool.ID, tool.Directory)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	tool.Directory = directory
	return tool, nil
}

func (s *system) ListTools(ctx context.Context) ([]types.ToolSummary, error) {
	tools, err := readObjects[types.ToolDefinition](ctx, s.paths.toolsRoot())
	if err != nil {
		return nil, err
	}
	summaries := make([]types.ToolSummary, 0, len(tools))
	for _, tool := range tools {
		summaries = append(summaries, types.ToolSummary{ID: tool.ID, Name: tool.Name, Description: tool.Description, Type: tool.Type, UpdatedAt: tool.UpdatedAt})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })
	return summaries, nil
}

func (s *system) DeleteTool(ctx context.Context, toolID string) error {
	dir, err := s.paths.toolDir(toolID)
	if err != nil {
		return err
	}
	if err := moveToRecycle(ctx, s.paths, types.StorageItemTool, toolID, dir); err != nil {
		return err
	}
	return s.rebuildToolIndex(ctx)
}

func (s *system) resolveToolDirectory(toolID string, directory string) (string, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return "", nil
	}
	if filepath.IsAbs(directory) {
		return filepath.Clean(directory), nil
	}
	if filepath.VolumeName(directory) != "" {
		return "", storageInvalid("relative tool directory cannot include volume name", nil)
	}
	toolDir, err := s.paths.toolDir(toolID)
	if err != nil {
		return "", err
	}
	resolved := filepath.Clean(filepath.Join(toolDir, directory))
	if !isWithin(toolDir, resolved) {
		return "", storageInvalid("relative tool directory escapes tool package", nil)
	}
	return resolved, nil
}
