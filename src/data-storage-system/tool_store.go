package datastorage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) SaveTool(ctx context.Context, tool types.ToolDefinition) error {
	if _, err := cleanID(tool.ID); err != nil {
		return err
	}
	tool.BodyDirectory = "."
	tool.DataDirectory = ""
	tool.UserConfig = nil
	tool.PromptDescriptionOverride = ""
	tool.Compatibility = types.CompatibilityStatus{}
	tool.Status = ""
	tool.StatusMessage = ""
	target, err := s.paths.toolBodyDefinitionFile(tool.ID)
	if err != nil {
		return err
	}
	if err := writeJSON(ctx, target, tool); err != nil {
		return err
	}
	return s.rebuildToolIndex(ctx)
}

func (s *system) LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error) {
	toolID, err := cleanID(toolID)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	tool, err := s.loadToolDefinition(ctx, toolID)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	bodyDirectory, err := s.resolveToolBodyDirectory(tool.ID, tool.BodyDirectory)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	dataDirectory, err := s.paths.toolDataDir(tool.ID)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	settings, err := s.loadToolUserSettings(ctx, tool.ID)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	tool.BodyDirectory = bodyDirectory
	tool.DataDirectory = dataDirectory
	tool.UserConfig = copyToolMap(settings.UserConfig)
	tool.PromptDescriptionOverride = settings.PromptDescriptionOverride
	if settings.UpdatedAt.After(tool.UpdatedAt) {
		tool.UpdatedAt = settings.UpdatedAt
	}
	return tool, nil
}

func (s *system) ListTools(ctx context.Context) ([]types.ToolSummary, error) {
	entries, err := os.ReadDir(s.paths.toolBodiesRoot())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, storageReadFailed("failed to scan tool body directory", err)
	}
	summaries := make([]types.ToolSummary, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, storageReadFailed("scan cancelled", err)
		}
		if !entry.IsDir() {
			continue
		}
		toolID := entry.Name()
		tool, err := s.loadToolDefinition(ctx, toolID)
		if err != nil {
			summaries = append(summaries, unavailableToolSummary(toolID, err))
			continue
		}
		settings, settingsErr := s.loadToolUserSettings(ctx, tool.ID)
		if settingsErr != nil {
			summaries = append(summaries, unavailableToolSummary(tool.ID, settingsErr))
			continue
		}
		updatedAt := tool.UpdatedAt
		if settings.UpdatedAt.After(updatedAt) {
			updatedAt = settings.UpdatedAt
		}
		summaries = append(summaries, types.ToolSummary{ID: tool.ID, Name: tool.Name, Description: tool.Description, Version: tool.Version, EucliBoxCompatibility: tool.EucliBoxCompatibility, Type: tool.Type, UpdatedAt: updatedAt})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })
	return summaries, nil
}

func (s *system) SaveToolUserSettings(ctx context.Context, toolID string, settings types.ToolUserSettings) (types.ToolDefinition, error) {
	id, err := cleanID(toolID)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	if _, err := s.loadToolDefinition(ctx, id); err != nil {
		return types.ToolDefinition{}, err
	}
	if settings.UserConfig == nil {
		settings.UserConfig = map[string]any{}
	}
	settings.UserConfig = copyToolMap(settings.UserConfig)
	settings.PromptDescriptionOverride = strings.TrimSpace(settings.PromptDescriptionOverride)
	settings.UpdatedAt = time.Now().UTC()
	target, err := s.paths.toolUserSettingsFile(id)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	if err := writeJSON(ctx, target, settings); err != nil {
		return types.ToolDefinition{}, err
	}
	if err := s.rebuildToolIndex(ctx); err != nil {
		return types.ToolDefinition{}, err
	}
	return s.LoadTool(ctx, id)
}

func (s *system) DeleteTool(ctx context.Context, toolID string) error {
	id, err := cleanID(toolID)
	if err != nil {
		return err
	}
	bodyDir, err := s.paths.toolBodyDir(id)
	if err != nil {
		return err
	}
	dataDir, err := s.paths.toolDataDir(id)
	if err != nil {
		return err
	}
	if err := moveToRecycle(ctx, s.paths, types.StorageItemTool, id, bodyDir); err != nil {
		return err
	}
	if dataInfo, err := os.Stat(dataDir); err == nil && dataInfo.IsDir() {
		if err := os.RemoveAll(dataDir); err != nil {
			return storageWriteFailed("failed to remove tool user data", err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return storageReadFailed("failed to inspect tool user data", err)
	}
	return s.rebuildToolIndex(ctx)
}

func (s *system) loadToolDefinition(ctx context.Context, toolID string) (types.ToolDefinition, error) {
	target, err := s.paths.toolBodyDefinitionFile(toolID)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	tool, err := readJSON[types.ToolDefinition](ctx, target)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	if strings.TrimSpace(tool.ID) != toolID {
		return types.ToolDefinition{}, storageInvalid("tool id does not match body directory", nil)
	}
	return tool, nil
}

func (s *system) loadToolUserSettings(ctx context.Context, toolID string) (types.ToolUserSettings, error) {
	target, err := s.paths.toolUserSettingsFile(toolID)
	if err != nil {
		return types.ToolUserSettings{}, err
	}
	settings, err := readJSON[types.ToolUserSettings](ctx, target)
	if errors.Is(err, os.ErrNotExist) {
		return types.ToolUserSettings{UserConfig: map[string]any{}}, nil
	}
	if err != nil {
		return types.ToolUserSettings{}, err
	}
	settings.UserConfig = copyToolMap(settings.UserConfig)
	settings.PromptDescriptionOverride = strings.TrimSpace(settings.PromptDescriptionOverride)
	return settings, nil
}

func (s *system) resolveToolBodyDirectory(toolID string, directory string) (string, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return "", storageInvalid("tool body directory is required", nil)
	}
	if filepath.IsAbs(directory) || filepath.VolumeName(directory) != "" {
		return "", storageInvalid("tool body directory must be relative", nil)
	}
	bodyDir, err := s.paths.toolBodyDir(toolID)
	if err != nil {
		return "", err
	}
	resolved := filepath.Clean(filepath.Join(bodyDir, directory))
	if !isWithin(bodyDir, resolved) {
		return "", storageInvalid("relative tool body directory escapes tool body", nil)
	}
	return resolved, nil
}

func unavailableToolSummary(toolID string, cause error) types.ToolSummary {
	return types.ToolSummary{ID: toolID, Name: toolID, Description: "工具本体资料不可用", Status: types.ToolAvailabilityUnavailable, StatusMessage: cause.Error()}
}

func copyToolMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
