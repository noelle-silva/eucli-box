package toolcalling

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

func capabilityTestPlan(storage *fakeToolStorage, tool types.ToolDefinition) types.ToolRunPlan {
	return types.ToolRunPlan{
		Scope: types.ToolRunScope{RoleID: "developer", SessionID: "session-1"},
		Tool:  tool,
	}
}

func capabilityTestTool() types.ToolDefinition {
	return types.ToolDefinition{
		ID:   "image-worker",
		Name: "image-worker",
		Capabilities: []types.ToolCapability{
			{ID: types.ToolCapabilityWorkspace, Access: types.ToolCapabilityAccessRead, Name: "工作区路径"},
			{ID: types.ToolCapabilitySessionAttachments, Access: types.ToolCapabilityAccessRead, Name: "读取会话图片"},
			{ID: types.ToolCapabilitySessionAttachments, Access: types.ToolCapabilityAccessWrite, Name: "写入会话图片"},
			{ID: types.ToolCapabilitySessionState, Access: types.ToolCapabilityAccessRead, Name: "读取会话状态"},
		},
		CapabilityGrants: map[string]bool{
			types.ToolCapabilityGrantKey(types.ToolCapabilitySessionAttachments, types.ToolCapabilityAccessRead):  true,
			types.ToolCapabilityGrantKey(types.ToolCapabilitySessionAttachments, types.ToolCapabilityAccessWrite): true,
			types.ToolCapabilityGrantKey(types.ToolCapabilitySessionState, types.ToolCapabilityAccessRead):        true,
		},
	}
}

func TestCapabilityRequestRequiresDeclaration(t *testing.T) {
	storage := newFakeToolStorage()
	tool := types.ToolDefinition{ID: "plain-tool", Name: "plain-tool"}
	session := newCapabilitySession(capabilityTestPlan(storage, tool), storage)
	result := session.handle(context.Background(), toolcontrol.CapabilityRequest{Capability: types.ToolCapabilitySessionAttachments, Access: types.ToolCapabilityAccessRead, Payload: json.RawMessage(`{"operation":"list"}`)})
	if result.Status != toolcontrol.CapabilityStatusFailed || !strings.Contains(result.Error, "未声明") {
		t.Fatalf("undeclared capability result = %#v", result)
	}
}

func TestCapabilityRequestRequiresUserGrant(t *testing.T) {
	storage := newFakeToolStorage()
	tool := types.ToolDefinition{ID: "image-worker", Name: "image-worker", Capabilities: capabilityTestTool().Capabilities}
	session := newCapabilitySession(capabilityTestPlan(storage, tool), storage)
	result := session.handle(context.Background(), toolcontrol.CapabilityRequest{Capability: types.ToolCapabilitySessionAttachments, Access: types.ToolCapabilityAccessRead, Payload: json.RawMessage(`{"operation":"list"}`)})
	if result.Status != toolcontrol.CapabilityStatusDenied || result.Error != toolcontrol.CapabilityDeniedMessage {
		t.Fatalf("ungranted capability result = %#v", result)
	}
}

func TestSessionAttachmentsReadListsAndReadsImages(t *testing.T) {
	storage := newFakeToolStorage()
	dataURL := "data:image/png;base64,aGVsbG8="
	storage.images["sessions/roles/developer/session-1/attachments/att-9/image.png"] = dataURL
	storage.sessions["session-1"] = types.Session{ID: "session-1", Messages: []types.Message{{ID: "m1", Attachments: []types.MessageAttachment{{ID: "att-9", Kind: "image", Name: "参考图", Mime: "image/png", Path: "sessions/roles/developer/session-1/attachments/att-9/image.png"}}}}}
	session := newCapabilitySession(capabilityTestPlan(storage, capabilityTestTool()), storage)

	list := session.handle(context.Background(), toolcontrol.CapabilityRequest{Capability: types.ToolCapabilitySessionAttachments, Access: types.ToolCapabilityAccessRead, Payload: json.RawMessage(`{"operation":"list"}`)})
	if list.Status != toolcontrol.CapabilityStatusSuccess {
		t.Fatalf("list result = %#v", list)
	}
	encodedList, _ := json.Marshal(list.Payload)
	if !strings.Contains(string(encodedList), "att-9") {
		t.Fatalf("list payload = %s", encodedList)
	}

	read := session.handle(context.Background(), toolcontrol.CapabilityRequest{Capability: types.ToolCapabilitySessionAttachments, Access: types.ToolCapabilityAccessRead, Payload: json.RawMessage(`{"operation":"read","attachmentId":"att-9"}`)})
	encodedRead, _ := json.Marshal(read.Payload)
	if read.Status != toolcontrol.CapabilityStatusSuccess || !strings.Contains(string(encodedRead), dataURL) {
		t.Fatalf("read result = %#v payload = %s", read, encodedRead)
	}
}

func TestSessionAttachmentWriteIsProxiedAndCollected(t *testing.T) {
	storage := newFakeToolStorage()
	session := newCapabilitySession(capabilityTestPlan(storage, capabilityTestTool()), storage)
	result := session.handle(context.Background(), toolcontrol.CapabilityRequest{Capability: types.ToolCapabilitySessionAttachments, Access: types.ToolCapabilityAccessWrite, Payload: json.RawMessage(`{"name":"生成图","dataUrl":"data:image/png;base64,aGVsbG8="}`)})
	if result.Status != toolcontrol.CapabilityStatusSuccess {
		t.Fatalf("write result = %#v", result)
	}
	produced := session.attachments()
	if len(produced) != 1 || produced[0].Name != "生成图" || produced[0].Kind != "image" {
		t.Fatalf("produced attachments = %#v", produced)
	}
	if len(storage.images) != 1 {
		t.Fatalf("storage images = %#v", storage.images)
	}
}

func TestSessionAttachmentWriteRejectsOversizePayload(t *testing.T) {
	storage := newFakeToolStorage()
	session := newCapabilitySession(capabilityTestPlan(storage, capabilityTestTool()), storage)
	session.attachmentQuota = 32
	payload, _ := json.Marshal(types.SessionAttachmentWriteRequest{Name: "大图", DataURL: "data:image/png;base64," + strings.Repeat("A", 64)})
	result := session.handle(context.Background(), toolcontrol.CapabilityRequest{Capability: types.ToolCapabilitySessionAttachments, Access: types.ToolCapabilityAccessWrite, Payload: payload})
	if result.Status != toolcontrol.CapabilityStatusFailed || !strings.Contains(result.Error, "单张") {
		t.Fatalf("oversize write result = %#v", result)
	}
	if len(storage.images) != 0 {
		t.Fatalf("oversize write must not reach storage, images = %#v", storage.images)
	}
}

// TestSessionAttachmentWriteBudgetIsReleasedOnFailure 验证写入预算：
// 失败写入原额退回，成功写入计入预算，预算耗尽后拒绝。
func TestSessionAttachmentWriteBudgetIsReleasedOnFailure(t *testing.T) {
	storage := newFakeToolStorage()
	session := newCapabilitySession(capabilityTestPlan(storage, capabilityTestTool()), storage)
	session.attachmentBudget = 23

	failedPayload, _ := json.Marshal(types.SessionAttachmentWriteRequest{Name: "非法", DataURL: "data:text/plain;base64,"})
	failed := session.handle(context.Background(), toolcontrol.CapabilityRequest{Capability: types.ToolCapabilitySessionAttachments, Access: types.ToolCapabilityAccessWrite, Payload: failedPayload})
	if failed.Status != toolcontrol.CapabilityStatusFailed {
		t.Fatalf("invalid write result = %#v", failed)
	}

	succeeded, _ := json.Marshal(types.SessionAttachmentWriteRequest{Name: "合法", DataURL: "data:image/png;base64,A"})
	result := session.handle(context.Background(), toolcontrol.CapabilityRequest{Capability: types.ToolCapabilitySessionAttachments, Access: types.ToolCapabilityAccessWrite, Payload: succeeded})
	if result.Status != toolcontrol.CapabilityStatusSuccess {
		t.Fatalf("write after released budget result = %#v", result)
	}

	exhausted := session.handle(context.Background(), toolcontrol.CapabilityRequest{Capability: types.ToolCapabilitySessionAttachments, Access: types.ToolCapabilityAccessWrite, Payload: succeeded})
	if exhausted.Status != toolcontrol.CapabilityStatusFailed || !strings.Contains(exhausted.Error, "总量") {
		t.Fatalf("exhausted budget result = %#v", exhausted)
	}
}

func TestSessionStateReadReturnsDeclaredValues(t *testing.T) {
	storage := newFakeToolStorage()
	storage.sessions["session-1"] = types.Session{ID: "session-1", Title: "测试会话", Messages: []types.Message{{ID: "m1", TokenEstimate: 12}, {ID: "m2", TokenEstimate: 30}}}
	storage.roles["developer"] = types.Role{ID: "developer", Name: "开发者"}
	session := newCapabilitySession(capabilityTestPlan(storage, capabilityTestTool()), storage)

	result := session.handle(context.Background(), toolcontrol.CapabilityRequest{Capability: types.ToolCapabilitySessionState, Access: types.ToolCapabilityAccessRead, Payload: json.RawMessage(`{}`)})
	if result.Status != toolcontrol.CapabilityStatusSuccess {
		t.Fatalf("state result = %#v", result)
	}
	encoded, _ := json.Marshal(result.Payload)
	for _, expected := range []string{"current-time", "session-title", "session-message-count", "session-token-estimate", "role-name"} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("state payload missing %s: %s", expected, encoded)
		}
	}
	if !strings.Contains(string(encoded), "\"value\":42") {
		t.Fatalf("state payload missing token estimate: %s", encoded)
	}
}

// TestReadWorkspaceReturnsRegisteredDirectories 验证注入类能力（工作区路径）
// 无需任何用户授权：宿主随执行主动提供的是工作环境事实，不设授权开关。
func TestReadWorkspaceReturnsRegisteredDirectories(t *testing.T) {
	storage := newFakeToolStorage()
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", Name: "工作区", Directories: []types.WorkspaceDirectory{{Path: "C:/work", Alias: "work"}}}
	plan := capabilityTestPlan(storage, capabilityTestTool())
	plan.Scope.WorkspaceID = "workspace-1"
	session := newCapabilitySession(plan, storage)

	result := session.handle(context.Background(), toolcontrol.CapabilityRequest{Capability: types.ToolCapabilityWorkspace, Access: types.ToolCapabilityAccessRead})
	encoded, _ := json.Marshal(result.Payload)
	if result.Status != toolcontrol.CapabilityStatusSuccess || !strings.Contains(string(encoded), "work") {
		t.Fatalf("workspace result = %#v payload = %s", result, encoded)
	}
}

// TestWorkspaceFenceSkipsToolWithoutDeclaration 验证围栏只对声明了工作区能力的
// 工具生效：未声明者即便带路径参数也不进入校验流程。
func TestWorkspaceFenceSkipsToolWithoutDeclaration(t *testing.T) {
	hostDir := t.TempDir()
	t.Chdir(hostDir)
	tool := testTool(t, buildTool(t, `package main
import "fmt"
func main() { fmt.Print(`+"`"+`{"status":"success","content":"ok","metadata":{}}`+"`"+`) }
`))
	tool.Capabilities = nil
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", Name: "Workspace", Directories: []types.WorkspaceDirectory{{Path: hostDir, Alias: "host"}}}
	system := newTestToolSystem(t, &fakePermission{decision: types.PermissionDecision{ID: "d1", ActionID: "a1", ToolName: tool.Name, Status: types.PermissionStatusAllowed}}, storage, Config{})

	plan, err := system.Prepare(context.Background(), types.ToolRunScope{RoleID: "developer", WorkspaceID: "workspace-1"}, types.ToolAction{ID: "a1", ToolName: tool.Name, Arguments: map[string]any{"path": "inside.txt"}})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if plan.WorkspaceFence != nil {
		t.Fatalf("undeclared tool must not enter workspace fence, plan = %#v", plan)
	}
}

// workspaceEchoTool 回显执行输入中的工作区标识，用于验证启动注入行为。
func workspaceEchoTool(t *testing.T) string {
	t.Helper()
	return buildTool(t, `package main
import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"eucli-box/pkg/types"
)

func main() {
	payload, _ := io.ReadAll(os.Stdin)
	var input types.ToolExecutionInput
	_ = json.Unmarshal(payload, &input)
	workspaceID := ""
	if input.Workspace != nil {
		workspaceID = input.Workspace.ID
	}
	output := map[string]any{"status": "success", "content": workspaceID, "metadata": map[string]any{}}
	encoded, _ := json.Marshal(output)
	fmt.Print(string(encoded))
}
`)
}

func TestExecuteInjectsWorkspaceValueOnlyForDeclaredTool(t *testing.T) {
	executable := workspaceEchoTool(t)
	storage := newFakeToolStorage()
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", Name: "工作区", Directories: []types.WorkspaceDirectory{{Path: t.TempDir(), Alias: "work"}}}

	declared := testTool(t, executable)
	system := newTestToolSystem(t, &fakePermission{}, storage, Config{})
	plan := allowedPlan(declared, executable)
	plan.Scope = types.ToolRunScope{RoleID: "developer", WorkspaceID: "workspace-1"}
	result, err := system.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "workspace-1" {
		t.Fatalf("declared tool workspace = %q, result = %#v", result.Content, result)
	}

	undeclared := declared
	undeclared.Capabilities = nil
	plan = allowedPlan(undeclared, executable)
	plan.Scope = types.ToolRunScope{RoleID: "developer", WorkspaceID: "workspace-1"}
	result, err = system.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "" {
		t.Fatalf("undeclared tool must not receive workspace, result = %#v", result)
	}
}

func TestSaveToolUserSettingsRejectsUndeclaredGrant(t *testing.T) {
	storage := newFakeToolStorage()
	tool := testTool(t, buildTool(t, `package main
func main() {}
`))
	storage.tools[tool.ID] = tool
	system := newTestToolSystem(t, &fakePermission{}, storage, Config{})
	_, err := system.SaveToolUserSettings(context.Background(), tool.ID, types.ToolUserSettings{
		UserConfig:       map[string]any{},
		CapabilityGrants: map[string]bool{types.ToolCapabilityGrantKey(types.ToolCapabilitySessionAttachments, types.ToolCapabilityAccessRead): true},
	})
	assertAppErrorCode(t, err, "tool.invalid_request")
}

// TestSaveToolUserSettingsRejectsInjectedCapabilityGrant 验证注入类能力
// 不接受用户授权：工作区路径随执行自动提供，不存在可授权的开关。
func TestSaveToolUserSettingsRejectsInjectedCapabilityGrant(t *testing.T) {
	storage := newFakeToolStorage()
	tool := testTool(t, buildTool(t, `package main
func main() {}
`))
	storage.tools[tool.ID] = tool
	system := newTestToolSystem(t, &fakePermission{}, storage, Config{})
	_, err := system.SaveToolUserSettings(context.Background(), tool.ID, types.ToolUserSettings{
		UserConfig:       map[string]any{},
		CapabilityGrants: map[string]bool{types.ToolCapabilityGrantKey(types.ToolCapabilityWorkspace, types.ToolCapabilityAccessRead): true},
	})
	assertAppErrorCode(t, err, "tool.invalid_request")
}
