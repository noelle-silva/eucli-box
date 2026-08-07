package datastorage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

func newManagedTestSystem(t *testing.T) (*system, string) {
	t.Helper()
	root := t.TempDir()
	programRoot := filepath.Join(t.TempDir(), "program", "tools")
	created, err := NewSystem(Config{RootDir: root, ToolBodiesRoot: programRoot})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	system, ok := created.(*system)
	if !ok {
		t.Fatalf("unexpected system type %T", created)
	}
	if err := system.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	return system, programRoot
}

func installManagedTool(t *testing.T, programRoot string, id string, version string) {
	t.Helper()
	contentDir := t.TempDir()
	binaryPath := filepath.ToSlash(filepath.Join("binary", "windows-amd64", id+".exe"))
	definition := types.ToolDefinition{
		ID:                    id,
		Name:                  "Demo " + id,
		Description:           "demo tool",
		Version:               version,
		EucliBoxCompatibility: types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		DefaultInvocationMode: "sync",
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
		binaryPath:        []byte("tool-binary"),
		"README.md":       []byte("# demo\n"),
		"CHANGELOG.md":    []byte("## v1\n"),
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
	product := types.ReleaseProductRecord{
		SchemaVersion:  release.ReleaseManifestSchemaVersion,
		Artifact:       types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: id},
		Version:        version,
		Platform:       types.ReleasePlatformWindowsX64,
		OfficialSource: "https://github.com/noelle-silva/eucli-box-ai-tools",
		Compatibility:  &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		Source:         types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
	}
	productPayload, err := json.MarshalIndent(product, "", "  ")
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "release-product.json"), productPayload, 0o644); err != nil {
		t.Fatalf("write product: %v", err)
	}
	fileRecords, err := release.CollectFileRecords(contentDir)
	if err != nil {
		t.Fatalf("collect files: %v", err)
	}
	store, err := release.NewProgramStore(filepath.Join(programRoot, id), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: id})
	if err != nil {
		t.Fatalf("NewProgramStore() error = %v", err)
	}
	prepared, err := store.PrepareVersion(context.Background(), contentDir, product, fileRecords)
	if err != nil {
		t.Fatalf("PrepareVersion() error = %v", err)
	}
	if err := store.Activate(context.Background(), prepared, ""); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
}

func TestManagedToolsNotInstalledStayOutOfList(t *testing.T) {
	system, programRoot := newManagedTestSystem(t)
	if err := os.MkdirAll(filepath.Join(programRoot, "ghost"), 0o755); err != nil {
		t.Fatalf("mkdir ghost: %v", err)
	}
	tools, err := system.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("tools = %#v", tools)
	}
	if _, err := system.LoadTool(context.Background(), "ghost"); err == nil {
		t.Fatal("LoadTool(ghost) error = nil")
	}
}

func TestManagedToolLoadsFromCurrentVersionDirectory(t *testing.T) {
	system, programRoot := newManagedTestSystem(t)
	installManagedTool(t, programRoot, "demo", "0.1.0")
	tools, err := system.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 1 || tools[0].ID != "demo" || tools[0].Version != "0.1.0" {
		t.Fatalf("tools = %#v", tools)
	}
	tool, err := system.LoadTool(context.Background(), "demo")
	if err != nil {
		t.Fatalf("LoadTool() error = %v", err)
	}
	if !strings.Contains(tool.BodyDirectory, filepath.Join("versions", "0.1.0")) {
		t.Fatalf("body directory = %s", tool.BodyDirectory)
	}
	if !strings.HasSuffix(tool.DataDirectory, filepath.Join("tool-data", "demo")) {
		t.Fatalf("data directory = %s", tool.DataDirectory)
	}
}

func TestManagedToolUserSettingsStayInToolData(t *testing.T) {
	system, programRoot := newManagedTestSystem(t)
	installManagedTool(t, programRoot, "demo", "0.1.0")
	updated, err := system.SaveToolUserSettings(context.Background(), "demo", types.ToolUserSettings{UserConfig: map[string]any{"mode": "fast"}})
	if err != nil {
		t.Fatalf("SaveToolUserSettings() error = %v", err)
	}
	if updated.UserConfig["mode"] != "fast" {
		t.Fatalf("user config = %#v", updated.UserConfig)
	}
	settingsPath := filepath.Join(system.paths.toolDataRoot(), "demo", "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings file missing: %v", err)
	}
	reloaded, err := system.LoadTool(context.Background(), "demo")
	if err != nil {
		t.Fatalf("LoadTool() error = %v", err)
	}
	if reloaded.UserConfig["mode"] != "fast" {
		t.Fatalf("reloaded user config = %#v", reloaded.UserConfig)
	}
}

func TestManagedToolRejectsSaveToolAsDevelopmentEntry(t *testing.T) {
	system, _ := newManagedTestSystem(t)
	err := system.SaveTool(context.Background(), types.ToolDefinition{ID: "demo"})
	if err == nil {
		t.Fatal("SaveTool() error = nil")
	}
}

func TestManagedToolBrokenCurrentRecordIsUnavailable(t *testing.T) {
	system, programRoot := newManagedTestSystem(t)
	if err := os.MkdirAll(filepath.Join(programRoot, "broken"), 0o755); err != nil {
		t.Fatalf("mkdir broken: %v", err)
	}
	if err := os.WriteFile(filepath.Join(programRoot, "broken", "current.json"), []byte(`{"broken":true}`), 0o644); err != nil {
		t.Fatalf("write current: %v", err)
	}
	tools, err := system.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 1 || tools[0].ID != "broken" || tools[0].Status != types.ToolAvailabilityUnavailable {
		t.Fatalf("tools = %#v", tools)
	}
}

func TestManagedToolIndexStaysInDataArea(t *testing.T) {
	system, programRoot := newManagedTestSystem(t)
	installManagedTool(t, programRoot, "demo", "0.1.0")
	if _, err := os.Stat(filepath.Join(programRoot, "index.json")); err == nil {
		t.Fatal("index.json written into program root")
	}
	if _, err := os.Stat(filepath.Join(system.paths.toolDataRoot(), "index.json")); err != nil {
		t.Fatalf("index.json missing in data area: %v", err)
	}
}
