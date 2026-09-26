package toolcalling

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"eucli-box/pkg/types"
)

// hostDirectoryEchoTool 回显执行输入中的宿主工作目录。
func hostDirectoryEchoTool(t *testing.T) string {
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
	output := map[string]any{"status": "success", "content": input.HostWorkingDirectory, "metadata": map[string]any{}}
	encoded, _ := json.Marshal(output)
	fmt.Print(string(encoded))
}
`)
}

// TestExecuteUsesConfiguredWorkDirectory 验证无工作区会话下，宿主交给工具的
// 工作目录是配置的默认工作目录，而不是宿主部署目录。
func TestExecuteUsesConfiguredWorkDirectory(t *testing.T) {
	executable := hostDirectoryEchoTool(t)
	storage := newFakeToolStorage()
	workDirectory := t.TempDir()
	storage.workDirectory = workDirectory
	tool := testTool(t, executable)
	system := newTestToolSystem(t, &fakePermission{}, storage, Config{})

	result, err := system.Execute(context.Background(), allowedPlan(tool, executable))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != workDirectory {
		t.Fatalf("tool work directory = %q, want %q", result.Content, workDirectory)
	}
}

// TestSaveToolWorkDirectoryConfigCreatesDirectory 验证保存配置时目录被真实创建。
func TestSaveToolWorkDirectoryConfigCreatesDirectory(t *testing.T) {
	storage := newFakeToolStorage()
	system := newTestToolSystem(t, &fakePermission{}, storage, Config{})
	target := filepath.Join(t.TempDir(), "tool-work")

	saved, err := system.SaveToolWorkDirectoryConfig(context.Background(), types.ToolWorkDirectoryConfig{Directory: target})
	if err != nil {
		t.Fatalf("SaveToolWorkDirectoryConfig() error = %v", err)
	}
	if saved.Directory != target {
		t.Fatalf("saved directory = %q, want %q", saved.Directory, target)
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Fatalf("work directory was not created: %v", err)
	}
}

// TestSaveToolWorkDirectoryConfigRejectsUnavailableDirectory 验证不可创建的
// 目录被拒绝保存，配置不会被写坏。
func TestSaveToolWorkDirectoryConfigRejectsUnavailableDirectory(t *testing.T) {
	storage := newFakeToolStorage()
	system := newTestToolSystem(t, &fakePermission{}, storage, Config{})
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("occupied"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := system.SaveToolWorkDirectoryConfig(context.Background(), types.ToolWorkDirectoryConfig{Directory: blocker})
	assertAppErrorCode(t, err, "tool.invalid_request")
}

// TestLoadToolWorkDirectoryConfigIsAlwaysPopulated 验证配置恒有值：未设置时
// 读取结果为默认目录，不存在"空值"状态。
func TestLoadToolWorkDirectoryConfigIsAlwaysPopulated(t *testing.T) {
	storage := newFakeToolStorage()
	system := newTestToolSystem(t, &fakePermission{}, storage, Config{})

	config, err := system.LoadToolWorkDirectoryConfig(context.Background())
	if err != nil {
		t.Fatalf("LoadToolWorkDirectoryConfig() error = %v", err)
	}
	if config.Directory != types.DefaultToolWorkDirectory() {
		t.Fatalf("work directory = %q, want default %q", config.Directory, types.DefaultToolWorkDirectory())
	}
}
