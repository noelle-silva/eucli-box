package systemplugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestDisablePluginPausesAndEnableResumesResolution(t *testing.T) {
	root := t.TempDir()
	directory := writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("switch-plugin", "开关插件", onDemandHosting()),
		behavior: map[string]any{"responseFile": "response.json"},
	})
	if err := os.WriteFile(filepath.Join(directory, "response.json"), []byte(`{"status":"success","values":{"value":"on"}}`), 0o644); err != nil {
		t.Fatalf("Write response error = %v", err)
	}
	system := newTestSystem(t, root)

	values, problems := system.ResolvePlaceholderValues(context.Background(), []types.SystemPluginPlaceholderSource{testSource("switch-plugin", "value", "value")})
	if len(problems) != 0 || len(values) != 1 || values[0].Value != "on" {
		t.Fatalf("values = %#v, problems = %#v", values, problems)
	}
	if _, err := system.DisablePlugin(context.Background(), "switch-plugin"); err != nil {
		t.Fatalf("DisablePlugin() error = %v", err)
	}
	plugins, err := system.ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("ListPlugins() error = %v", err)
	}
	if len(plugins) != 1 || plugins[0].Enabled {
		t.Fatalf("ListPlugins() = %#v", plugins)
	}
	statePayload, err := os.ReadFile(filepath.Join(root, "data", "switch-plugin", "state.json"))
	if err != nil || !strings.Contains(string(statePayload), `"enabled": false`) {
		t.Fatalf("state file = %q, error = %v", string(statePayload), err)
	}
	if err := os.WriteFile(filepath.Join(directory, "response.json"), []byte(`{"status":"success","values":{"value":"off"}}`), 0o644); err != nil {
		t.Fatalf("Rewrite response error = %v", err)
	}
	values, problems = system.ResolvePlaceholderValues(context.Background(), []types.SystemPluginPlaceholderSource{testSource("switch-plugin", "value", "value")})
	if len(values) != 0 || len(problems) != 1 || problems[0].Type != types.PlaceholderProblemPluginDisabled {
		t.Fatalf("disabled values = %#v, problems = %#v", values, problems)
	}
	if _, err := system.EnablePlugin(context.Background(), "switch-plugin"); err != nil {
		t.Fatalf("EnablePlugin() error = %v", err)
	}
	values, problems = system.ResolvePlaceholderValues(context.Background(), []types.SystemPluginPlaceholderSource{testSource("switch-plugin", "value", "value")})
	if len(problems) != 0 || len(values) != 1 || values[0].Value != "off" {
		t.Fatalf("enabled values = %#v, problems = %#v", values, problems)
	}
}

func TestDisabledStatePersistsAcrossRestart(t *testing.T) {
	root := t.TempDir()
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("persist-plugin", "持久开关插件", onDemandHosting()),
		behavior: map[string]any{"values": map[string]string{"value": "must-not-run"}},
	})
	system := newTestSystem(t, root)
	if _, err := system.DisablePlugin(context.Background(), "persist-plugin"); err != nil {
		t.Fatalf("DisablePlugin() error = %v", err)
	}
	if err := system.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	restarted := newTestSystem(t, root)
	values, problems := restarted.ResolvePlaceholderValues(context.Background(), []types.SystemPluginPlaceholderSource{testSource("persist-plugin", "value", "value")})
	if len(values) != 0 || len(problems) != 1 || problems[0].Type != types.PlaceholderProblemPluginDisabled {
		t.Fatalf("values = %#v, problems = %#v", values, problems)
	}
}

func TestDisablePluginStopsResidentSessionAndStartSkipsDisabled(t *testing.T) {
	root := t.TempDir()
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("resident-switch", "常驻开关插件", residentHosting(types.SystemPluginStartBoot)),
		behavior: map[string]any{"values": map[string]string{"value": "resident"}},
	})
	system := newTestSystem(t, root)
	if err := system.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !waitForSessionCount(system, "resident-switch", 1) {
		t.Fatalf("常驻会话没有被拉起")
	}
	real := realSystemOf(system)
	real.mu.Lock()
	instance := real.residents["resident-switch"]
	real.mu.Unlock()
	if instance == nil {
		t.Fatalf("resident session missing")
	}
	if _, err := system.DisablePlugin(context.Background(), "resident-switch"); err != nil {
		t.Fatalf("DisablePlugin() error = %v", err)
	}
	if !waitForSessionCount(system, "resident-switch", 0) {
		t.Fatalf("停用后会话仍然存在")
	}
	select {
	case <-instance.done:
	case <-time.After(5 * time.Second):
		t.Fatalf("停用后进程没有真实退出")
	}
	if err := system.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	restarted := newTestSystem(t, root)
	if err := restarted.Start(context.Background()); err != nil {
		t.Fatalf("restarted Start() error = %v", err)
	}
	if hasResidentSession(restarted, "resident-switch") {
		t.Fatalf("停用插件在重启后被拉起")
	}
}

func TestDisableAndEnableRejectedWhileUpdating(t *testing.T) {
	root := t.TempDir()
	writeTestPlugin(t, root, testPluginSpec{manifest: testManifest("busy-plugin", "忙碌插件", onDemandHosting())})
	system := newTestSystem(t, root)
	real := realSystemOf(system)
	activity := real.activityFor("busy-plugin")
	if code := activity.beginUpdate("operation-test", func() {}, time.Second); code != "" {
		t.Fatalf("beginUpdate() = %q", code)
	}
	defer func() {
		activity.endUpdate()
		activity.finishUpdate()
	}()
	if _, err := system.DisablePlugin(context.Background(), "busy-plugin"); err == nil {
		t.Fatalf("expected DisablePlugin to be rejected while updating")
	}
	if _, err := system.EnablePlugin(context.Background(), "busy-plugin"); err == nil {
		t.Fatalf("expected EnablePlugin to be rejected while updating")
	}
}

func TestAvailablePlaceholderInterfacesMarksDisabledPlugins(t *testing.T) {
	root := t.TempDir()
	writeTestPlugin(t, root, testPluginSpec{manifest: testManifest("marks-plugin", "标记插件", onDemandHosting())})
	system := newTestSystem(t, root)
	if _, err := system.DisablePlugin(context.Background(), "marks-plugin"); err != nil {
		t.Fatalf("DisablePlugin() error = %v", err)
	}
	interfaces, err := system.AvailablePlaceholderInterfaces(context.Background(), types.PlaceholderLibrary{})
	if err != nil {
		t.Fatalf("AvailablePlaceholderInterfaces() error = %v", err)
	}
	if len(interfaces) != 1 || !interfaces[0].Disabled || interfaces[0].InterfaceID != "value" {
		t.Fatalf("interfaces = %#v", interfaces)
	}
}

func TestSavePluginUserConfigOnDisabledResidentPluginDoesNotNotify(t *testing.T) {
	root := t.TempDir()
	track := filepath.Join(root, "track.jsonl")
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("quiet-plugin", "静默插件", residentHosting(types.SystemPluginStartBoot)),
		behavior: map[string]any{"trackFile": track},
	})
	if err := os.MkdirAll(filepath.Join(root, "data", "quiet-plugin"), 0o755); err != nil {
		t.Fatalf("Mkdir state dir error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "data", "quiet-plugin", "state.json"), []byte("{\"enabled\":false}\n"), 0o644); err != nil {
		t.Fatalf("Write state error = %v", err)
	}
	system := newTestSystem(t, root)
	if _, err := system.SavePluginUserConfig(context.Background(), "quiet-plugin", types.SystemPluginUserConfig{UserConfig: map[string]any{"mode": "fast"}}); err != nil {
		t.Fatalf("SavePluginUserConfig() error = %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if len(readTrackEntries(t, track)) != 0 {
		t.Fatalf("停用插件不应收到配置通知：%#v", readTrackEntries(t, track))
	}
}

// 启动时机是托管参数：随启动的常驻插件在 Start 时拉起，惰性常驻与按需插件不预启动。
func TestStartLaunchesBootResidentPluginsOnly(t *testing.T) {
	root := t.TempDir()
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("boot-plugin", "随启动插件", residentHosting(types.SystemPluginStartBoot)),
		behavior: map[string]any{"values": map[string]string{"value": "boot"}},
	})
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("lazy-plugin", "惰性常驻插件", residentHosting(types.SystemPluginStartLazy)),
		behavior: map[string]any{"values": map[string]string{"value": "lazy"}},
	})
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("demand-plugin", "按需插件", onDemandHosting()),
		behavior: map[string]any{"values": map[string]string{"value": "demand"}},
	})
	system := newTestSystem(t, root)
	if err := system.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !waitForSessionCount(system, "boot-plugin", 1) {
		t.Fatalf("随启动插件没有被拉起")
	}
	if hasResidentSession(system, "lazy-plugin") || hasResidentSession(system, "demand-plugin") {
		t.Fatalf("非随启动插件被预启动了")
	}
}

// 惰性常驻插件首次使用时启动，之后复用同一条通道。
func TestLazyResidentPluginStartsOnFirstUseAndIsReused(t *testing.T) {
	root := t.TempDir()
	track := filepath.Join(root, "track.jsonl")
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("lazy-resident", "惰性常驻", residentHosting(types.SystemPluginStartLazy)),
		behavior: map[string]any{"trackFile": track, "values": map[string]string{"value": "lazy"}},
	})
	system := newTestSystem(t, root)
	for index := 0; index < 2; index++ {
		values, problems := system.ResolvePlaceholderValues(context.Background(), []types.SystemPluginPlaceholderSource{testSource("lazy-resident", "value", "value")})
		if len(problems) != 0 || len(values) != 1 {
			t.Fatalf("values = %#v, problems = %#v", values, problems)
		}
	}
	if len(trackEntriesOfKind(t, track, "hello")) != 1 {
		t.Fatalf("常驻通道没有被复用：%#v", readTrackEntries(t, track))
	}
	if len(trackEntriesOfKind(t, track, "invoke")) != 2 {
		t.Fatalf("调用次数 = %#v", readTrackEntries(t, track))
	}
}
