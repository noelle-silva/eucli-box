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

func TestParseTextToolRequestsExtractsIntentWithoutConsumingContent(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	intents, err := system.ParseTextToolRequests(context.Background(), `I will check it.

<<<TOOL_REQUEST>>>
[tool]: web-search
[query]: 东京明天天气
[limit]: 5
<<<END_TOOL_REQUEST>>>

I will continue after the result.`)
	if err != nil {
		t.Fatalf("ParseTextToolRequests() error = %v", err)
	}
	if len(intents) != 1 || intents[0].ToolName != "web-search" || intents[0].Arguments["query"] != "东京明天天气" || intents[0].Arguments["limit"] != "5" {
		t.Fatalf("intents = %#v", intents)
	}
	if intents[0].ID == "" || intents[0].Raw == "" || intents[0].Source != types.ToolCallSourceTextProtocol {
		t.Fatalf("intent metadata = %#v", intents[0])
	}
}

func TestTextToolInstructionsDescribeProtocolAndTools(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	prompt, err := system.TextToolInstructions(context.Background(), []types.ToolDefinition{{ID: "web-search", Name: "web-search", Description: "Search the web", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}}}})
	if err != nil {
		t.Fatalf("TextToolInstructions() error = %v", err)
	}
	if prompt.Role != "system" || !strings.Contains(prompt.Content, "<<<TOOL_REQUEST>>>") || !strings.Contains(prompt.Content, "[tool]: tool-name") || !strings.Contains(prompt.Content, "web-search") || !strings.Contains(prompt.Content, "query") || !strings.Contains(prompt.Content, "execute independent tools in parallel") {
		t.Fatalf("prompt = %#v", prompt)
	}
}

func TestTextToolInstructionsPreferPromptDescription(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	prompt, err := system.TextToolInstructions(context.Background(), []types.ToolDefinition{{ID: "shell_command", Name: "shell_command", Description: "Short description", PromptDescription: "Detailed prompt usage"}})
	if err != nil {
		t.Fatalf("TextToolInstructions() error = %v", err)
	}
	if !strings.Contains(prompt.Content, "Detailed prompt usage") || strings.Contains(prompt.Content, "Short description") {
		t.Fatalf("prompt = %s", prompt.Content)
	}
}

func TestSaveToolUserSettingsUpdatesConfigAndPromptOverride(t *testing.T) {
	storage := newFakeToolStorage()
	system := newTestToolSystem(t, &fakePermission{}, storage, Config{})
	tool := types.ToolDefinition{ID: "shell_command", Name: "shell_command", Description: "Run shell command", Type: "local", DefaultConfig: map[string]any{"provider": "git-bash"}}
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

func TestTextToolInstructionsPreferPromptDescriptionOverride(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	prompt, err := system.TextToolInstructions(context.Background(), []types.ToolDefinition{{ID: "shell_command", Name: "shell_command", Description: "Short description", PromptDescription: "Detailed prompt usage", PromptDescriptionOverride: "User prompt usage"}})
	if err != nil {
		t.Fatalf("TextToolInstructions() error = %v", err)
	}
	if !strings.Contains(prompt.Content, "User prompt usage") || strings.Contains(prompt.Content, "Detailed prompt usage") || strings.Contains(prompt.Content, "Short description") {
		t.Fatalf("prompt = %s", prompt.Content)
	}
}

func TestParseTextToolRequestsExtractsMultipleBlocks(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	intents, err := system.ParseTextToolRequests(context.Background(), `<<<TOOL_REQUEST>>>
[tool]: web-search
[query]: 东京明天天气
<<<END_TOOL_REQUEST>>>

<<<TOOL_REQUEST>>>
[tool]: read-file
[path]: README.md
<<<END_TOOL_REQUEST>>>`)
	if err != nil {
		t.Fatalf("ParseTextToolRequests() error = %v", err)
	}
	if len(intents) != 2 || intents[0].ToolName != "web-search" || intents[1].ToolName != "read-file" || intents[1].Arguments["path"] != "README.md" {
		t.Fatalf("intents = %#v", intents)
	}
}

func TestTextToolProtocolIgnoresMarkersInsideMarkdownFence(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	source := "Example:\n```text\n<<<TOOL_REQUEST>>>\n[tool]: web-search\n[query]: 东京明天天气\n<<<END_TOOL_REQUEST>>>\n```"
	intents, err := system.ParseTextToolRequests(context.Background(), source)
	if err != nil {
		t.Fatalf("ParseTextToolRequests() error = %v", err)
	}
	if len(intents) != 0 {
		t.Fatalf("intents=%#v", intents)
	}
}

func TestParseTextToolRequestsFailsOnBadProtocol(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	_, err := system.ParseTextToolRequests(context.Background(), `<<<TOOL_REQUEST>>>
[query]: 东京明天天气
<<<END_TOOL_REQUEST>>>`)
	assertAppErrorCode(t, err, "tool.protocol_invalid")
}

func TestPrepareReturnsDeniedPlanWhenPermissionDenies(t *testing.T) {
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	system := newTestToolSystem(t, &fakePermission{decision: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusDenied, Reason: "blocked"}}, storage, Config{})
	plan, err := system.Prepare(context.Background(), "developer", "", types.ToolAction{ID: "a1", ToolName: tool.Name})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if plan.PlanStatus != types.ToolPlanStatusDenied || plan.Decision.Reason != "blocked" {
		t.Fatalf("plan = %#v", plan)
	}
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

func TestExecuteNormalizesTimeout(t *testing.T) {
	executable := buildTool(t, `package main
import "time"
func main() { time.Sleep(2 * time.Second) }
`)
	tool := testTool(t, executable)
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{ToolTimeout: 10 * time.Millisecond})
	result, err := system.Execute(context.Background(), allowedPlan(tool, executable))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != types.ToolStatusFailed || result.Error != "tool execution timed out" {
		t.Fatalf("result = %#v", result)
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
	_, err := system.Prepare(context.Background(), "developer", "", types.ToolAction{ID: "a1", ToolName: tool.Name})
	assertAppErrorCode(t, err, "tool.not_found")
}

func TestWorkspaceFenceAllowsRegisteredPath(t *testing.T) {
	hostDir := t.TempDir()
	t.Chdir(hostDir)
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	tool.ID = "file_operator"
	tool.Name = "file_operator"
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", Name: "Workspace", Directories: []types.WorkspaceDirectory{{Path: hostDir, Alias: "host"}}}
	system := newTestToolSystem(t, &fakePermission{decision: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusAllowed}}, storage, Config{})

	plan, err := system.Prepare(context.Background(), "developer", "workspace-1", types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"action": "read", "path": "inside.txt"}})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if plan.PlanStatus != types.ToolPlanStatusReady || plan.WorkspaceFence == nil || plan.WorkspaceFence.RequiresConfirmation {
		t.Fatalf("plan = %#v", plan)
	}
	if len(plan.WorkspaceFence.Paths) != 1 || !plan.WorkspaceFence.Paths[0].WithinWorkspace {
		t.Fatalf("fence paths = %#v", plan.WorkspaceFence.Paths)
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
	tool.ID = "file_operator"
	tool.Name = "file_operator"
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", Name: "Workspace", Directories: []types.WorkspaceDirectory{{Path: hostDir, Alias: "host"}}}
	system := newTestToolSystem(t, &fakePermission{decision: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusAllowed}}, storage, Config{})

	plan, err := system.Prepare(context.Background(), "developer", "workspace-1", types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"action": "read", "path": filepath.Join(outsideDir, "outside.txt")}})
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
	tool.ID = "file_operator"
	tool.Name = "file_operator"
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", Name: "Workspace", Directories: []types.WorkspaceDirectory{{Path: hostDir, Alias: "host"}}}
	permissions := &fakePermission{decision: types.PermissionDecision{ID: "role-decision", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusNeedsConfirmation}}
	system := newTestToolSystem(t, permissions, storage, Config{})

	plan, err := system.Prepare(context.Background(), "developer", "workspace-1", types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"action": "read", "path": filepath.Join(outsideDir, "outside.txt")}})
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
		ID:          "file-reader",
		Name:        "file-reader",
		Description: "Read files",
		Type:        "local",
		Directory:   dir,
		Binaries:    []types.ToolBinary{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Path: filepath.Base(executable)}},
		UserConfig:  map[string]any{"limit": 10},
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

func buildTool(t *testing.T, source string) string {
	t.Helper()
	dir := t.TempDir()
	sourceFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(sourceFile, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	exe := filepath.Join(dir, "tool")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, sourceFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build helper failed: %v\n%s", err, output)
	}
	return exe
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
	tools      map[string]types.ToolDefinition
	workspaces map[string]types.Workspace
}

func newFakeToolStorage() *fakeToolStorage {
	return &fakeToolStorage{tools: map[string]types.ToolDefinition{}, workspaces: map[string]types.Workspace{}}
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
