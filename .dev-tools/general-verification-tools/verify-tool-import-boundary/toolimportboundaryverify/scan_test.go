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

func TestScanUnitImports(t *testing.T) {
	root := t.TempDir()
	unitDir := filepath.Join(root, "tools", "file_reader")
	if err := os.MkdirAll(filepath.Join(unitDir, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := "package main\n\nimport (\n\t\"fmt\"\n\t\"eucli-box/tools/file_reader/internal/types\"\n\t\"eucli-box/internal/foo\"\n)\n"
	if err := os.WriteFile(filepath.Join(unitDir, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	target := unit{Name: "file_reader", Dir: unitDir}
	violations, err := scanUnitImports(root, target, []string{"eucli-box"}, "eucli-box/tools/file_reader")
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

// TestScanPluginImports 钉住系统插件同样受同一套边界校验约束：
// 插件引用自己文件夹内的代码放行，引用仓库内其他位置（含其他插件）判违规。
func TestScanPluginImports(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "system-plugins", "weather-plugin")
	if err := os.MkdirAll(filepath.Join(pluginDir, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := "package main\n\nimport (\n\t\"fmt\"\n\t\"eucli-box/system-plugins/weather-plugin/internal/systemplugin\"\n\t\"eucli-box/system-plugins/time-plugin/internal/systemplugin\"\n\t\"devtools/common/toolkit\"\n)\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	target := unit{Name: "weather-plugin", Dir: pluginDir}
	violations, err := scanUnitImports(root, target, []string{"eucli-box", "devtools"}, "eucli-box/system-plugins/weather-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 2 {
		t.Fatalf("期望 2 处越界引用，实际 %d：%+v", len(violations), violations)
	}
	if violations[0].Import != "eucli-box/system-plugins/time-plugin/internal/systemplugin" {
		t.Errorf("第一条越界引用路径错误：%q", violations[0].Import)
	}
	if violations[1].Import != "devtools/common/toolkit" {
		t.Errorf("第二条越界引用路径错误：%q", violations[1].Import)
	}
	if violations[0].File != "system-plugins/weather-plugin/main.go" {
		t.Errorf("越界引用文件路径错误：%q", violations[0].File)
	}
}

// TestUnitImportPrefix 钉住单元应处的包路径前缀，并拒绝单元目录内出现独立 go.mod 的架构漂移。
func TestUnitImportPrefix(t *testing.T) {
	root := t.TempDir()
	toolsCategory := boundaryCategory{Key: "tools", Label: "AI 工具"}
	pluginCategory := boundaryCategory{Key: "system-plugins", Label: "系统插件"}

	toolDir := filepath.Join(root, "tools", "file_reader")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prefix, err := unitImportPrefix("eucli-box", toolsCategory, unit{Name: "file_reader", Dir: toolDir})
	if err != nil {
		t.Fatal(err)
	}
	if prefix != "eucli-box/tools/file_reader" {
		t.Errorf("工具前缀错误：%q", prefix)
	}

	pluginDir := filepath.Join(root, "system-plugins", "time-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prefix, err = unitImportPrefix("eucli-box", pluginCategory, unit{Name: "time-plugin", Dir: pluginDir})
	if err != nil {
		t.Fatal(err)
	}
	if prefix != "eucli-box/system-plugins/time-plugin" {
		t.Errorf("插件前缀错误：%q", prefix)
	}

	if err := os.WriteFile(filepath.Join(pluginDir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := unitImportPrefix("eucli-box", pluginCategory, unit{Name: "time-plugin", Dir: pluginDir}); err == nil {
		t.Error("插件目录内出现独立 go.mod 时应当失败")
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
