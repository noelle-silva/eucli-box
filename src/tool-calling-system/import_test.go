package toolcalling

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// writeImportArchive 把成品目录打包成磁盘上的导入包。
func writeImportArchive(t *testing.T, contentDir string) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "import.zip")
	if err := os.WriteFile(target, zipBytes(t, contentDir), 0o644); err != nil {
		t.Fatalf("write import archive: %v", err)
	}
	return target
}

// buildImportToolContents 生成一个纯本地最小工具包目录：只有工具定义与可执行文件。
func buildImportToolContents(t *testing.T, id string, version string, minimum string, maximum string) string {
	t.Helper()
	exe := buildTool(t, probeToolSource)
	binaryPayload, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	contentDir := t.TempDir()
	binaryPath := filepath.ToSlash(filepath.Join("binary", "windows-amd64", id+".exe"))
	definition := types.ToolDefinition{
		ID:                    id,
		Name:                  "Demo " + id,
		Description:           "demo tool",
		Version:               version,
		EucliBoxCompatibility: types.EucliBoxCompatibility{MinimumVersion: minimum, MaximumVersionExclusive: maximum},
		DefaultInvocationMode: types.ToolInvocationModeSync,
		Type:                  "local",
		BodyDirectory:         ".",
		Binaries:              []types.ToolBinary{{GOOS: "windows", GOARCH: "amd64", Path: binaryPath}},
	}
	definitionPayload, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	files := map[string][]byte{
		"definition.json": definitionPayload,
		binaryPath:        binaryPayload,
	}
	for name, payload := range files {
		path := filepath.Join(contentDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return contentDir
}

// TestImportToolPackageInstallsMinimalLocalPackage 纯本地最小包（无说明文档、无发行资料）
// 直接导入并完成准备、探测与切换。
func TestImportToolPackageInstallsMinimalLocalPackage(t *testing.T) {
	fixture := newToolOperationFixture(t)
	contentDir := buildImportToolContents(t, "local-demo", "0.1.0", "0.1.0", "0.2.0")
	state, err := fixture.system.ImportToolPackage(context.Background(), writeImportArchive(t, contentDir))
	if err != nil {
		t.Fatalf("ImportToolPackage() error = %v", err)
	}
	if isTerminalArtifactStatus(state.Status) {
		t.Fatalf("ImportToolPackage() returned terminal state = %#v", state)
	}
	final := fixture.waitToolTerminal(t, "local-demo")
	if final.Status != types.ArtifactStatusActive || final.CurrentVersion != "0.1.0" {
		t.Fatalf("final state = %#v", final)
	}
	tool, err := fixture.system.LoadTool(context.Background(), "local-demo")
	if err != nil {
		t.Fatalf("LoadTool() error = %v", err)
	}
	if tool.Version != "0.1.0" {
		t.Fatalf("tool version = %s", tool.Version)
	}
	if _, err := os.Stat(filepath.Join(fixture.programRoot, "local-demo", "current.json")); err != nil {
		t.Fatalf("current.json missing: %v", err)
	}
}

// TestImportToolPackageRejectsPackageWithoutDefinition 缺身份定义文件的包同步拒绝。
func TestImportToolPackageRejectsPackageWithoutDefinition(t *testing.T) {
	fixture := newToolOperationFixture(t)
	contentDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(contentDir, "README.md"), []byte("nothing here"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	_, err := fixture.system.ImportToolPackage(context.Background(), writeImportArchive(t, contentDir))
	if err == nil || !strings.Contains(err.Error(), "工具定义文件") {
		t.Fatalf("ImportToolPackage() error = %v", err)
	}
}

// TestImportToolPackageRejectsIncompatiblePackage 兼容声明不满足时同步拒绝并给出原因。
func TestImportToolPackageRejectsIncompatiblePackage(t *testing.T) {
	fixture := newToolOperationFixture(t)
	contentDir := buildImportToolContents(t, "local-demo", "0.1.0", "0.9.0", "1.0.0")
	state, err := fixture.system.ImportToolPackage(context.Background(), writeImportArchive(t, contentDir))
	if err != nil {
		t.Fatalf("ImportToolPackage() error = %v", err)
	}
	if state.Status != types.ArtifactStatusBlocked || state.Error.Code != types.ArtifactErrorCompatibility {
		t.Fatalf("state = %#v", state)
	}
	if state.Error.Message == "" {
		t.Fatal("missing compatibility reason")
	}
	if _, err := os.Stat(filepath.Join(fixture.programRoot, "local-demo")); !os.IsNotExist(err) {
		t.Fatal("incompatible package left program artifacts")
	}
}

// TestImportToolPackageUpdatesInstalledTool 导入更高版本即完成更新。
func TestImportToolPackageUpdatesInstalledTool(t *testing.T) {
	fixture := newToolOperationFixture(t)
	first := buildImportToolContents(t, "local-demo", "0.1.0", "0.1.0", "0.2.0")
	if _, err := fixture.system.ImportToolPackage(context.Background(), writeImportArchive(t, first)); err != nil {
		t.Fatalf("ImportToolPackage(first) error = %v", err)
	}
	if final := fixture.waitToolTerminal(t, "local-demo"); final.Status != types.ArtifactStatusActive {
		t.Fatalf("first final = %#v", final)
	}
	second := buildImportToolContents(t, "local-demo", "0.1.1", "0.1.0", "0.2.0")
	if _, err := fixture.system.ImportToolPackage(context.Background(), writeImportArchive(t, second)); err != nil {
		t.Fatalf("ImportToolPackage(second) error = %v", err)
	}
	final := fixture.waitToolTerminal(t, "local-demo")
	if final.Status != types.ArtifactStatusActive || final.CurrentVersion != "0.1.1" {
		t.Fatalf("second final = %#v", final)
	}
}
