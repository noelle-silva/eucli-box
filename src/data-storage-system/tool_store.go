package datastorage

import (
	"context"
	"sort"

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
	return s.rebuildToolIndex(ctx)
}

func (s *system) LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error) {
	target, err := s.paths.toolDataFile(toolID)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	return readJSON[types.ToolDefinition](ctx, target)
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
