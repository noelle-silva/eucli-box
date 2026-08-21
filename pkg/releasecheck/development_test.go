package releasecheck

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// TestDevelopmentSourceReaderUsesRealBuiltPackage 用当前源码构建的开发版工具
// 成品验证本地候选；成品目录通过环境变量传入，缺失时跳过。
func TestDevelopmentSourceReaderUsesRealBuiltPackage(t *testing.T) {
	packageRoot := strings.TrimSpace(os.Getenv("EUCLI_DEV_TOOL_TEST_PACKAGE_ROOT"))
	if packageRoot == "" {
		t.Skip("未设置 EUCLI_DEV_TOOL_TEST_PACKAGE_ROOT，跳过开发成品集成测试")
	}
	if _, err := os.Stat(packageRoot); err != nil {
		t.Skipf("开发成品目录不存在，跳过：%s", err)
	}
	reader, err := NewDevelopmentSourceReader("1", packageRoot)
	if err != nil {
		t.Fatalf("NewDevelopmentSourceReader() error = %v", err)
	}
	candidate, err := reader.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "shell_command"})
	if err != nil {
		t.Fatalf("LatestCandidate() error = %v", err)
	}
	if candidate.Version == "" || candidate.ArchiveURL == "" || candidate.SHA256 == "" {
		t.Fatalf("candidate = %#v", candidate)
	}
	source, err := DevelopmentSource(candidate)
	if err != nil {
		t.Fatalf("DevelopmentSource() error = %v", err)
	}
	if !source.Development || source.ArchiveURL != candidate.ArchiveURL {
		t.Fatalf("development source = %#v", source)
	}
}

func TestDevelopmentSourceInactiveByDefault(t *testing.T) {
	reader, err := NewDevelopmentSourceReader("", "")
	if err != nil {
		t.Fatalf("NewDevelopmentSourceReader() error = nil expected %v", err)
	}
	if reader != nil {
		t.Fatalf("reader should be nil when source flag absent")
	}
}

func TestDevelopmentSourceRejectsWrongIdentity(t *testing.T) {
	_, err := NewDevelopmentSourceReader("1", filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("NewDevelopmentSourceReader() error = nil, want missing root error")
	}
}
