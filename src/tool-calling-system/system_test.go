package toolcalling

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

func TestNormalizeIntentCreatesStandardAction(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	action, err := system.NormalizeIntent(context.Background(), types.ToolIntent{ToolName: " file-reader ", Arguments: map[string]any{" path ": "README.md"}})
	if err != nil {
		t.Fatalf("NormalizeIntent() error = %v", err)
	}
	if action.ToolName != "file-reader" || action.ID == "" || action.Arguments["path"] != "README.md" {
		t.Fatalf("action = %#v", action)
	}
}

func TestSaveToolUserSettingsUpdatesConfigAndPromptOverride(t *testing.T) {
	storage := newFakeToolStorage()
	system := newTestToolSystem(t, &fakePermission{}, storage, Config{})
	tool := types.ToolDefinition{ID: "shell_command", Name: "shell_command", Description: "Run shell command", DefaultInvocationMode: types.ToolInvocationModeSync, Type: "local", DefaultConfig: map[string]any{"provider": "git-bash"}}
	if err := storage.SaveTool(context.Background(), tool); err != nil {
		t.Fatalf("SaveTool() error = %v", err)
	}

	updated, err := system.SaveToolUserSettings(context.Background(), tool.ID, types.ToolUserSettings{UserConfig: map[string]any{"timeoutMs": float64(2000)}, PromptDescriptionOverride: "Run one safe command"})
	if err != nil {
		t.Fatalf("SaveToolUserSettings() error = %v", err)
	}
	if updated.UserConfig["timeoutMs"] != float64(2000) || updated.PromptDescriptionOverride != "Run one safe command" || updated.DefaultConfig["provider"] != "git-bash" || updated.Description != tool.Description {
		t.Fatalf("updated tool = %#v", updated)
	}
}

func TestPrepareReturnsDeniedPlanWhenPermissionDenies(t *testing.T) {
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	system := newTestToolSystem(t, &fakePermission{decision: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusDenied, Reason: "blocked"}}, storage, Config{})
	plan, err := system.Prepare(context.Background(), types.ToolRunScope{RoleID: "developer"}, types.ToolAction{ID: "a1", ToolName: tool.Name})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if plan.PlanStatus != types.ToolPlanStatusDenied || plan.Decision.Reason != "blocked" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestPrepareRejectsMissingToolDefaultInvocationMode(t *testing.T) {
	tool := testTool(t, buildTool(t, `package main
func main() {}
`))
	tool.DefaultInvocationMode = ""
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	system := newTestToolSystem(t, &fakePermission{decision: types.PermissionDecision{Status: types.PermissionStatusAllowed}}, storage, Config{})

	_, err := system.Prepare(context.Background(), types.ToolRunScope{RoleID: "developer"}, types.ToolAction{ID: "a1", ToolName: tool.Name})
	assertAppErrorCode(t, err, "tool.invalid_request")
}

func TestPrepareRejectsInvalidActionInvocationMode(t *testing.T) {
	storage := newFakeToolStorage()
	system := newTestToolSystem(t, &fakePermission{}, storage, Config{})

	_, err := system.Prepare(context.Background(), types.ToolRunScope{RoleID: "developer"}, types.ToolAction{ID: "a1", ToolName: "file-reader", InvocationMode: "later"})
	assertAppErrorCode(t, err, "tool.invalid_request")
}

func TestApplyConfirmationUpdatesPlan(t *testing.T) {
	storage := newFakeToolStorage()
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	storage.tools[tool.ID] = tool
	permissions := &fakePermission{confirmation: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: "web-search", Status: types.PermissionStatusAllowed, Reason: "approved"}}
	system := newTestToolSystem(t, permissions, storage, Config{})
	plan := types.ToolRunPlan{ID: "plan-1", Action: types.ToolAction{ID: "a1", ToolName: tool.Name}, Tool: tool, Decision: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusNeedsConfirmation}, PlanStatus: types.ToolPlanStatusNeedsConfirmation}
	updated, err := system.ApplyConfirmation(context.Background(), plan, types.ToolConfirmation{DecisionID: "d1", Approved: true})
	if err != nil {
		t.Fatalf("ApplyConfirmation() error = %v", err)
	}
	if updated.Decision.Status != types.PermissionStatusAllowed {
		t.Fatalf("updated = %#v", updated)
	}
}

func TestExecuteRunsToolAndParsesResult(t *testing.T) {
	executable := buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{"source":"test"}}`+"`"+`) }
`)
	tool := testTool(t, executable)
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	result, err := system.Execute(context.Background(), allowedPlan(tool, executable))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != types.ToolStatusSuccess || result.Content != "ok" || result.Metadata["source"] != "test" {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteRecordsRealExecutionDuration(t *testing.T) {
	executable := buildTool(t, `package main
import (
  "fmt"
  "time"
)
func main() {
  time.Sleep(120 * time.Millisecond)
  fmt.Print(`+"`"+`{"status":"success","content":"ok"}`+"`"+`)
}
`)
	tool := testTool(t, executable)
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	result, err := system.Execute(context.Background(), allowedPlan(tool, executable))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if result.DurationMs < 120 || result.DurationMs > 30_000 {
		t.Fatalf("DurationMs = %d, want within [120, 30000]", result.DurationMs)
	}
}

func TestExecutePassesSharedToolExecutionInput(t *testing.T) {
	executable := buildTool(t, `package main
import (
  "encoding/json"
  "os"
)
type input struct {
  DefaultConfig map[string]any `+"`json:\"defaultConfig\"`"+`
  HostWorkingDirectory string `+"`json:\"hostWorkingDirectory\"`"+`
}
func main() {
  var in input
  if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil { panic(err) }
  json.NewEncoder(os.Stdout).Encode(map[string]any{"status":"success","content":"ok","metadata":map[string]any{"hostWorkingDirectory":in.HostWorkingDirectory,"defaultMode":in.DefaultConfig["mode"]}})
}
`)
	tool := testTool(t, executable)
	tool.DefaultConfig = map[string]any{"mode": "test"}
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	result, err := system.Execute(context.Background(), allowedPlan(tool, executable))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Metadata["hostWorkingDirectory"] == "" || result.Metadata["defaultMode"] != "test" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecutePreservesFailedToolMetadata(t *testing.T) {
	executable := buildTool(t, `package main
import (
  "encoding/json"
  "os"
)
func main() {
  json.NewEncoder(os.Stdout).Encode(map[string]any{"status":"failed","content":"bad output","error":"bad exit","metadata":map[string]any{"stdout":"x","exitCode":7}})
}
`)
	tool := testTool(t, executable)
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	result, err := system.Execute(context.Background(), allowedPlan(tool, executable))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != types.ToolStatusFailed || result.Error != "bad exit" || result.Content != "bad output" || result.Metadata["stdout"] != "x" {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteNormalizesDamagedOutput(t *testing.T) {
	executable := buildTool(t, `package main
import "fmt"
func main() { fmt.Print("not-json") }
`)
	tool := testTool(t, executable)
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	result, err := system.Execute(context.Background(), allowedPlan(tool, executable))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != types.ToolStatusFailed || result.Error == "" {
		t.Fatalf("result = %#v", result)
	}
}

// TestExecuteFailsWhenToolIgnoresControlProtocol 验证标配控制通道：
// 任何不实现控制协议的二进制都按协议失败归类，不存在旧总时限兜底。
func TestExecuteFailsWhenToolIgnoresControlProtocol(t *testing.T) {
	executable := buildRawTool(t, `package main
import "time"
func main() { time.Sleep(2 * time.Second) }
`)
	tool := testTool(t, executable)
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{ToolWatchdogTimeout: 300 * time.Millisecond, ToolWatchdogPingInterval: 100 * time.Millisecond})
	result, err := system.Execute(context.Background(), allowedPlan(tool, executable))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != types.ToolStatusFailed || result.Metadata["failureKind"] != "tool_protocol_failed" {
		t.Fatalf("result = %#v", result)
	}
}

// TestExecuteReportsToolFailureBeforeControlHandshake 验证：工具在控制通道握手
// 完成前失败退出时，业务端透传工具自己输出的真实错误，而不是只报协议失败。
func TestExecuteReportsToolFailureBeforeControlHandshake(t *testing.T) {
	executable := buildRawTool(t, `package main
import (
  "encoding/json"
  "os"
)
func main() {
  json.NewEncoder(os.Stdout).Encode(map[string]any{"status":"failed","content":"command analysis failed: component missing","error":"command analysis failed: component missing","metadata":map[string]any{"error":"command analysis failed: component missing"}})
}
`)
	tool := testTool(t, executable)
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	result, err := system.Execute(context.Background(), allowedPlan(tool, executable))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != types.ToolStatusFailed || result.Metadata["failureKind"] != "tool_protocol_failed" {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Error, "command analysis failed: component missing") || !strings.Contains(result.Content, "command analysis failed: component missing") {
		t.Fatalf("protocol failure must carry the tool-reported cause, result = %#v", result)
	}
}

func TestPrepareFailsWhenPlatformBinaryMissing(t *testing.T) {
	tool := testTool(t, buildTool(t, `package main
func main() {}
`))
	tool.Binaries = []types.ToolBinary{{GOOS: "nope", GOARCH: "nope", Path: tool.Binaries[0].Path}}
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	system := newTestToolSystem(t, &fakePermission{decision: types.PermissionDecision{Status: types.PermissionStatusAllowed}}, storage, Config{})
	_, err := system.Prepare(context.Background(), types.ToolRunScope{RoleID: "developer"}, types.ToolAction{ID: "a1", ToolName: tool.Name})
	assertAppErrorCode(t, err, "tool.not_found")
}

// TestWorkspaceFenceAllowsRelativePathFromWorkspaceBase 验证围栏与工具一致：
// 相对路径以工作区首个注册目录为基准解析，宿主目录与之不同也不误判越界。
func TestWorkspaceFenceAllowsRelativePathFromWorkspaceBase(t *testing.T) {
	hostDir := t.TempDir()
	workspaceDir := t.TempDir()
	t.Chdir(hostDir)
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", Name: "Workspace", Directories: []types.WorkspaceDirectory{{Path: workspaceDir, Alias: "work"}}}
	system := newTestToolSystem(t, &fakePermission{decision: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusAllowed}}, storage, Config{})

	plan, err := system.Prepare(context.Background(), types.ToolRunScope{RoleID: "developer", WorkspaceID: "workspace-1"}, types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"action": "read", "path": "inside.txt"}})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if plan.PlanStatus != types.ToolPlanStatusReady || plan.WorkspaceFence == nil || plan.WorkspaceFence.RequiresConfirmation {
		t.Fatalf("plan = %#v", plan)
	}
	if len(plan.WorkspaceFence.Paths) != 1 || !plan.WorkspaceFence.Paths[0].WithinWorkspace || plan.WorkspaceFence.Paths[0].MatchedDirectoryAlias != "work" {
		t.Fatalf("fence paths = %#v", plan.WorkspaceFence.Paths)
	}
}

// TestWorkspaceFenceRequiresConfirmationForRelativeEscape 验证相对路径
// 从工作区基准向上逃逸时仍然进入确认流程。
func TestWorkspaceFenceRequiresConfirmationForRelativeEscape(t *testing.T) {
	hostDir := t.TempDir()
	workspaceDir := t.TempDir()
	t.Chdir(hostDir)
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", Name: "Workspace", Directories: []types.WorkspaceDirectory{{Path: workspaceDir, Alias: "work"}}}
	system := newTestToolSystem(t, &fakePermission{decision: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusAllowed}}, storage, Config{})

	plan, err := system.Prepare(context.Background(), types.ToolRunScope{RoleID: "developer", WorkspaceID: "workspace-1"}, types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"action": "read", "path": filepath.Join("..", "outside.txt")}})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if plan.PlanStatus != types.ToolPlanStatusNeedsConfirmation || plan.WorkspaceFence == nil || !plan.WorkspaceFence.RequiresConfirmation {
		t.Fatalf("plan = %#v", plan)
	}
}

// TestStopToolExecutionStopsRunningTool 验证 035：面向用户的停止动作取消具体
// 执行实例，该执行按用户取消归类，工具活动计数归零。
func TestStopToolExecutionStopsRunningTool(t *testing.T) {
	executable := buildTool(t, `package main
import "time"
func main() { time.Sleep(30 * time.Second) }
`)
	tool := testTool(t, executable)
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	done := make(chan types.ToolResult, 1)
	go func() {
		result, _ := system.Execute(context.Background(), allowedPlan(tool, executable))
		done <- result
	}()
	time.Sleep(800 * time.Millisecond)
	activity, err := system.ToolActivity(context.Background(), tool.ID)
	if err != nil || !activity.Active {
		t.Fatalf("activity before stop = %#v, err = %v", activity, err)
	}
	stopResult, err := system.StopToolExecution(context.Background(), tool.ID)
	if err != nil {
		t.Fatalf("StopToolExecution() error = %v", err)
	}
	if stopResult.Terminated != 1 {
		t.Fatalf("stop result = %#v", stopResult)
	}
	select {
	case result := <-done:
		if result.Status != types.ToolStatusCancelled || result.Metadata["failureKind"] != "user_cancelled" {
			t.Fatalf("stopped result = %#v", result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("stopped tool did not finish")
	}
	activity, err = system.ToolActivity(context.Background(), tool.ID)
	if err != nil || activity.Active {
		t.Fatalf("activity after stop = %#v, err = %v", activity, err)
	}
	stopResult, err = system.StopToolExecution(context.Background(), tool.ID)
	if err != nil || stopResult.Terminated != 0 {
		t.Fatalf("stop idle result = %#v, err = %v", stopResult, err)
	}
}

func TestWorkspaceFenceRequiresConfirmationForOutsidePath(t *testing.T) {
	hostDir := t.TempDir()
	outsideDir := t.TempDir()
	t.Chdir(hostDir)
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", Name: "Workspace", Directories: []types.WorkspaceDirectory{{Path: hostDir, Alias: "host"}}}
	system := newTestToolSystem(t, &fakePermission{decision: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusAllowed}}, storage, Config{})

	plan, err := system.Prepare(context.Background(), types.ToolRunScope{RoleID: "developer", WorkspaceID: "workspace-1"}, types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"action": "read", "path": filepath.Join(outsideDir, "outside.txt")}})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if plan.PlanStatus != types.ToolPlanStatusNeedsConfirmation || plan.WorkspaceFence == nil || !plan.WorkspaceFence.RequiresConfirmation {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.Decision.Status != types.PermissionStatusNeedsConfirmation || plan.Decision.Details["workspaceFence"] == nil {
		t.Fatalf("decision = %#v", plan.Decision)
	}
}

func TestWorkspaceFenceApprovalContinuesToRoleConfirmation(t *testing.T) {
	hostDir := t.TempDir()
	outsideDir := t.TempDir()
	t.Chdir(hostDir)
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", Name: "Workspace", Directories: []types.WorkspaceDirectory{{Path: hostDir, Alias: "host"}}}
	permissions := &fakePermission{decision: types.PermissionDecision{ID: "role-decision", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusNeedsConfirmation}}
	system := newTestToolSystem(t, permissions, storage, Config{})

	plan, err := system.Prepare(context.Background(), types.ToolRunScope{RoleID: "developer", WorkspaceID: "workspace-1"}, types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"action": "read", "path": filepath.Join(outsideDir, "outside.txt")}})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	updated, err := system.ApplyConfirmation(context.Background(), plan, types.ToolConfirmation{DecisionID: plan.Decision.ID, Approved: true})
	if err != nil {
		t.Fatalf("ApplyConfirmation() error = %v", err)
	}
	if updated.PlanStatus != types.ToolPlanStatusNeedsConfirmation || updated.Decision.ID != "role-decision" {
		t.Fatalf("updated = %#v", updated)
	}
	if updated.WorkspaceFence == nil || updated.WorkspaceFence.RequiresConfirmation {
		t.Fatalf("workspace fence = %#v", updated.WorkspaceFence)
	}
}

func TestExecuteReturnsDeniedResult(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	result, err := system.Execute(context.Background(), types.ToolRunPlan{Action: types.ToolAction{ID: "a1", ToolName: "file-reader"}, Tool: types.ToolDefinition{ID: "file-reader"}, PlanStatus: types.ToolPlanStatusDenied, Decision: types.PermissionDecision{Status: types.PermissionStatusDenied, Reason: "blocked"}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != types.ToolStatusDenied || result.Error != "blocked" {
		t.Fatalf("result = %#v", result)
	}
}

func TestApplyConfirmationRejectsUserRefusal(t *testing.T) {
	storage := newFakeToolStorage()
	tool := testTool(t, buildTool(t, `package main
func main() {}
`))
	storage.tools[tool.ID] = tool
	system := newTestToolSystem(t, &fakePermission{}, storage, Config{})
	plan := types.ToolRunPlan{ID: "plan-1", Action: types.ToolAction{ID: "a1", ToolName: tool.Name}, Tool: tool, Decision: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusNeedsConfirmation}, PlanStatus: types.ToolPlanStatusNeedsConfirmation}
	updated, err := system.ApplyConfirmation(context.Background(), plan, types.ToolConfirmation{DecisionID: "d1", Approved: false})
	if err != nil {
		t.Fatalf("ApplyConfirmation() error = %v", err)
	}
	if updated.PlanStatus != types.ToolPlanStatusDenied {
		t.Fatalf("plan status = %s", updated.PlanStatus)
	}
}

func newTestToolSystem(t *testing.T, permission PermissionSystem, storage StorageSystem, config Config) System {
	t.Helper()
	system, err := NewSystem(config, permission, storage)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	return system
}

func testTool(t *testing.T, executable string) types.ToolDefinition {
	t.Helper()
	dir := filepath.Dir(executable)
	return types.ToolDefinition{
		ID:                    "file-reader",
		Name:                  "file-reader",
		Description:           "Read files",
		Version:               "0.1.0",
		EucliBoxCompatibility: types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		DefaultInvocationMode: types.ToolInvocationModeSync,
		Type:                  "local",
		Capabilities:          []types.ToolCapability{{ID: types.ToolCapabilityWorkspace, Access: types.ToolCapabilityAccessRead, Name: "工作区路径"}},
		InputSchema:           map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string", "format": "filepath"}}},
		BodyDirectory:         dir,
		DataDirectory:         filepath.Join(dir, "data"),
		Binaries:              []types.ToolBinary{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Path: filepath.Base(executable)}},
		UserConfig:            map[string]any{"limit": 10},
	}
}

func allowedPlan(tool types.ToolDefinition, executable string) types.ToolRunPlan {
	return types.ToolRunPlan{
		Action:     types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"path": "README.md"}},
		Tool:       tool,
		Decision:   types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusAllowed},
		PlanStatus: types.ToolPlanStatusReady,
		Executable: executable,
	}
}

// buildTool compiles a controlled helper tool: the user source becomes run(),
// wrapped by the standard tool runtime (control channel + heartbeat) so all
// helper tools behave like real platform tools.
func buildTool(t *testing.T, source string) string {
	t.Helper()
	return buildHelperTool(t, source, true)
}

// buildRawTool compiles a bare helper tool without control support; such a tool
// is exactly what the platform classifies as a protocol failure.
func buildRawTool(t *testing.T, source string) string {
	t.Helper()
	return buildHelperTool(t, source, false)
}

// buildHelperTool compiles the helper in a self-contained module so the build
// never depends on the current working directory or the repository go.work.
func buildHelperTool(t *testing.T, source string, controlled bool) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(helperModuleGoMod(t)), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if controlled {
		body := strings.Replace(source, "func main() {", "func run() {", 1)
		mainSource := `package main

import (
	"context"
	"time"

	"eucli-box/pkg/toolcontrol"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := toolcontrol.AdoptControl(ctx)
	if err == nil && client != nil {
		defer client.Close()
		go func() { _ = client.Serve(ctx) }()
	}
	run()
}
`
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSource), 0o644); err != nil {
			t.Fatalf("WriteFile(main) error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "body.go"), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile(body) error = %v", err)
		}
	} else {
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644); err != nil {
			t.Fatalf("WriteFile(main) error = %v", err)
		}
	}
	exe := filepath.Join(dir, "tool")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build helper failed: %v\n%s", err, output)
	}
	return exe
}

// helperModuleGoMod renders a self-contained module pointing at the repository.
func helperModuleGoMod(t *testing.T) string {
	t.Helper()
	_, callerFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(callerFile)))
	return fmt.Sprintf("module ebbchelper\n\ngo 1.23\n\nrequire eucli-box v0.0.0\n\nreplace eucli-box => %s\n", filepath.ToSlash(repoRoot))
}

type fakePermission struct {
	decision     types.PermissionDecision
	confirmation types.PermissionDecision
	err          error
}

func (f *fakePermission) Decide(ctx context.Context, roleID string, action types.ToolAction) (types.PermissionDecision, error) {
	if f.err != nil {
		return types.PermissionDecision{}, f.err
	}
	if f.decision.ID == "" {
		return types.PermissionDecision{ID: "d1", ActionID: action.ID, ToolName: action.ToolName, Status: types.PermissionStatusAllowed}, nil
	}
	return f.decision, nil
}

func (f *fakePermission) ApplyConfirmation(ctx context.Context, decision types.PermissionDecision, confirmation types.ToolConfirmation) (types.PermissionDecision, error) {
	if f.err != nil {
		return types.PermissionDecision{}, f.err
	}
	if f.confirmation.ID == "" {
		return types.PermissionDecision{ID: decision.ID, ActionID: decision.ActionID, ToolName: decision.ToolName, Status: types.PermissionStatusAllowed}, nil
	}
	return f.confirmation, nil
}

type fakeToolStorage struct {
	tools       map[string]types.ToolDefinition
	workspaces  map[string]types.Workspace
	roles       map[string]types.Role
	groups      map[string]types.ChatGroup
	sessions    map[string]types.Session
	attachments map[string]types.RunAttachment
	images      map[string]string
}

func newFakeToolStorage() *fakeToolStorage {
	return &fakeToolStorage{tools: map[string]types.ToolDefinition{}, workspaces: map[string]types.Workspace{}, roles: map[string]types.Role{}, groups: map[string]types.ChatGroup{}, sessions: map[string]types.Session{}, attachments: map[string]types.RunAttachment{}, images: map[string]string{}}
}

func (f *fakeToolStorage) SaveTool(ctx context.Context, tool types.ToolDefinition) error {
	f.tools[tool.ID] = tool
	return nil
}

func (f *fakeToolStorage) LoadTool(ctx context.Context, toolID string) (types.ToolDefinition, error) {
	tool, ok := f.tools[toolID]
	if !ok {
		return types.ToolDefinition{}, errors.New("tool missing")
	}
	return tool, nil
}

func (f *fakeToolStorage) ListTools(ctx context.Context) ([]types.ToolSummary, error) {
	summaries := make([]types.ToolSummary, 0, len(f.tools))
	for _, tool := range f.tools {
		summaries = append(summaries, types.ToolSummary{ID: tool.ID, Name: tool.Name, Description: tool.Description, Type: tool.Type, UpdatedAt: tool.UpdatedAt})
	}
	return summaries, nil
}

func (f *fakeToolStorage) SaveToolUserSettings(ctx context.Context, toolID string, settings types.ToolUserSettings) (types.ToolDefinition, error) {
	tool, ok := f.tools[toolID]
	if !ok {
		return types.ToolDefinition{}, errors.New("tool missing")
	}
	tool.UserConfig = settings.UserConfig
	tool.PromptDescriptionOverride = settings.PromptDescriptionOverride
	f.tools[toolID] = tool
	return tool, nil
}

func (f *fakeToolStorage) LoadWorkspace(ctx context.Context, workspaceID string) (types.Workspace, error) {
	workspace, ok := f.workspaces[workspaceID]
	if !ok {
		return types.Workspace{}, errors.New("workspace missing")
	}
	return workspace, nil
}

func (f *fakeToolStorage) LoadRole(ctx context.Context, roleID string) (types.Role, error) {
	role, ok := f.roles[roleID]
	if !ok {
		return types.Role{}, errors.New("role missing")
	}
	return role, nil
}

func (f *fakeToolStorage) LoadChatGroup(ctx context.Context, groupID string) (types.ChatGroup, error) {
	group, ok := f.groups[groupID]
	if !ok {
		return types.ChatGroup{}, errors.New("group missing")
	}
	return group, nil
}

func (f *fakeToolStorage) LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error) {
	session, ok := f.sessions[sessionID]
	if !ok {
		return types.Session{}, errors.New("session missing")
	}
	return session, nil
}

func (f *fakeToolStorage) LoadGroupSession(ctx context.Context, groupID string, sessionID string) (types.Session, error) {
	session, ok := f.sessions[sessionID]
	if !ok {
		return types.Session{}, errors.New("session missing")
	}
	return session, nil
}

func (f *fakeToolStorage) LoadWorkspaceSession(ctx context.Context, workspaceID string, roleID string, sessionID string) (types.Session, error) {
	session, ok := f.sessions[sessionID]
	if !ok {
		return types.Session{}, errors.New("session missing")
	}
	return session, nil
}

func (f *fakeToolStorage) SaveSessionMessageAttachment(ctx context.Context, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	return f.saveAttachment("roles", roleID, sessionID, attachment)
}

func (f *fakeToolStorage) SaveGroupSessionMessageAttachment(ctx context.Context, groupID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	return f.saveAttachment("groups", groupID, sessionID, attachment)
}

func (f *fakeToolStorage) SaveWorkspaceSessionMessageAttachment(ctx context.Context, workspaceID string, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	return f.saveAttachment("workspaces/"+workspaceID, roleID, sessionID, attachment)
}

func (f *fakeToolStorage) saveAttachment(scope string, scopeID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	if !strings.HasPrefix(attachment.DataURL, "data:image/") {
		return types.MessageAttachment{}, errors.New("attachment must be an image data url")
	}
	id := fmt.Sprintf("att-%d", len(f.attachments)+1)
	path := "sessions/" + scope + "/" + scopeID + "/" + sessionID + "/attachments/" + id + "/image.png"
	f.attachments[path] = attachment
	f.images[path] = attachment.DataURL
	return types.MessageAttachment{ID: id, Kind: "image", Name: attachment.Name, Mime: "image/png", Path: path}, nil
}

func (f *fakeToolStorage) LoadSessionAttachmentImage(ctx context.Context, relPath string) (string, error) {
	dataURL, ok := f.images[relPath]
	if !ok {
		return "", errors.New("attachment image missing")
	}
	return dataURL, nil
}

func assertAppErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %s", code)
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not AppError", err)
	}
	if appErr.Code != code {
		t.Fatalf("code = %s, want %s", appErr.Code, code)
	}
}

func Example_statuses() {
	fmt.Println(types.ToolStatusSuccess, types.ToolStatusFailed, types.ToolStatusDenied, types.ToolStatusCancelled)
	// Output: success failed denied cancelled
}
