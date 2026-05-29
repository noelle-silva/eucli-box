package toolcalling

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	if plan.Status != types.ToolStatusDenied || plan.Decision.Reason != "blocked" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestApplyConfirmationUpdatesPlan(t *testing.T) {
	permissions := &fakePermission{confirmation: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: "web-search", Status: types.PermissionStatusAllowed, Reason: "approved"}}
	system := newTestToolSystem(t, permissions, newFakeToolStorage(), Config{})
	plan := types.ToolRunPlan{Decision: types.PermissionDecision{ID: "d1", Status: types.PermissionStatusNeedsConfirmation}}
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

func TestExecuteReturnsDeniedResult(t *testing.T) {
	system := newTestToolSystem(t, &fakePermission{}, newFakeToolStorage(), Config{})
	result, err := system.Execute(context.Background(), types.ToolRunPlan{Action: types.ToolAction{ID: "a1", ToolName: "file-reader"}, Decision: types.PermissionDecision{Status: types.PermissionStatusDenied, Reason: "blocked"}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != types.ToolStatusDenied || result.Error != "blocked" {
		t.Fatalf("result = %#v", result)
	}
}

func Example_statuses() {
	fmt.Println(types.ToolStatusSuccess, types.ToolStatusFailed, types.ToolStatusDenied, types.ToolStatusCancelled)
	// Output: success failed denied cancelled
}
