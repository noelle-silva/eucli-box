package systemplugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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

func TestResolvePlaceholderValuesRunsPluginRequestsInParallel(t *testing.T) {
	root := t.TempDir()
	markerFile := filepath.Join(root, "marker.txt")
	waitScript, markScript := parallelTestScripts(markerFile)
	waitDir := writeTestScriptPlugin(t, root, "wait-plugin", `{"status":"success","values":{"value":"wait"}}`, waitScript)
	writeTestManifest(t, waitDir, testPluginManifest("wait-plugin", "等待插件", types.SystemPluginLifecycleOnDemand))
	markDir := writeTestScriptPlugin(t, root, "mark-plugin", `{"status":"success","values":{"value":"mark"}}`, markScript)
	writeTestManifest(t, markDir, testPluginManifest("mark-plugin", "标记插件", types.SystemPluginLifecycleOnDemand))
	system, err := NewSystem(Config{SourceDir: root, DataDir: filepath.Join(root, "data"), Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	values, problems := system.ResolvePlaceholderValues(context.Background())
	if len(problems) != 0 {
		t.Fatalf("ResolvePlaceholderValues() problems = %#v", problems)
	}
	got := valueTexts(values)
	if strings.Join(got, ",") != "mark,wait" {
		t.Fatalf("ResolvePlaceholderValues() values = %#v", values)
	}
}

func TestResolvePlaceholderValuesKeepsSuccessfulPluginWhenAnotherFails(t *testing.T) {
	root := t.TempDir()
	failDir := writeTestPlugin(t, root, "fail-plugin", `{"status":"failed","error":"broken"}`)
	writeTestManifest(t, failDir, testPluginManifest("fail-plugin", "失败插件", types.SystemPluginLifecycleOnDemand))
	successDir := writeTestPlugin(t, root, "success-plugin", `{"status":"success","values":{"value":"ok"}}`)
	writeTestManifest(t, successDir, testPluginManifest("success-plugin", "成功插件", types.SystemPluginLifecycleOnDemand))
	system, err := NewSystem(Config{SourceDir: root, DataDir: filepath.Join(root, "data"), Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	values, problems := system.ResolvePlaceholderValues(context.Background())
	if len(values) != 1 || values[0].Value != "ok" {
		t.Fatalf("ResolvePlaceholderValues() values = %#v", values)
	}
	if len(problems) != 1 || problems[0].Name != "value" || problems[0].Type != types.PlaceholderProblemPluginFailed {
		t.Fatalf("ResolvePlaceholderValues() problems = %#v", problems)
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

func testPluginManifest(pluginID string, pluginName string, lifecycleType string) map[string]any {
	return map[string]any{
		"id":            pluginID,
		"name":          pluginName,
		"description":   pluginName,
		"version":       "0.1.0",
		"lifecycleType": lifecycleType,
		"binaries": []map[string]string{{
			"goos":   runtime.GOOS,
			"goarch": runtime.GOARCH,
			"path":   testPluginScriptName(),
		}},
		"placeholderInterfaces": []map[string]string{{
			"id":          "value",
			"defaultName": "value",
			"description": "值",
		}},
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

func writeTestScriptPlugin(t *testing.T, root string, pluginID string, response string, script string) string {
	t.Helper()
	pluginDir := filepath.Join(root, pluginID)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("Mkdir plugin dir error = %v", err)
	}
	writeTestPluginResponse(t, pluginDir, response)
	if err := os.WriteFile(filepath.Join(pluginDir, testPluginScriptName()), []byte(script), 0o755); err != nil {
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

func parallelTestScripts(markerFile string) (string, string) {
	if runtime.GOOS == "windows" {
		marker := strings.ReplaceAll(markerFile, `\`, `\\`)
		waitScript := "@echo off\r\nfor /L %%i in (1,1,20) do (\r\n  if exist \"" + marker + "\" goto found\r\n  ping -n 2 127.0.0.1 > nul\r\n)\r\necho {\"status\":\"failed\",\"error\":\"marker missing\"}\r\nexit /b 0\r\n:found\r\ntype response.json\r\n"
		markScript := "@echo off\r\necho ready>\"" + marker + "\"\r\ntype response.json\r\n"
		return waitScript, markScript
	}
	marker := strings.ReplaceAll(markerFile, `'`, `'\\''`)
	waitScript := "#!/bin/sh\ni=0\nwhile [ $i -lt 50 ]; do\n  if [ -f '" + marker + "' ]; then\n    cat response.json\n    exit 0\n  fi\n  i=$((i + 1))\n  sleep 0.1\ndone\nprintf '%s\\n' '{\"status\":\"failed\",\"error\":\"marker missing\"}'\n"
	markScript := "#!/bin/sh\nprintf '%s\\n' ready > '" + marker + "'\ncat response.json\n"
	return waitScript, markScript
}

func valueTexts(values []types.SystemPluginPlaceholderValue) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.Value)
	}
	sort.Strings(out)
	return out
}
