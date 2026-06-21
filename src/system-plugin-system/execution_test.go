package systemplugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestCachedHeartbeatPluginUsesCachedValues(t *testing.T) {
	root := t.TempDir()
	pluginDir := writeTestPlugin(t, root, "cached-plugin", `{"status":"success","values":{"status":"initial"}}`)
	writeTestManifest(t, pluginDir, map[string]any{
		"id":                  "cached-plugin",
		"name":                "缓存插件",
		"description":         "提供缓存值",
		"version":             "0.1.0",
		"lifecycleType":       types.SystemPluginLifecycleCachedHeartbeat,
		"heartbeatIntervalMs": 3600000,
		"binaries": []map[string]string{{
			"goos":   runtime.GOOS,
			"goarch": runtime.GOARCH,
			"path":   testPluginScriptName(),
		}},
		"placeholderInterfaces": []map[string]string{{
			"id":          "status",
			"defaultName": "status",
			"description": "状态",
		}},
	})
	system, err := NewSystem(Config{SourceDir: root, DataDir: filepath.Join(root, "data"), Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	if err := system.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = system.Shutdown(context.Background()) })
	writeTestPluginResponse(t, pluginDir, `{"status":"success","values":{"status":"changed"}}`)
	values, problems := system.ResolvePlaceholderValues(context.Background())
	if len(problems) != 0 {
		t.Fatalf("ResolvePlaceholderValues() problems = %#v", problems)
	}
	if len(values) != 1 || values[0].Value != "initial" {
		t.Fatalf("ResolvePlaceholderValues() values = %#v", values)
	}
}

func TestCachedHeartbeatPluginRefreshesAfterConfigSave(t *testing.T) {
	root := t.TempDir()
	pluginDir := writeTestPlugin(t, root, "cached-plugin", `{"status":"success","values":{"status":"initial"}}`)
	writeTestManifest(t, pluginDir, map[string]any{
		"id":                  "cached-plugin",
		"name":                "缓存插件",
		"description":         "提供缓存值",
		"version":             "0.1.0",
		"lifecycleType":       types.SystemPluginLifecycleCachedHeartbeat,
		"heartbeatIntervalMs": 3600000,
		"binaries": []map[string]string{{
			"goos":   runtime.GOOS,
			"goarch": runtime.GOARCH,
			"path":   testPluginScriptName(),
		}},
		"placeholderInterfaces": []map[string]string{{
			"id":          "status",
			"defaultName": "status",
			"description": "状态",
		}},
	})
	system, err := NewSystem(Config{SourceDir: root, DataDir: filepath.Join(root, "data"), Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	if err := system.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = system.Shutdown(context.Background()) })
	writeTestPluginResponse(t, pluginDir, `{"status":"success","values":{"status":"changed"}}`)
	if _, err := system.SavePluginUserConfig(context.Background(), "cached-plugin", types.SystemPluginUserConfig{UserConfig: map[string]any{"enabled": true}}); err != nil {
		t.Fatalf("SavePluginUserConfig() error = %v", err)
	}
	values, problems := system.ResolvePlaceholderValues(context.Background())
	if len(problems) != 0 {
		t.Fatalf("ResolvePlaceholderValues() problems = %#v", problems)
	}
	if len(values) != 1 || values[0].Value != "changed" {
		t.Fatalf("ResolvePlaceholderValues() values = %#v", values)
	}
}

func writeTestManifest(t *testing.T, pluginDir string, manifest map[string]any) {
	t.Helper()
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("Marshal manifest error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("Write manifest error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "config.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("Write config error = %v", err)
	}
}

func writeTestPlugin(t *testing.T, root string, pluginID string, response string) string {
	t.Helper()
	pluginDir := filepath.Join(root, pluginID)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("Mkdir plugin dir error = %v", err)
	}
	writeTestPluginResponse(t, pluginDir, response)
	if runtime.GOOS == "windows" {
		payload := "@echo off\r\ntype response.json\r\n"
		if err := os.WriteFile(filepath.Join(pluginDir, testPluginScriptName()), []byte(payload), 0o755); err != nil {
			t.Fatalf("Write plugin script error = %v", err)
		}
		return pluginDir
	}
	payload := "#!/bin/sh\ncat response.json\n"
	if err := os.WriteFile(filepath.Join(pluginDir, testPluginScriptName()), []byte(payload), 0o755); err != nil {
		t.Fatalf("Write plugin script error = %v", err)
	}
	return pluginDir
}

func writeTestPluginResponse(t *testing.T, pluginDir string, response string) {
	t.Helper()
	response = strings.TrimSpace(response) + "\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "response.json"), []byte(response), 0o644); err != nil {
		t.Fatalf("Write response error = %v", err)
	}
}

func testPluginScriptName() string {
	if runtime.GOOS == "windows" {
		return "plugin.bat"
	}
	return "plugin.sh"
}
