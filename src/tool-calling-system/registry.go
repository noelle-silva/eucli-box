package toolcalling

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

func (s *system) SaveTool(ctx context.Context, tool types.ToolDefinition) error {
	if err := validateToolDefinition(tool); err != nil {
		return err
	}
	tool.DefaultInvocationMode = types.CleanToolInvocationMode(tool.DefaultInvocationMode)
	now := time.Now().UTC()
	if tool.CreatedAt.IsZero() {
		tool.CreatedAt = now
	}
	tool.UpdatedAt = now
	if err := s.storage.SaveTool(ctx, tool); err != nil {
		return toolStorageFailed("failed to save tool", err)
	}
	return nil
}

func (s *system) LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error) {
	if strings.TrimSpace(toolID) == "" {
		return types.ToolDefinition{}, toolInvalid("tool id is required", nil)
	}
	tool, err := s.storage.LoadTool(ctx, toolID)
	if err != nil {
		return types.ToolDefinition{}, toolNotFound("failed to load tool", err)
	}
	return s.annotateTool(tool), nil
}

func (s *system) ListTools(ctx context.Context) ([]types.ToolSummary, error) {
	summaries, err := s.storage.ListTools(ctx)
	if err != nil {
		return nil, toolStorageFailed("failed to list tools", err)
	}
	tools := make([]types.ToolSummary, 0, len(summaries))
	for _, summary := range summaries {
		if summary.Status == types.ToolAvailabilityUnavailable {
			tools = append(tools, summary)
			continue
		}
		tool, err := s.storage.LoadTool(ctx, summary.ID)
		if err != nil {
			summary.Status = types.ToolAvailabilityUnavailable
			summary.StatusMessage = "工具本体资料不可读取：" + err.Error()
			tools = append(tools, summary)
			continue
		}
		tool = s.annotateTool(tool)
		tools = append(tools, types.ToolSummary{ID: tool.ID, Name: tool.Name, Description: tool.Description, Version: tool.Version, EucliBoxCompatibility: tool.EucliBoxCompatibility, Compatibility: tool.Compatibility, Status: tool.Status, StatusMessage: tool.StatusMessage, Type: tool.Type, UpdatedAt: tool.UpdatedAt})
	}
	return tools, nil
}

func (s *system) SaveToolUserSettings(ctx context.Context, toolID string, settings types.ToolUserSettings) (types.ToolDefinition, error) {
	if strings.TrimSpace(toolID) == "" {
		return types.ToolDefinition{}, toolInvalid("tool id is required", nil)
	}
	tool, err := s.storage.SaveToolUserSettings(ctx, toolID, settings)
	if err != nil {
		return types.ToolDefinition{}, toolStorageFailed("failed to save tool user settings", err)
	}
	return s.annotateTool(tool), nil
}

func (s *system) resolveTool(ctx context.Context, toolName string) (types.ToolDefinition, error) {
	summaries, err := s.ListTools(ctx)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	for _, summary := range summaries {
		if summary.ID == toolName || summary.Name == toolName {
			return s.LoadTool(ctx, summary.ID)
		}
	}
	return types.ToolDefinition{}, toolNotFound("tool definition was not found", nil)
}

func validateToolDefinition(tool types.ToolDefinition) error {
	if err := validateToolCore(tool); err != nil {
		return err
	}
	if err := release.ValidateVersion(tool.Version); err != nil {
		return toolInvalid("tool version is invalid: "+err.Error(), err)
	}
	if err := release.ValidateEucliBoxCompatibility(tool.EucliBoxCompatibility); err != nil {
		return toolInvalid("tool eucli-box compatibility is invalid: "+err.Error(), err)
	}
	return nil
}

func validateToolCore(tool types.ToolDefinition) error {
	if strings.TrimSpace(tool.ID) == "" {
		return toolInvalid("tool id is required", nil)
	}
	if strings.TrimSpace(tool.Name) == "" {
		return toolInvalid("tool name is required", nil)
	}
	if strings.TrimSpace(tool.Description) == "" {
		return toolInvalid("tool description is required", nil)
	}
	if !types.ValidExplicitToolInvocationMode(tool.DefaultInvocationMode) {
		return toolInvalid("tool default invocation mode must be sync or async", nil)
	}
	if tool.Type != "local" && tool.Type != "network" {
		return toolInvalid("tool type must be local or network", nil)
	}
	if strings.TrimSpace(tool.BodyDirectory) == "" {
		return toolInvalid("tool body directory is required", nil)
	}
	if len(tool.Binaries) == 0 {
		return toolInvalid("tool must declare at least one platform binary", nil)
	}
	return nil
}

func ensureToolBodyDirectory(tool types.ToolDefinition) error {
	if strings.TrimSpace(tool.BodyDirectory) == "" {
		return toolInvalid("tool body directory is required", nil)
	}
	info, err := os.Stat(tool.BodyDirectory)
	if err != nil {
		return toolNotFound("tool body directory does not exist", err)
	}
	if !info.IsDir() {
		return toolInvalid("tool body directory is not a directory", nil)
	}
	return nil
}

func cleanExecutablePath(tool types.ToolDefinition, executable string) (string, error) {
	toolDir, err := filepath.Abs(tool.BodyDirectory)
	if err != nil {
		return "", toolInvalid("failed to resolve tool body directory", err)
	}
	var resolved string
	if filepath.IsAbs(executable) {
		resolved = filepath.Clean(executable)
	} else {
		resolved = filepath.Join(toolDir, executable)
	}
	absResolved, err := filepath.Abs(resolved)
	if err != nil {
		return "", toolInvalid("failed to resolve tool executable", err)
	}
	if !pathWithin(toolDir, absResolved) {
		return "", toolInvalid("tool executable must stay inside tool body directory", nil)
	}
	return absResolved, nil
}

func (s *system) annotateTool(tool types.ToolDefinition) types.ToolDefinition {
	status := release.AssessEucliBoxCompatibility(tool.Version, s.boxVersion, tool.EucliBoxCompatibility)
	if err := validateToolCore(tool); err != nil {
		status = types.CompatibilityStatus{Reason: "工具本体资料无效：" + err.Error(), CurrentEucliBoxVersion: s.boxVersion, RequiredEucliBoxCompatibility: tool.EucliBoxCompatibility}
	}
	tool.Compatibility = status
	if status.Compatible {
		tool.Status = types.ToolAvailabilityActive
		tool.StatusMessage = ""
		return tool
	}
	tool.Status = types.ToolAvailabilityUnavailable
	tool.StatusMessage = status.Reason
	return tool
}

func pathWithin(base string, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
