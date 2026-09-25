package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"devtools/common/toolkit"
	"eucli-box/pkg/types"
	toolcalling "eucli-box/src/tool-calling-system"
)

// fakeProviderSource 提供四类命令：
//   - flood<size>：输出指定字节数的 ASCII 文本（默认 2MB），验证中间省略与字节事实；
//   - spam<count>：输出 count 条小短行（默认 12000 行），超过一万条更新上限；
//   - spawn-sleep：拉起一个子进程长时间睡眠（自举子进程），验证进程树终止出口；
//   - child-sleep：被自举的子进程，睡眠 30 秒。
const fakeProviderSource = `package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func main() {
	command := ""
	if len(os.Args) > 1 {
		command = os.Args[len(os.Args)-1]
	}
	switch {
	case command == "print":
		fmt.Fprintln(os.Stdout, "fixture-ok")
	case strings.HasPrefix(command, "flood"):
		size := 2 << 20
		if parts := strings.Fields(command); len(parts) == 2 {
			if v, err := strconv.Atoi(parts[1]); err == nil && v > 0 {
				size = v
			}
		}
		line := strings.Repeat("a", 256) + "\n"
		for written := 0; written < size; written += len(line) {
			fmt.Print(line)
		}
	case strings.HasPrefix(command, "spam"):
		count := 12000
		if parts := strings.Fields(command); len(parts) == 2 {
			if v, err := strconv.Atoi(parts[1]); err == nil && v > 0 {
				count = v
			}
		}
		for i := 0; i < count; i++ {
			fmt.Println("spam-line")
		}
	case command == "spawn-sleep":
		self := os.Args[0]
		cmd := exec.Command(self, "child-sleep")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_ = os.WriteFile("child.pid", []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)
		time.Sleep(30 * time.Second)
		_ = cmd.Wait()
	case command == "child-sleep":
		time.Sleep(30 * time.Second)
	default:
		fmt.Fprintln(os.Stdout, command)
	}
}
`

type fakePermissionSystem struct {
	decision types.PermissionDecision
}

func (f *fakePermissionSystem) Decide(ctx context.Context, roleID string, action types.ToolAction) (types.PermissionDecision, error) {
	if f.decision.ID != "" {
		return f.decision, nil
	}
	return types.PermissionDecision{ID: "d1", ActionID: action.ID, ToolName: action.ToolName, Status: types.PermissionStatusAllowed}, nil
}

func (f *fakePermissionSystem) ApplyConfirmation(ctx context.Context, decision types.PermissionDecision, confirmation types.ToolConfirmation) (types.PermissionDecision, error) {
	return decision, nil
}

type fakeToolStorage struct {
	tools      map[string]types.ToolDefinition
	workspaces map[string]types.Workspace
}

func (f *fakeToolStorage) SaveTool(ctx context.Context, tool types.ToolDefinition) error {
	if f.tools == nil {
		f.tools = map[string]types.ToolDefinition{}
	}
	f.tools[tool.ID] = tool
	return nil
}

func (f *fakeToolStorage) LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error) {
	if f.tools == nil {
		f.tools = map[string]types.ToolDefinition{}
	}
	tool, ok := f.tools[toolID]
	if !ok {
		return types.ToolDefinition{}, fmt.Errorf("tool %s not found", toolID)
	}
	return tool, nil
}

func (f *fakeToolStorage) ListTools(ctx context.Context) ([]types.ToolSummary, error) {
	return []types.ToolSummary{}, nil
}

func (f *fakeToolStorage) SaveToolUserSettings(ctx context.Context, toolID string, settings types.ToolUserSettings) (types.ToolDefinition, error) {
	return types.ToolDefinition{}, nil
}

func (f *fakeToolStorage) LoadWorkspace(ctx context.Context, workspaceID string) (types.Workspace, error) {
	return types.Workspace{ID: workspaceID}, nil
}

func (f *fakeToolStorage) LoadRole(ctx context.Context, roleID string) (types.Role, error) {
	return types.Role{}, fmt.Errorf("role %s not found", roleID)
}

func (f *fakeToolStorage) LoadChatGroup(ctx context.Context, groupID string) (types.ChatGroup, error) {
	return types.ChatGroup{}, fmt.Errorf("group %s not found", groupID)
}

func (f *fakeToolStorage) LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error) {
	return types.Session{}, fmt.Errorf("session %s not found", sessionID)
}

func (f *fakeToolStorage) LoadGroupSession(ctx context.Context, groupID string, sessionID string) (types.Session, error) {
	return types.Session{}, fmt.Errorf("session %s not found", sessionID)
}

func (f *fakeToolStorage) LoadWorkspaceSession(ctx context.Context, workspaceID string, roleID string, sessionID string) (types.Session, error) {
	return types.Session{}, fmt.Errorf("session %s not found", sessionID)
}

func (f *fakeToolStorage) SaveSessionMessageAttachment(ctx context.Context, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	return types.MessageAttachment{}, fmt.Errorf("attachment storage is not available")
}

func (f *fakeToolStorage) SaveGroupSessionMessageAttachment(ctx context.Context, groupID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	return types.MessageAttachment{}, fmt.Errorf("attachment storage is not available")
}

func (f *fakeToolStorage) SaveWorkspaceSessionMessageAttachment(ctx context.Context, workspaceID string, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	return types.MessageAttachment{}, fmt.Errorf("attachment storage is not available")
}

func (f *fakeToolStorage) LoadSessionAttachmentImage(ctx context.Context, relPath string) (string, error) {
	return "", fmt.Errorf("attachment %s not found", relPath)
}

func buildFixture(ctx context.Context, root string, run *toolkit.VerificationRun) (fixture, error) {
	toolDir := filepath.Join(run.Work, "tool")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		return fixture{}, err
	}
	ext := executableExtension()
	shellExe := filepath.Join(toolDir, "shell_command"+ext)
	if _, err := toolkit.RunCommandCapture(ctx, "编译 shell_command", run.Work, run.Evidence, run.Temp, nil, "go", "build", "-o", shellExe, "eucli-box/tools/shell_command/cmd/shell_command"); err != nil {
		return fixture{}, err
	}
	providerDir := filepath.Join(toolDir, "providers", "git-bash")
	if err := os.MkdirAll(providerDir, 0o755); err != nil {
		return fixture{}, err
	}
	providerExe := filepath.Join(providerDir, "fake-provider"+ext)
	sourceFile := filepath.Join(run.Work, "fake-provider.go")
	if err := os.WriteFile(sourceFile, []byte(fakeProviderSource), 0o644); err != nil {
		return fixture{}, err
	}
	if _, err := toolkit.RunCommandCapture(ctx, "编译假 provider", run.Work, run.Evidence, run.Temp, nil, "go", "build", "-o", providerExe, sourceFile); err != nil {
		return fixture{}, err
	}
	config := map[string]any{
		"defaultProvider":             "git-bash",
		"allowModelProviderSelection": true,
		"providers": []map[string]any{{
			"id":       "git-bash",
			"kind":     "git-bash",
			"mode":     "bundled",
			"enabled":  true,
			"priority": 10,
			"encoding": "utf-8",
			"executables": []types.ToolBinary{{
				GOOS:   runtime.GOOS,
				GOARCH: runtime.GOARCH,
				Path:   filepath.ToSlash(filepath.Join("providers", "git-bash", "fake-provider"+ext)),
			}},
		}},
		"limits": map[string]any{
			"maxOutputChars": 200000,
		},
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return fixture{}, err
	}
	if err := os.WriteFile(filepath.Join(toolDir, "config.json"), payload, 0o644); err != nil {
		return fixture{}, err
	}
	return fixture{root: toolDir, shellExe: shellExe}, nil
}

func newHostSystem(config toolcalling.Config, storage *fakeToolStorage) (toolcalling.System, error) {
	return toolcalling.NewSystem(config, &fakePermissionSystem{}, storage)
}

func executeHost(ctx context.Context, system toolcalling.System, fixture fixture, args map[string]any) (types.ToolResult, error) {
	definition := types.ToolDefinition{
		ID:                    "shell_command",
		Name:                  "shell_command",
		Description:           "fixture",
		Version:               "0.1.0",
		DefaultInvocationMode: types.ToolInvocationModeSync,
		Type:                  "local",
		BodyDirectory:         fixture.root,
		Binaries:              []types.ToolBinary{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Path: filepath.Base(fixture.shellExe)}},
	}
	plan := types.ToolRunPlan{
		ID:             "plan-verify",
		Action:         types.ToolAction{ID: "act-verify", ToolName: "shell_command", Arguments: args},
		Tool:           definition,
		InvocationMode: types.ToolInvocationModeSync,
		Decision:       types.PermissionDecision{ID: "d1", ActionID: "act-verify", ToolName: "shell_command", Status: types.PermissionStatusAllowed},
		PlanStatus:     types.ToolPlanStatusReady,
		Executable:     fixture.shellExe,
	}
	return system.Execute(ctx, plan)
}

func captureGitStatus(ctx context.Context, root string, run *toolkit.VerificationRun) (string, error) {
	output, err := toolkit.RunCommandCapture(ctx, "记录源码工作区状态", run.Work, run.Evidence, run.Temp, nil, "git", "-C", root, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	return output, nil
}

func metaBool(metadata map[string]any, key string) bool {
	value, ok := metadata[key]
	if !ok {
		return false
	}
	if boolean, ok := value.(bool); ok {
		return boolean
	}
	return false
}

func metaString(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return fmt.Sprintf("%v", value)
	}
	return text
}

func metaFloat(metadata map[string]any, key string) float64 {
	value, ok := metadata[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}
