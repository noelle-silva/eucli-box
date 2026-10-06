package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestForeignImport(t *testing.T) {
	repoModules := []string{"eucli-box", "devtools"}
	allowedPrefix := "eucli-box/tools/file_reader"
	cases := []struct {
		name       string
		importPath string
		want       bool
	}{
		{"标准库", "fmt", false},
		{"第三方依赖", "github.com/gorilla/websocket", false},
		{"本工具内部包", "eucli-box/tools/file_reader/internal/types", false},
		{"本工具根包", "eucli-box/tools/file_reader", false},
		{"相似前缀的另一工具", "eucli-box/tools/file_readerX", true},
		{"其他工具", "eucli-box/tools/web_fetch/internal/types", true},
		{"仓库内部共享包", "eucli-box/internal/foo", true},
		{"仓库根包", "eucli-box", true},
		{"开发工具模块", "devtools/common/toolkit", true},
	}
	for _, testCase := range cases {
		if got := foreignImport(repoModules, allowedPrefix, testCase.importPath); got != testCase.want {
			t.Errorf("%s：foreignImport(%q)=%v，期望 %v", testCase.name, testCase.importPath, got, testCase.want)
		}
	}
}

func TestScanToolImports(t *testing.T) {
	root := t.TempDir()
	toolDir := filepath.Join(root, "tools", "file_reader")
	if err := os.MkdirAll(filepath.Join(toolDir, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := "package main\n\nimport (\n\t\"fmt\"\n\t\"eucli-box/tools/file_reader/internal/types\"\n\t\"eucli-box/internal/foo\"\n)\n"
	if err := os.WriteFile(filepath.Join(toolDir, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := Tool{Name: "file_reader", Dir: toolDir}
	violations, err := scanToolImports(root, tool, []string{"eucli-box"}, "eucli-box/tools/file_reader")
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 {
		t.Fatalf("期望 1 处越界引用，实际 %d：%+v", len(violations), violations)
	}
	if violations[0].Import != "eucli-box/internal/foo" {
		t.Errorf("越界引用路径错误：%q", violations[0].Import)
	}
	if violations[0].File != "tools/file_reader/main.go" {
		t.Errorf("越界引用文件路径错误：%q", violations[0].File)
	}
	if violations[0].Line != 6 {
		t.Errorf("越界引用行号错误：%d", violations[0].Line)
	}
}

func TestReadModulePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(path, []byte("module eucli-box\n\ngo 1.23.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	modulePath, err := readModulePath(path)
	if err != nil {
		t.Fatal(err)
	}
	if modulePath != "eucli-box" {
		t.Errorf("module 路径错误：%q", modulePath)
	}
}

func TestParseGoWorkUsePaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go.work")
	content := "go 1.23.0\n\nuse (\n\t.\n\t./.dev-tools // 工具模块\n)\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs, err := parseGoWorkUsePaths(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 || dirs[0] != "." || dirs[1] != "./.dev-tools" {
		t.Errorf("use 目录解析错误：%+v", dirs)
	}
}
