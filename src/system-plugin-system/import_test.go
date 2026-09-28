package systemplugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/systemplugin"
	"eucli-box/pkg/types"
)

// writeImportPluginArchive 把成品目录打包成磁盘上的导入包。
func writeImportPluginArchive(t *testing.T, contentDir string) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "import.zip")
	if err := os.WriteFile(target, zipPluginBytes(t, contentDir), 0o644); err != nil {
		t.Fatalf("write import archive: %v", err)
	}
	return target
}

// buildImportPluginContents 生成一个纯本地最小插件包目录：
// 只有身份声明、配置默认值与可执行文件（含桩插件行为资料）。
func buildImportPluginContents(t *testing.T, id string, version string, minimum string, maximum string) string {
	t.Helper()
	binaryPayload, err := os.ReadFile(stubExecutable)
	if err != nil {
		t.Fatalf("read stub executable: %v", err)
	}
	contentDir := t.TempDir()
	binaryPath := filepath.ToSlash(filepath.Join("binary", id+".exe"))
	manifest := types.SystemPluginManifest{
		ProtocolVersion:       systemplugin.ProtocolVersion,
		ID:                    id,
		Name:                  "Demo " + id,
		Description:           "demo plugin",
		Version:               version,
		EucliBoxCompatibility: types.EucliBoxCompatibility{MinimumVersion: minimum, MaximumVersionExclusive: maximum},
		Hosting:               onDemandHosting(),
		Binaries:              []types.SystemPluginBinary{{GOOS: "windows", GOARCH: "amd64", Path: binaryPath}},
		Capabilities: []types.SystemPluginCapability{{
			Type:       systemplugin.CapabilityPlaceholderValues,
			Interfaces: []types.SystemPluginPlaceholderInterface{{ID: "value", DefaultName: "demo value", Description: "demo"}},
		}},
	}
	manifestPayload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	files := map[string][]byte{
		"manifest.json": manifestPayload,
		"config.json":   []byte("{}\n"),
		"stub.json":     []byte("{}\n"),
		binaryPath:      binaryPayload,
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

// TestImportPluginPackageInstallsMinimalLocalPackage 纯本地最小插件包
// 直接导入并完成停机、准备、探测与切换。
func TestImportPluginPackageInstallsMinimalLocalPackage(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	contentDir := buildImportPluginContents(t, "local-demo", "0.1.0", "0.1.0", "0.2.0")
	state, err := fixture.system.ImportPluginPackage(context.Background(), writeImportPluginArchive(t, contentDir))
	if err != nil {
		t.Fatalf("ImportPluginPackage() error = %v", err)
	}
	if isTerminalArtifactStatus(state.Status) {
		t.Fatalf("ImportPluginPackage() returned terminal state = %#v", state)
	}
	final := fixture.waitPluginTerminal(t, "local-demo")
	if final.Status != types.ArtifactStatusActive || final.CurrentVersion != "0.1.0" {
		t.Fatalf("final state = %#v", final)
	}
	plugins, err := fixture.system.ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("ListPlugins() error = %v", err)
	}
	found := false
	for _, plugin := range plugins {
		if plugin.ID == "local-demo" && plugin.Version == "0.1.0" && plugin.Installed {
			found = true
		}
	}
	if !found {
		t.Fatalf("imported plugin not listed: %#v", plugins)
	}
}

// TestImportPluginPackageRejectsPackageWithoutManifest 缺身份声明的包同步拒绝。
func TestImportPluginPackageRejectsPackageWithoutManifest(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	contentDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(contentDir, "README.md"), []byte("nothing here"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	_, err := fixture.system.ImportPluginPackage(context.Background(), writeImportPluginArchive(t, contentDir))
	if err == nil || !strings.Contains(err.Error(), "身份声明") {
		t.Fatalf("ImportPluginPackage() error = %v", err)
	}
}

// TestImportPluginPackageRejectsIncompatiblePackage 兼容声明不满足时同步拒绝并给出原因。
func TestImportPluginPackageRejectsIncompatiblePackage(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	contentDir := buildImportPluginContents(t, "local-demo", "0.1.0", "0.9.0", "1.0.0")
	state, err := fixture.system.ImportPluginPackage(context.Background(), writeImportPluginArchive(t, contentDir))
	if err != nil {
		t.Fatalf("ImportPluginPackage() error = %v", err)
	}
	if state.Status != types.ArtifactStatusBlocked || state.Error.Code != types.ArtifactErrorCompatibility {
		t.Fatalf("state = %#v", state)
	}
	if state.Error.Message == "" {
		t.Fatal("missing compatibility reason")
	}
	if _, err := os.Stat(filepath.Join(fixture.sourceDir, "local-demo")); !os.IsNotExist(err) {
		t.Fatal("incompatible package left program artifacts")
	}
}
