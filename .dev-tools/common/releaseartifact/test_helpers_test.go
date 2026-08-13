package releaseartifact

import (
	"os"
	"path/filepath"
	"testing"
)

func repositoryRootForTest(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	current, err := filepath.Abs(workingDirectory)
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	for {
		// 工具资料位于主仓库根 tools/；开发工具模块自身的 go.mod 不是工具资料根，
		// 必须继续向上找到同时具备 go.mod 与工具资料目录的主仓库根。
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(current, "tools", "context7", "tool.json")); err == nil {
				return current
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("repository root not found from %s", workingDirectory)
		}
		current = parent
	}
}
