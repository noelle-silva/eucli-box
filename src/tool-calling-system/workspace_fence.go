package toolcalling

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

type workspaceFencePathCandidate struct {
	Argument string
	RawPath  string
}

type workspaceFenceDirectory struct {
	Directory types.WorkspaceDirectory
	Absolute  string
}

// evaluateWorkspaceFence 只对声明了工作区能力的工具评估路径围栏；
// 路径候选统一来自工具自身对路径类参数的声明，不存在按工具名开的小灶。
func (s *system) evaluateWorkspaceFence(ctx context.Context, scope types.ToolRunScope, tool types.ToolDefinition, action types.ToolAction) (*types.ToolWorkspaceFence, error) {
	workspaceID := strings.TrimSpace(scope.WorkspaceID)
	if workspaceID == "" {
		return nil, nil
	}
	if !types.ToolDeclaresCapability(tool, types.ToolCapabilityWorkspace, types.ToolCapabilityAccessRead) {
		return nil, nil
	}
	candidates, err := schemaWorkspacePathCandidates(tool, action)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	workspace, err := s.storage.LoadWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, toolStorageFailed("failed to load workspace", err)
	}
	directories, err := normalizeWorkspaceFenceDirectories(workspace.Directories)
	if err != nil {
		return nil, err
	}
	hostWorkingDirectory, err := os.Getwd()
	if err != nil {
		return nil, toolInvalid("failed to resolve host working directory", err)
	}
	fence := &types.ToolWorkspaceFence{WorkspaceID: workspace.ID, RegisteredDirectories: workspace.Directories, Paths: make([]types.ToolWorkspaceFencePath, 0, len(candidates))}
	for _, candidate := range candidates {
		path, err := evaluateWorkspaceFencePath(hostWorkingDirectory, directories, candidate)
		if err != nil {
			return nil, err
		}
		if !path.WithinWorkspace {
			fence.RequiresConfirmation = true
		}
		fence.Paths = append(fence.Paths, path)
	}
	return fence, nil
}

// schemaWorkspacePathCandidates 从工具调用参数中提取所有被声明为
// 文件路径（format=filepath）的路径候选；这是路径识别的唯一来源。
func schemaWorkspacePathCandidates(tool types.ToolDefinition, action types.ToolAction) ([]workspaceFencePathCandidate, error) {
	argumentNames := filepathSchemaArgumentNames(tool.InputSchema)
	candidates := make([]workspaceFencePathCandidate, 0, len(argumentNames))
	for _, argumentName := range argumentNames {
		value, ok, err := effectiveStringWorkspaceArgument(tool, action, argumentName, false)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if strings.TrimSpace(value) == "" {
			value = "."
		}
		candidates = append(candidates, workspaceFencePathCandidate{Argument: argumentName, RawPath: value})
	}
	return candidates, nil
}

func filepathSchemaArgumentNames(schema map[string]any) []string {
	properties, ok := objectMap(schema["properties"])
	if !ok {
		return nil
	}
	names := []string{}
	for name, value := range properties {
		property, ok := objectMap(value)
		if !ok {
			continue
		}
		format, _ := property["format"].(string)
		if format == "filepath" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func objectMap(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	return typed, ok
}

func effectiveStringWorkspaceArgument(tool types.ToolDefinition, action types.ToolAction, key string, required bool) (string, bool, error) {
	if value, ok := action.Arguments[key]; ok && value != nil {
		text, err := workspaceArgumentStringValue(value, key, required)
		return text, true, err
	}
	if value, ok := tool.UserConfig[key]; ok && value != nil {
		text, err := workspaceArgumentStringValue(value, key, required)
		return text, true, err
	}
	if value, ok := tool.DefaultConfig[key]; ok && value != nil {
		text, err := workspaceArgumentStringValue(value, key, required)
		return text, true, err
	}
	if required {
		return "", false, toolInvalid("workspace filepath argument is required", nil)
	}
	return "", false, nil
}

func workspaceArgumentStringValue(value any, key string, required bool) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", toolInvalid("workspace filepath argument must be a string", nil)
	}
	if required && strings.TrimSpace(text) == "" {
		return "", toolInvalid("workspace filepath argument is required", nil)
	}
	return text, nil
}

func normalizeWorkspaceFenceDirectories(directories []types.WorkspaceDirectory) ([]workspaceFenceDirectory, error) {
	normalized := make([]workspaceFenceDirectory, 0, len(directories))
	for _, directory := range directories {
		rawPath := strings.TrimSpace(directory.Path)
		if rawPath == "" {
			return nil, toolInvalid("workspace directory path is required", nil)
		}
		absolute, err := filepath.Abs(rawPath)
		if err != nil {
			return nil, toolInvalid("failed to resolve workspace directory path", err)
		}
		absolute, err = filepath.EvalSymlinks(filepath.Clean(absolute))
		if err != nil {
			return nil, toolInvalid("failed to resolve workspace directory real path", err)
		}
		normalized = append(normalized, workspaceFenceDirectory{Directory: directory, Absolute: filepath.Clean(absolute)})
	}
	return normalized, nil
}

func evaluateWorkspaceFencePath(hostWorkingDirectory string, directories []workspaceFenceDirectory, candidate workspaceFencePathCandidate) (types.ToolWorkspaceFencePath, error) {
	absolute, err := resolveWorkspaceFencePath(hostWorkingDirectory, candidate.RawPath)
	if err != nil {
		return types.ToolWorkspaceFencePath{}, err
	}
	path := types.ToolWorkspaceFencePath{Argument: candidate.Argument, RawPath: candidate.RawPath, AbsolutePath: absolute}
	for _, directory := range directories {
		if pathWithin(directory.Absolute, absolute) {
			path.WithinWorkspace = true
			path.MatchedDirectoryAlias = directory.Directory.Alias
			return path, nil
		}
	}
	path.Reason = workspaceFenceOutsideReason(directories)
	return path, nil
}

func resolveWorkspaceFencePath(hostWorkingDirectory string, rawPath string) (string, error) {
	requested := strings.TrimSpace(rawPath)
	if requested == "" {
		requested = "."
	}
	if strings.ContainsRune(requested, '\x00') {
		return "", toolInvalid("workspace filepath cannot contain null bytes", nil)
	}
	base, err := filepath.Abs(strings.TrimSpace(hostWorkingDirectory))
	if err != nil {
		return "", toolInvalid("failed to resolve host working directory", err)
	}
	resolved := requested
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(base, resolved)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return "", toolInvalid("failed to resolve workspace filepath", err)
	}
	canonical, err := canonicalWorkspaceFencePath(filepath.Clean(absolute))
	if err != nil {
		return "", toolInvalid("failed to resolve workspace filepath real path", err)
	}
	return filepath.Clean(canonical), nil
}

func canonicalWorkspaceFencePath(path string) (string, error) {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Abs(real)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent, err := nearestExistingWorkspaceFenceParent(path)
	if err != nil {
		return "", err
	}
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(parent, path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(realParent, relative))
}

func nearestExistingWorkspaceFenceParent(path string) (string, error) {
	current := filepath.Clean(path)
	for {
		info, err := os.Stat(current)
		if err == nil {
			if info.IsDir() {
				return current, nil
			}
			return filepath.Dir(current), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing parent found for %s", path)
		}
		current = parent
	}
}

func workspaceFenceOutsideReason(directories []workspaceFenceDirectory) string {
	if len(directories) == 0 {
		return "workspace has no registered directories"
	}
	return "path is outside workspace registered directories"
}

func workspaceFenceDecision(action types.ToolAction, fence *types.ToolWorkspaceFence) types.PermissionDecision {
	return types.PermissionDecision{
		ID:        utils.NewID("permission"),
		ActionID:  action.ID,
		ToolName:  action.ToolName,
		Status:    types.PermissionStatusNeedsConfirmation,
		Reason:    "tool path is outside workspace registered directories",
		Details:   map[string]any{"workspaceFence": fence},
		CreatedAt: time.Now().UTC(),
	}
}
