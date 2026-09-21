package systemplugin

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestDisablePluginPausesAndEnableResumesResolution(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	pluginDir := writeTestPlugin(t, root, "switch-plugin", `{"status":"success","values":{"value":"on"}}`)
	writeTestManifest(t, pluginDir, testPluginManifest("switch-plugin", "开关插件", types.SystemPluginLifecycleOnDemand))
	created, err := NewSystem(Config{SourceDir: root, DataDir: dataDir, Timeout: testPluginTimeout()})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	t.Cleanup(func() { _ = created.Shutdown(context.Background()) })

	plugins, err := created.ListPlugins(context.Background())
	if err != nil || len(plugins) != 1 || !plugins[0].Enabled {
		t.Fatalf("ListPlugins() = %#v, err = %v", plugins, err)
	}

	view, err := created.DisablePlugin(context.Background(), "switch-plugin")
	if err != nil || view.Enabled {
		t.Fatalf("DisablePlugin() view = %#v, err = %v", view, err)
	}
	payload, err := os.ReadFile(filepath.Join(dataDir, "switch-plugin", "state.json"))
	if err != nil || !strings.Contains(string(payload), `"enabled": false`) {
		t.Fatalf("state.json = %q, err = %v", string(payload), err)
	}

	// 停用后修改插件响应：再次解析必须既不取值也不执行插件，而是明确上报已停用。
	writeTestPluginResponse(t, pluginDir, `{"status":"success","values":{"value":"off"}}`)
	values, problems := created.ResolvePlaceholderValues(context.Background())
	if len(values) != 0 {
		t.Fatalf("ResolvePlaceholderValues() values = %#v", values)
	}
	if len(problems) != 1 || problems[0].Type != types.PlaceholderProblemPluginDisabled {
		t.Fatalf("ResolvePlaceholderValues() problems = %#v", problems)
	}

	view, err = created.EnablePlugin(context.Background(), "switch-plugin")
	if err != nil || !view.Enabled {
		t.Fatalf("EnablePlugin() view = %#v, err = %v", view, err)
	}
	values, problems = created.ResolvePlaceholderValues(context.Background())
	if len(problems) != 0 || len(values) != 1 || values[0].Value != "off" {
		t.Fatalf("ResolvePlaceholderValues() after enable values = %#v, problems = %#v", values, problems)
	}
}

func TestDisabledStatePersistsAcrossRestart(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	pluginDir := writeTestPlugin(t, root, "switch-plugin", `{"status":"success","values":{"value":"on"}}`)
	writeTestManifest(t, pluginDir, testPluginManifest("switch-plugin", "开关插件", types.SystemPluginLifecycleOnDemand))
	first, err := NewSystem(Config{SourceDir: root, DataDir: dataDir, Timeout: testPluginTimeout()})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	if _, err := first.DisablePlugin(context.Background(), "switch-plugin"); err != nil {
		t.Fatalf("DisablePlugin() error = %v", err)
	}
	if err := first.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	second, err := NewSystem(Config{SourceDir: root, DataDir: dataDir, Timeout: testPluginTimeout()})
	if err != nil {
		t.Fatalf("NewSystem() restart error = %v", err)
	}
	t.Cleanup(func() { _ = second.Shutdown(context.Background()) })
	plugins, err := second.ListPlugins(context.Background())
	if err != nil || len(plugins) != 1 || plugins[0].Enabled {
		t.Fatalf("ListPlugins() after restart = %#v, err = %v", plugins, err)
	}
	values, problems := second.ResolvePlaceholderValues(context.Background())
	if len(values) != 0 || len(problems) != 1 || problems[0].Type != types.PlaceholderProblemPluginDisabled {
		t.Fatalf("ResolvePlaceholderValues() after restart values = %#v, problems = %#v", values, problems)
	}
}

func TestDisableCachedHeartbeatPluginClearsCachedValues(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	pluginDir := writeTestPlugin(t, root, "cached-switch", `{"status":"success","values":{"value":"initial"}}`)
	manifest := testPluginManifest("cached-switch", "缓存开关插件", types.SystemPluginLifecycleCachedHeartbeat)
	manifest["heartbeatIntervalMs"] = 3600000
	writeTestManifest(t, pluginDir, manifest)
	created, err := NewSystem(Config{SourceDir: root, DataDir: dataDir, Timeout: testPluginTimeout()})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	if err := created.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = created.Shutdown(context.Background()) })
	values, problems := created.ResolvePlaceholderValues(context.Background())
	if len(problems) != 0 || len(values) != 1 || values[0].Value != "initial" {
		t.Fatalf("cached values before disable = %#v, problems = %#v", values, problems)
	}

	if _, err := created.DisablePlugin(context.Background(), "cached-switch"); err != nil {
		t.Fatalf("DisablePlugin() error = %v", err)
	}
	values, problems = created.ResolvePlaceholderValues(context.Background())
	if len(values) != 0 || len(problems) != 1 || problems[0].Type != types.PlaceholderProblemPluginDisabled {
		t.Fatalf("cached values after disable = %#v, problems = %#v", values, problems)
	}
	writeTestPluginResponse(t, pluginDir, `{"status":"success","values":{"value":"changed"}}`)

	// 重启后必须跳过停用插件：不得刷新缓存，也不得产生新值。
	second, err := NewSystem(Config{SourceDir: root, DataDir: dataDir, Timeout: testPluginTimeout()})
	if err != nil {
		t.Fatalf("NewSystem() restart error = %v", err)
	}
	if err := second.Start(context.Background()); err != nil {
		t.Fatalf("Start() restart error = %v", err)
	}
	t.Cleanup(func() { _ = second.Shutdown(context.Background()) })
	values, problems = second.ResolvePlaceholderValues(context.Background())
	if len(values) != 0 || len(problems) != 1 || problems[0].Type != types.PlaceholderProblemPluginDisabled {
		t.Fatalf("cached values after restart = %#v, problems = %#v", values, problems)
	}
}

func TestDisablePluginStopsPersistentProcessAndStartSkipsDisabled(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	writeTestBinaryPlugin(t, root, "persistent-switch", types.SystemPluginLifecyclePersistent)
	created, err := NewSystem(Config{SourceDir: root, DataDir: dataDir, Timeout: testPluginTimeout()})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	real := created.(*system)
	real.updateWaitTimeout = 5 * time.Second
	if err := created.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = created.Shutdown(context.Background()) })
	real.mu.Lock()
	process := real.persistent["persistent-switch"]
	real.mu.Unlock()
	if process == nil {
		t.Fatal("persistent process did not start before disable")
	}

	if _, err := created.DisablePlugin(context.Background(), "persistent-switch"); err != nil {
		t.Fatalf("DisablePlugin() error = %v", err)
	}
	real.mu.Lock()
	_, registered := real.persistent["persistent-switch"]
	real.mu.Unlock()
	if registered {
		t.Fatal("persistent process is still registered after disable")
	}
	if process.cmd.ProcessState == nil {
		t.Fatal("persistent process did not exit after disable")
	}
	values, problems := created.ResolvePlaceholderValues(context.Background())
	if len(values) != 0 || len(problems) != 1 || problems[0].Type != types.PlaceholderProblemPluginDisabled {
		t.Fatalf("values after disable = %#v, problems = %#v", values, problems)
	}
	if err := created.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	// 重启后必须跳过停用插件，不启动长驻进程。
	second, err := NewSystem(Config{SourceDir: root, DataDir: dataDir, Timeout: testPluginTimeout()})
	if err != nil {
		t.Fatalf("NewSystem() restart error = %v", err)
	}
	if err := second.Start(context.Background()); err != nil {
		t.Fatalf("Start() restart error = %v", err)
	}
	t.Cleanup(func() { _ = second.Shutdown(context.Background()) })
	realSecond := second.(*system)
	realSecond.mu.Lock()
	_, registered = realSecond.persistent["persistent-switch"]
	realSecond.mu.Unlock()
	if registered {
		t.Fatal("disabled persistent plugin must not start after restart")
	}
}

func TestDisableAndEnableRejectedWhileUpdating(t *testing.T) {
	root := t.TempDir()
	pluginDir := writeTestPlugin(t, root, "switch-plugin", `{"status":"success","values":{"value":"on"}}`)
	writeTestManifest(t, pluginDir, testPluginManifest("switch-plugin", "开关插件", types.SystemPluginLifecycleOnDemand))
	created, err := NewSystem(Config{SourceDir: root, DataDir: filepath.Join(root, "data"), Timeout: testPluginTimeout()})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	t.Cleanup(func() { _ = created.Shutdown(context.Background()) })
	real := created.(*system)
	activity := real.activityFor("switch-plugin")
	if blocked := activity.beginUpdate("test-operation", func() {}, time.Second); blocked != "" {
		t.Fatalf("beginUpdate() blocked = %q", blocked)
	}
	defer func() {
		activity.endUpdate()
		activity.finishUpdate()
	}()

	if _, err := created.DisablePlugin(context.Background(), "switch-plugin"); err == nil {
		t.Fatal("DisablePlugin() must be rejected while updating")
	}
	if _, err := created.EnablePlugin(context.Background(), "switch-plugin"); err == nil {
		t.Fatal("EnablePlugin() must be rejected while updating")
	}
}

func TestAvailablePlaceholderInterfacesMarksDisabledPlugins(t *testing.T) {
	root := t.TempDir()
	pluginDir := writeTestPlugin(t, root, "switch-plugin", `{"status":"success","values":{"value":"on"}}`)
	writeTestManifest(t, pluginDir, testPluginManifest("switch-plugin", "开关插件", types.SystemPluginLifecycleOnDemand))
	created, err := NewSystem(Config{SourceDir: root, DataDir: filepath.Join(root, "data"), Timeout: testPluginTimeout()})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	t.Cleanup(func() { _ = created.Shutdown(context.Background()) })
	if _, err := created.DisablePlugin(context.Background(), "switch-plugin"); err != nil {
		t.Fatalf("DisablePlugin() error = %v", err)
	}
	interfaces, err := created.AvailablePlaceholderInterfaces(context.Background(), types.PlaceholderLibrary{})
	if err != nil {
		t.Fatalf("AvailablePlaceholderInterfaces() error = %v", err)
	}
	if len(interfaces) != 1 || !interfaces[0].Disabled {
		t.Fatalf("AvailablePlaceholderInterfaces() = %#v", interfaces)
	}
}

func TestSavePluginUserConfigOnDisabledCachedPluginDoesNotRefresh(t *testing.T) {
	root := t.TempDir()
	pluginDir := writeTestPlugin(t, root, "cached-switch", `{"status":"success","values":{"value":"initial"}}`)
	manifest := testPluginManifest("cached-switch", "缓存开关插件", types.SystemPluginLifecycleCachedHeartbeat)
	manifest["heartbeatIntervalMs"] = 3600000
	writeTestManifest(t, pluginDir, manifest)
	created, err := NewSystem(Config{SourceDir: root, DataDir: filepath.Join(root, "data"), Timeout: testPluginTimeout()})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	t.Cleanup(func() { _ = created.Shutdown(context.Background()) })
	if _, err := created.DisablePlugin(context.Background(), "cached-switch"); err != nil {
		t.Fatalf("DisablePlugin() error = %v", err)
	}
	writeTestPluginResponse(t, pluginDir, `{"status":"success","values":{"value":"changed"}}`)
	if _, err := created.SavePluginUserConfig(context.Background(), "cached-switch", types.SystemPluginUserConfig{UserConfig: map[string]any{"note": "x"}}); err != nil {
		t.Fatalf("SavePluginUserConfig() error = %v", err)
	}
	real := created.(*system)
	real.mu.Lock()
	_, cached := real.cachedValues["cached-switch"]
	real.mu.Unlock()
	if cached {
		t.Fatal("disabled cached plugin must not refresh cached values on config save")
	}
}

func writeTestBinaryPlugin(t *testing.T, root string, pluginID string, lifecycle string) string {
	t.Helper()
	pluginDir := filepath.Join(root, pluginID)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("Mkdir plugin dir error = %v", err)
	}
	executable := "plugin"
	if runtime.GOOS == "windows" {
		executable = "plugin.exe"
	}
	if err := os.WriteFile(filepath.Join(pluginDir, executable), buildPluginBinary(t, lifecycle), 0o755); err != nil {
		t.Fatalf("Write plugin binary error = %v", err)
	}
	manifest := testPluginManifest(pluginID, "二进制插件", lifecycle)
	manifest["binaries"] = []map[string]string{{
		"goos":   runtime.GOOS,
		"goarch": runtime.GOARCH,
		"path":   executable,
	}}
	writeTestManifest(t, pluginDir, manifest)
	return pluginDir
}
