package datastorage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// newToolConfigTestSystem 建立带一个开发态工具的数据存储系统：
// 工具定义写入 tool-bodies 目录，用户设置与配置区由后续动作写入。
func newToolConfigTestSystem(t *testing.T) *system {
	t.Helper()
	system := newTestSystem(t)
	tool := types.ToolDefinition{ID: "ai-image", Name: "ai-image", Description: "画图", Version: "0.1.0", Type: "local", BodyDirectory: ".", DefaultInvocationMode: types.ToolInvocationModeSync, Binaries: []types.ToolBinary{{GOOS: "windows", GOARCH: "amd64", Path: "binary/windows-amd64/ai-image.exe"}}}
	if err := system.SaveTool(context.Background(), tool); err != nil {
		t.Fatalf("SaveTool() error = %v", err)
	}
	return system
}

func TestToolConfigFilesRoundTrip(t *testing.T) {
	system := newToolConfigTestSystem(t)
	ctx := context.Background()

	files, err := system.LoadToolConfigFiles(ctx, "ai-image")
	if err != nil {
		t.Fatalf("LoadToolConfigFiles() error = %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("empty config expected, got %#v", files)
	}

	settings := types.ToolUserSettings{UserConfig: map[string]any{}, ConfigFiles: []types.ToolConfigFileWrite{
		{Path: "providers.json", Content: `{"defaultProvider":"demo"}`},
		{Path: "adapters/custom.json", Content: `{"id":"custom"}`},
	}}
	if _, err := system.SaveToolUserSettings(ctx, "ai-image", settings); err != nil {
		t.Fatalf("SaveToolUserSettings() error = %v", err)
	}
	files, err = system.LoadToolConfigFiles(ctx, "ai-image")
	if err != nil {
		t.Fatalf("LoadToolConfigFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 config files, got %#v", files)
	}
	if files[0].Path != "adapters/custom.json" || files[1].Path != "providers.json" {
		t.Fatalf("unexpected order: %#v", files)
	}

	// 文件内容不写进 settings.json。
	dataDir, err := system.paths.toolDataDir("ai-image")
	if err != nil {
		t.Fatalf("toolDataDir() error = %v", err)
	}
	settingsPath := filepath.Join(dataDir, "settings.json")
	payload, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if strings.Contains(string(payload), "defaultProvider") {
		t.Fatalf("settings.json must not carry config file content: %s", payload)
	}

	// 删除意图生效。
	settings.ConfigFiles = []types.ToolConfigFileWrite{{Path: "providers.json", Deleted: true}}
	if _, err := system.SaveToolUserSettings(ctx, "ai-image", settings); err != nil {
		t.Fatalf("SaveToolUserSettings() delete error = %v", err)
	}
	files, err = system.LoadToolConfigFiles(ctx, "ai-image")
	if err != nil {
		t.Fatalf("LoadToolConfigFiles() error = %v", err)
	}
	if len(files) != 1 || files[0].Path != "adapters/custom.json" {
		t.Fatalf("delete intent not applied: %#v", files)
	}
}

func TestToolConfigFileWritesRejectEscapingPaths(t *testing.T) {
	system := newToolConfigTestSystem(t)
	ctx := context.Background()
	for _, path := range []string{"../escape.json", "a/../../b.json", "/abs.json", "C:/abs.json"} {
		settings := types.ToolUserSettings{UserConfig: map[string]any{}, ConfigFiles: []types.ToolConfigFileWrite{{Path: path, Content: "{}"}}}
		if _, err := system.SaveToolUserSettings(ctx, "ai-image", settings); err == nil {
			t.Fatalf("path %q must be rejected", path)
		}
	}
}

func TestToolConfigFileWritesRejectOversizeContent(t *testing.T) {
	system := newToolConfigTestSystem(t)
	oversize := strings.Repeat("a", types.ToolConfigFileMaxBytes+1)
	settings := types.ToolUserSettings{UserConfig: map[string]any{}, ConfigFiles: []types.ToolConfigFileWrite{{Path: "big.json", Content: oversize}}}
	if _, err := system.SaveToolUserSettings(context.Background(), "ai-image", settings); err == nil {
		t.Fatal("oversize content must be rejected")
	}
}

func TestLoadToolConfigFilesRejectsOversizeExistingFile(t *testing.T) {
	system := newToolConfigTestSystem(t)
	dataDir, err := system.paths.toolDataDir("ai-image")
	if err != nil {
		t.Fatalf("toolDataDir() error = %v", err)
	}
	dir := filepath.Join(dataDir, types.ToolConfigDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.json"), []byte(strings.Repeat("a", types.ToolConfigFileMaxBytes+1)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := system.LoadToolConfigFiles(context.Background(), "ai-image"); err == nil {
		t.Fatal("oversize existing file must be rejected")
	}
}
