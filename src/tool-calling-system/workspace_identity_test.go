package toolcalling

import (
	"context"
	"testing"

	"eucli-box/pkg/types"
)

// workspaceIdentityEchoTool 把注入到输入里的工作区标识原样吐回，
// 用于验证注入只发生在声明了该能力的工具上。
func workspaceIdentityEchoTool(t *testing.T) string {
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
	output := map[string]any{"status": "success", "content": input.WorkspaceID, "metadata": map[string]any{}}
	encoded, _ := json.Marshal(output)
	fmt.Print(string(encoded))
}
`)
}

// TestExecuteInjectsWorkspaceIdentityOnlyForDeclaredTool 钉住注入语义：
// 声明了工作区标识能力的工具拿到当前工作区标识；未声明的一律拿不到；
// 不在工作区会话里时，声明的工具也拿不到。
func TestExecuteInjectsWorkspaceIdentityOnlyForDeclaredTool(t *testing.T) {
	executable := workspaceIdentityEchoTool(t)
	storage := newFakeToolStorage()
	storage.workspaces["workspace-1"] = types.Workspace{ID: "workspace-1", Name: "工作区"}

	declared := testTool(t, executable)
	declared.Capabilities = []types.ToolCapability{
		{ID: types.ToolCapabilityWorkspaceIdentity, Access: types.ToolCapabilityAccessRead, Name: "工作区标识"},
	}
	system := newTestToolSystem(t, &fakePermission{}, storage, Config{})
	plan := allowedPlan(declared, executable)
	plan.Scope = types.ToolRunScope{RoleID: "developer", WorkspaceID: "workspace-1"}
	result, err := system.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "workspace-1" {
		t.Fatalf("declared tool workspace identity = %q, result = %#v", result.Content, result)
	}

	undeclared := testTool(t, executable)
	plan = allowedPlan(undeclared, executable)
	plan.Scope = types.ToolRunScope{RoleID: "developer", WorkspaceID: "workspace-1"}
	result, err = system.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "" {
		t.Fatalf("undeclared tool must not receive workspace identity, result = %#v", result)
	}

	plan = allowedPlan(declared, executable)
	plan.Scope = types.ToolRunScope{RoleID: "developer"}
	result, err = system.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "" {
		t.Fatalf("role session must not inject workspace identity, result = %#v", result)
	}
}
