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

func TestParseTextToolRequestsExtractsCleanContentAndIntent(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	content, intents, err := system.ParseTextToolRequests(context.Background(), `I will check it.

<<<TOOL_REQUEST>>>
[tool]: web-search
[query]: 东京明天天气
[limit]: 5
<<<END_TOOL_REQUEST>>>

I will continue after the result.`)
	if err != nil {
		t.Fatalf("ParseTextToolRequests() error = %v", err)
	}
	if content != "I will check it.\n\nI will continue after the result." {
		t.Fatalf("content = %q", content)
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
	if prompt.Role != "system" || !strings.Contains(prompt.Content, "<<<TOOL_REQUEST>>>") || !strings.Contains(prompt.Content, "[tool]: tool-name") || !strings.Contains(prompt.Content, "web-search") || !strings.Contains(prompt.Content, "query") {
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

func TestParseTextToolRequestsExtractsMultipleBlocks(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	_, intents, err := system.ParseTextToolRequests(context.Background(), `<<<TOOL_REQUEST>>>
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

func TestVisibleTextToolContentHidesCompleteAndPartialBlocks(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	content, err := system.VisibleTextToolContent(context.Background(), "I will check.\n<<<TOOL_REQUEST>>>\n[tool]: web-search\n[query]: 东京明天天气")
	if err != nil {
		t.Fatalf("VisibleTextToolContent() error = %v", err)
	}
	if content != "I will check." {
		t.Fatalf("partial content = %q", content)
	}
	content, err = system.VisibleTextToolContent(context.Background(), "I will check.\n<<<TOOL_REQUEST>>>\n[tool]: web-search\n[query]: 东京明天天气\n<<<END_TOOL_REQUEST>>>\nDone.")
	if err != nil {
		t.Fatalf("VisibleTextToolContent() error = %v", err)
	}
	if content != "I will check.\nDone." {
		t.Fatalf("complete content = %q", content)
	}
	content, err = system.VisibleTextToolContent(context.Background(), "I will check.\n<<<TOOL")
	if err != nil {
		t.Fatalf("VisibleTextToolContent() error = %v", err)
	}
	if content != "I will check." {
		t.Fatalf("marker prefix content = %q", content)
	}
}

func TestTextToolProtocolIgnoresMarkersInsideMarkdownFence(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	source := "Example:\n```text\n<<<TOOL_REQUEST>>>\n[tool]: web-search\n[query]: 东京明天天气\n<<<END_TOOL_REQUEST>>>\n```"
	content, intents, err := system.ParseTextToolRequests(context.Background(), source)
	if err != nil {
		t.Fatalf("ParseTextToolRequests() error = %v", err)
	}
	if content != source || len(intents) != 0 {
		t.Fatalf("content=%q intents=%#v", content, intents)
	}
	visible, err := system.VisibleTextToolContent(context.Background(), source)
	if err != nil {
		t.Fatalf("VisibleTextToolContent() error = %v", err)
	}
	if visible != source {
		t.Fatalf("visible = %q", visible)
	}
}

func TestParseTextToolRequestsFailsOnBadProtocol(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	_, _, err := system.ParseTextToolRequests(context.Background(), `<<<TOOL_REQUEST>>>
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
	plan, err := system.Prepare(context.Background(), "developer", types.ToolAction{ID: "a1", ToolName: tool.Name})
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
	_, err := system.Prepare(context.Background(), "developer", types.ToolAction{ID: "a1", ToolName: tool.Name})
	assertAppErrorCode(t, err, "tool.not_found")
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
	tools map[string]types.ToolDefinition
}

func newFakeToolStorage() *fakeToolStorage {
	return &fakeToolStorage{tools: map[string]types.ToolDefinition{}}
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
