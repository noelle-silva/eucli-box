package toolcalling

import (
	"context"
	"path/filepath"
	"testing"

	"eucli-box/pkg/types"
)

// sessionAuthorizedScope 构造一个带会话身份的 workspace 执行范围。
func sessionAuthorizedScope(tool types.ToolDefinition, sessionID string) types.ToolRunScope {
	return types.ToolRunScope{RoleID: "developer", WorkspaceID: "workspace-1", SessionID: sessionID}
}

func newAuthorizedFenceStorage(t *testing.T, tool types.ToolDefinition, hostDir string, session types.Session) *fakeToolStorage {
	t.Helper()
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", Name: "Workspace", Directories: []types.WorkspaceDirectory{{Path: hostDir, Alias: "host"}}}
	storage.sessions[session.ID] = session
	return storage
}

// TestPrepareSkipsFenceWhenToolAuthorizedForSession 验证条款 3、7：
// 命中会话放行清单后，越界路径不再触发围栏确认，直接放行执行。
func TestPrepareSkipsFenceWhenToolAuthorizedForSession(t *testing.T) {
	hostDir := t.TempDir()
	outsideDir := t.TempDir()
	t.Chdir(hostDir)
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	session := types.Session{ID: "session-1", Metadata: types.PutSessionToolAuthorization(nil, tool.ID, true)}
	storage := newAuthorizedFenceStorage(t, tool, hostDir, session)
	system := newTestToolSystem(t, &fakePermission{decision: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusNeedsConfirmation}}, storage, Config{})

	plan, err := system.Prepare(context.Background(), sessionAuthorizedScope(tool, session.ID), types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"action": "read", "path": filepath.Join(outsideDir, "outside.txt")}})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if plan.PlanStatus != types.ToolPlanStatusReady {
		t.Fatalf("authorized plan should be ready, got %#v", plan)
	}
	if plan.WorkspaceFence != nil {
		t.Fatalf("authorized plan should skip fence, got %#v", plan.WorkspaceFence)
	}
	if plan.Decision.Status != types.PermissionStatusAllowed {
		t.Fatalf("authorized decision = %#v", plan.Decision)
	}
}

// TestPrepareStillDeniesAuthorizedToolOutsideRolePolicy 验证放行不复活硬性拒绝：
// 会话放行只提升“需询问”，不改变角色策略拒绝。
func TestPrepareStillDeniesAuthorizedToolOutsideRolePolicy(t *testing.T) {
	hostDir := t.TempDir()
	t.Chdir(hostDir)
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	session := types.Session{ID: "session-1", Metadata: types.PutSessionToolAuthorization(nil, tool.ID, true)}
	storage := newAuthorizedFenceStorage(t, tool, hostDir, session)
	system := newTestToolSystem(t, &fakePermission{decision: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusDenied, Reason: "blocked"}}, storage, Config{})

	plan, err := system.Prepare(context.Background(), sessionAuthorizedScope(tool, session.ID), types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"path": "inside.txt"}})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if plan.PlanStatus != types.ToolPlanStatusDenied {
		t.Fatalf("authorized tool outside role policy must stay denied, got %#v", plan)
	}
}

// TestApplyConfirmationRemembersSessionAuthorization 验证条款 5：
// 选择“本会话内始终同意”后写入放行清单，本次也直接放行。
func TestApplyConfirmationRemembersSessionAuthorization(t *testing.T) {
	hostDir := t.TempDir()
	t.Chdir(hostDir)
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	session := types.Session{ID: "session-1"}
	storage := newAuthorizedFenceStorage(t, tool, hostDir, session)
	permissions := &fakePermission{decision: types.PermissionDecision{ID: "role-decision", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusNeedsConfirmation}}
	system := newTestToolSystem(t, permissions, storage, Config{})

	plan, err := system.Prepare(context.Background(), sessionAuthorizedScope(tool, session.ID), types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"path": "inside.txt"}})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if plan.PlanStatus != types.ToolPlanStatusNeedsConfirmation {
		t.Fatalf("plan should need confirmation first, got %#v", plan)
	}
	updated, err := system.ApplyConfirmation(context.Background(), plan, types.ToolConfirmation{DecisionID: plan.Decision.ID, Approved: true, RememberForSession: true})
	if err != nil {
		t.Fatalf("ApplyConfirmation() error = %v", err)
	}
	if updated.PlanStatus != types.ToolPlanStatusReady || updated.Decision.Status != types.PermissionStatusAllowed {
		t.Fatalf("remembered confirmation must allow this run, got %#v", updated)
	}
	stored := storage.sessions[session.ID]
	if !types.SessionToolAuthorized(stored.Metadata, tool.ID) {
		t.Fatalf("session authorization was not persisted: %#v", stored.Metadata)
	}
}

// TestApplyConfirmationOnceDoesNotRemember 验证条款 6：
// “仅同意本次”维持现状，不写放行清单。
func TestApplyConfirmationOnceDoesNotRemember(t *testing.T) {
	hostDir := t.TempDir()
	t.Chdir(hostDir)
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	session := types.Session{ID: "session-1"}
	storage := newAuthorizedFenceStorage(t, tool, hostDir, session)
	permissions := &fakePermission{decision: types.PermissionDecision{ID: "role-decision", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusNeedsConfirmation}}
	system := newTestToolSystem(t, permissions, storage, Config{})

	plan, err := system.Prepare(context.Background(), sessionAuthorizedScope(tool, session.ID), types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"path": "inside.txt"}})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	updated, err := system.ApplyConfirmation(context.Background(), plan, types.ToolConfirmation{DecisionID: plan.Decision.ID, Approved: true})
	if err != nil {
		t.Fatalf("ApplyConfirmation() error = %v", err)
	}
	if updated.PlanStatus != types.ToolPlanStatusReady {
		t.Fatalf("once confirmation must allow this run, got %#v", updated)
	}
	if stored := storage.sessions[session.ID]; types.SessionToolAuthorized(stored.Metadata, tool.ID) {
		t.Fatalf("once confirmation must not persist authorization: %#v", stored.Metadata)
	}
}

// TestApplyConfirmationSessionRememberAppliesToNextRun 验证条款 7：
// 写入清单后，下一次调用运行前查清单命中，直接放行。
func TestApplyConfirmationSessionRememberAppliesToNextRun(t *testing.T) {
	hostDir := t.TempDir()
	outsideDir := t.TempDir()
	t.Chdir(hostDir)
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	session := types.Session{ID: "session-1"}
	storage := newAuthorizedFenceStorage(t, tool, hostDir, session)
	permissions := &fakePermission{decision: types.PermissionDecision{ID: "role-decision", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusNeedsConfirmation}}
	system := newTestToolSystem(t, permissions, storage, Config{})

	plan, err := system.Prepare(context.Background(), sessionAuthorizedScope(tool, session.ID), types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"path": filepath.Join(outsideDir, "outside.txt")}})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if plan.PlanStatus != types.ToolPlanStatusNeedsConfirmation {
		t.Fatalf("first run should need confirmation, got %#v", plan)
	}
	if _, err := system.ApplyConfirmation(context.Background(), plan, types.ToolConfirmation{DecisionID: plan.Decision.ID, Approved: true, RememberForSession: true}); err != nil {
		t.Fatalf("ApplyConfirmation() error = %v", err)
	}

	next, err := system.Prepare(context.Background(), sessionAuthorizedScope(tool, session.ID), types.ToolAction{ID: "a2", ToolName: tool.Name, Arguments: map[string]any{"path": filepath.Join(outsideDir, "outside.txt")}})
	if err != nil {
		t.Fatalf("Prepare() second run error = %v", err)
	}
	if next.PlanStatus != types.ToolPlanStatusReady {
		t.Fatalf("second run should be authorized directly, got %#v", next)
	}
}

// TestPrepareWithoutSessionStillUsesFence 验证无会话身份时不误放：
// 放行清单挂在会话上，没有会话就没有放行通道，围栏照旧。
func TestPrepareWithoutSessionStillUsesFence(t *testing.T) {
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
	if plan.PlanStatus != types.ToolPlanStatusNeedsConfirmation {
		t.Fatalf("session-less run must still fence, got %#v", plan)
	}
}
