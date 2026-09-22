package systemplugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/systemplugin"
	"eucli-box/pkg/types"
)

// 定向派发：只联系被引用占位符的所属插件，未引用的插件一个字都不问。
func TestResolvePlaceholderValuesOnlyContactsReferencedProviders(t *testing.T) {
	root := t.TempDir()
	alphaTrack := filepath.Join(root, "alpha-track.jsonl")
	betaTrack := filepath.Join(root, "beta-track.jsonl")
	gammaTrack := filepath.Join(root, "gamma-track.jsonl")
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("alpha-plugin", "甲插件", onDemandHosting()),
		behavior: map[string]any{"trackFile": alphaTrack, "values": map[string]string{"value": "alpha-value"}},
	})
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("beta-plugin", "乙插件", onDemandHosting()),
		behavior: map[string]any{"trackFile": betaTrack, "values": map[string]string{"value": "beta-value"}},
	})
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("gamma-plugin", "丙插件", onDemandHosting()),
		behavior: map[string]any{"trackFile": gammaTrack, "values": map[string]string{"value": "gamma-value"}},
	})
	system := newTestSystem(t, root)

	values, problems := system.ResolvePlaceholderValues(context.Background(), []types.SystemPluginPlaceholderSource{testSource("alpha-plugin", "value", "alpha")})
	if len(problems) != 0 {
		t.Fatalf("ResolvePlaceholderValues() problems = %#v", problems)
	}
	if len(values) != 1 || values[0].PluginID != "alpha-plugin" || values[0].Value != "alpha-value" || values[0].Name != "value" {
		t.Fatalf("ResolvePlaceholderValues() values = %#v", values)
	}
	if len(trackEntriesOfKind(t, alphaTrack, "invoke")) != 1 {
		t.Fatalf("alpha invoke entries = %#v", readTrackEntries(t, alphaTrack))
	}
	for _, path := range []string{betaTrack, gammaTrack} {
		if len(readTrackEntries(t, path)) != 0 {
			t.Fatalf("未被引用的插件被联系了：%s = %#v", path, readTrackEntries(t, path))
		}
	}
}

// 不同插件之间并行取值：等待插件依赖标记插件的文件，串行则无法完成。
func TestResolvePlaceholderValuesRunsPluginRequestsInParallel(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "marker.txt")
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("wait-plugin", "等待插件", onDemandHosting()),
		behavior: map[string]any{"waitForFile": marker, "values": map[string]string{"value": "wait"}},
	})
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("mark-plugin", "标记插件", onDemandHosting()),
		behavior: map[string]any{"createFile": marker, "values": map[string]string{"value": "mark"}},
	})
	system := newTestSystem(t, root)

	values, problems := system.ResolvePlaceholderValues(context.Background(), []types.SystemPluginPlaceholderSource{
		testSource("wait-plugin", "value", "wait"),
		testSource("mark-plugin", "value", "mark"),
	})
	if len(problems) != 0 {
		t.Fatalf("ResolvePlaceholderValues() problems = %#v", problems)
	}
	if len(values) != 2 {
		t.Fatalf("ResolvePlaceholderValues() values = %#v", values)
	}
	if values[0].PluginID != "wait-plugin" || values[1].PluginID != "mark-plugin" {
		t.Fatalf("取值顺序未保持来源顺序：%#v", values)
	}
}

// 单插件失败只影响该插件提供的值，不阻断其他插件。
func TestResolvePlaceholderValuesKeepsSuccessfulPluginWhenAnotherFails(t *testing.T) {
	root := t.TempDir()
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("fail-plugin", "失败插件", onDemandHosting()),
		behavior: map[string]any{"status": "failed", "error": "broken"},
	})
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("success-plugin", "成功插件", onDemandHosting()),
		behavior: map[string]any{"values": map[string]string{"value": "ok"}},
	})
	system := newTestSystem(t, root)

	values, problems := system.ResolvePlaceholderValues(context.Background(), []types.SystemPluginPlaceholderSource{
		testSource("fail-plugin", "value", "fail"),
		testSource("success-plugin", "value", "ok"),
	})
	if len(values) != 1 || values[0].Value != "ok" {
		t.Fatalf("ResolvePlaceholderValues() values = %#v", values)
	}
	if len(problems) != 1 || problems[0].Name != "value" || problems[0].Type != types.PlaceholderProblemPluginFailed {
		t.Fatalf("ResolvePlaceholderValues() problems = %#v", problems)
	}
}

func TestIncompatiblePluginRemainsVisibleAndDoesNotExecute(t *testing.T) {
	root := t.TempDir()
	track := filepath.Join(root, "track.jsonl")
	manifest := testManifest("future-plugin", "未来插件", onDemandHosting())
	manifest.EucliBoxCompatibility = types.EucliBoxCompatibility{MinimumVersion: "0.2.0", MaximumVersionExclusive: "0.3.0"}
	writeTestPlugin(t, root, testPluginSpec{manifest: manifest, behavior: map[string]any{"trackFile": track, "values": map[string]string{"value": "must-not-run"}}})
	system := newTestSystem(t, root)

	plugins, err := system.ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("ListPlugins() error = %v", err)
	}
	if len(plugins) != 1 || plugins[0].Status != types.SystemPluginStatusUnavailable || plugins[0].Compatibility.Compatible || !strings.Contains(plugins[0].StatusMessage, "不在所需范围") {
		t.Fatalf("ListPlugins() = %#v", plugins)
	}
	values, problems := system.ResolvePlaceholderValues(context.Background(), []types.SystemPluginPlaceholderSource{testSource("future-plugin", "value", "future")})
	if len(values) != 0 || len(problems) != 1 || problems[0].Type != types.PlaceholderProblemPluginFailed {
		t.Fatalf("values = %#v, problems = %#v", values, problems)
	}
	if len(readTrackEntries(t, track)) != 0 {
		t.Fatalf("不兼容插件被执行了：%#v", readTrackEntries(t, track))
	}
}

func TestInvalidPluginManifestRemainsVisibleWithoutInventedIdentity(t *testing.T) {
	root := t.TempDir()
	manifest := testManifest("invalid-plugin", "无效插件", onDemandHosting())
	manifest.ID = ""
	writeTestPlugin(t, root, testPluginSpec{directory: "invalid-plugin", manifest: manifest})
	system := newTestSystem(t, root)

	plugins, err := system.ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("ListPlugins() error = %v", err)
	}
	if len(plugins) != 1 || plugins[0].ID != "" || plugins[0].SourceID != "invalid-plugin" || plugins[0].Status != types.SystemPluginStatusUnavailable || !strings.Contains(plugins[0].StatusMessage, "id is required") {
		t.Fatalf("ListPlugins() = %#v", plugins)
	}
}

func TestUnsupportedProtocolVersionPluginIsUnavailable(t *testing.T) {
	root := t.TempDir()
	manifest := testManifest("newer-plugin", "新协议插件", onDemandHosting())
	manifest.ProtocolVersion = systemplugin.ProtocolVersion + 1
	writeTestPlugin(t, root, testPluginSpec{manifest: manifest})
	system := newTestSystem(t, root)

	plugins, err := system.ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("ListPlugins() error = %v", err)
	}
	if len(plugins) != 1 || plugins[0].Status != types.SystemPluginStatusUnavailable || !strings.Contains(plugins[0].StatusMessage, "protocolVersion") {
		t.Fatalf("ListPlugins() = %#v", plugins)
	}
}

func TestSavePluginUserConfigRejectsValuesOutsideDeclaredOptions(t *testing.T) {
	root := t.TempDir()
	manifest := testManifest("select-plugin", "选择插件", onDemandHosting())
	manifest.ConfigSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode": map[string]any{"type": "string", "enum": []any{"safe", "fast"}},
		},
	}
	writeTestPlugin(t, root, testPluginSpec{manifest: manifest})
	system := newTestSystem(t, root)

	if _, err := system.SavePluginUserConfig(context.Background(), "select-plugin", types.SystemPluginUserConfig{UserConfig: map[string]any{"mode": "safe"}}); err != nil {
		t.Fatalf("SavePluginUserConfig(safe) error = %v", err)
	}
	if _, err := system.SavePluginUserConfig(context.Background(), "select-plugin", types.SystemPluginUserConfig{UserConfig: map[string]any{"mode": "unsafe"}}); err == nil {
		t.Fatalf("expected enum validation error")
	}
}

// 配置变更通过控制通道送达常驻插件；插件自行决定刷新方式。
func TestSavePluginUserConfigNotifiesResidentPlugin(t *testing.T) {
	root := t.TempDir()
	track := filepath.Join(root, "track.jsonl")
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("resident-config", "常驻配置插件", residentHosting(types.SystemPluginStartBoot)),
		behavior: map[string]any{"trackFile": track, "values": map[string]string{"value": "resident"}},
	})
	system := newTestSystem(t, root)
	if err := system.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, err := system.SavePluginUserConfig(context.Background(), "resident-config", types.SystemPluginUserConfig{UserConfig: map[string]any{"mode": "fast"}}); err != nil {
		t.Fatalf("SavePluginUserConfig() error = %v", err)
	}
	entries := waitForTrackEntries(t, track, "config", 1)
	userConfig, _ := entries[len(entries)-1]["userConfig"].(map[string]any)
	if userConfig["mode"] != "fast" {
		t.Fatalf("config entries = %#v", entries)
	}
}

// 静态核对只回答事实：所属缺失、接口未声明、插件停用；不启动任何插件。
func TestPlaceholderSourceProblemsReportsStaticFacts(t *testing.T) {
	root := t.TempDir()
	track := filepath.Join(root, "track.jsonl")
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("idle-plugin", "空闲插件", onDemandHosting()),
		behavior: map[string]any{"trackFile": track},
	})
	disabledManifest := testManifest("disabled-plugin", "停用插件", onDemandHosting())
	writeTestPlugin(t, root, testPluginSpec{manifest: disabledManifest, behavior: map[string]any{"trackFile": track}})
	if err := os.MkdirAll(filepath.Join(root, "data", "disabled-plugin"), 0o755); err != nil {
		t.Fatalf("Mkdir state dir error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "data", "disabled-plugin", "state.json"), []byte("{\"enabled\":false}\n"), 0o644); err != nil {
		t.Fatalf("Write state error = %v", err)
	}
	system := newTestSystem(t, root)

	problems := system.PlaceholderSourceProblems(context.Background(), []types.SystemPluginPlaceholderSource{
		testSource("idle-plugin", "missing-interface", "missing"),
		testSource("disabled-plugin", "value", "disabled"),
		testSource("ghost-plugin", "value", "ghost"),
		testSource("idle-plugin", "value", "idle"),
	})
	if len(problems) != 3 {
		t.Fatalf("PlaceholderSourceProblems() = %#v", problems)
	}
	byName := problemTypes(problems)
	if byName["missing"] != types.PlaceholderProblemPluginFailed || byName["ghost"] != types.PlaceholderProblemPluginFailed || byName["value"] != types.PlaceholderProblemPluginDisabled {
		t.Fatalf("problems = %#v", problems)
	}
	if len(readTrackEntries(t, track)) != 0 {
		t.Fatalf("静态核对不应启动插件：%#v", readTrackEntries(t, track))
	}
}

// 握手把数据目录与配置交给插件，插件工作目录就是插件目录。
func TestSessionHelloCarriesDataDirectoryAndConfig(t *testing.T) {
	root := t.TempDir()
	track := filepath.Join(root, "track.jsonl")
	pluginDir := writeTestPlugin(t, root, testPluginSpec{
		manifest:      testManifest("hello-plugin", "握手插件", onDemandHosting()),
		defaultConfig: map[string]any{"fromDefault": "yes"},
		behavior:      map[string]any{"trackFile": track, "values": map[string]string{"value": "hello"}},
	})
	system := newTestSystem(t, root)
	if _, err := system.SavePluginUserConfig(context.Background(), "hello-plugin", types.SystemPluginUserConfig{UserConfig: map[string]any{"fromUser": "yes"}}); err != nil {
		t.Fatalf("SavePluginUserConfig() error = %v", err)
	}
	if _, problems := system.ResolvePlaceholderValues(context.Background(), []types.SystemPluginPlaceholderSource{testSource("hello-plugin", "value", "hello")}); len(problems) != 0 {
		t.Fatalf("ResolvePlaceholderValues() problems = %#v", problems)
	}
	entries := waitForTrackEntries(t, track, "hello", 1)
	entry := entries[len(entries)-1]
	if entry["dataDirectory"] != filepath.Join(root, "data", "hello-plugin") {
		t.Fatalf("dataDirectory = %#v", entry["dataDirectory"])
	}
	if entry["workingDirectory"] != pluginDir {
		t.Fatalf("workingDirectory = %#v, want %q", entry["workingDirectory"], pluginDir)
	}
	userConfig, _ := entry["userConfig"].(map[string]any)
	defaultConfig, _ := entry["defaultConfig"].(map[string]any)
	if userConfig["fromUser"] != "yes" || defaultConfig["fromDefault"] != "yes" {
		t.Fatalf("hello config = %#v", entry)
	}
}

func TestHandshakeValidationFailures(t *testing.T) {
	cases := []struct {
		name     string
		behavior map[string]any
	}{
		{name: "identity", behavior: map[string]any{"handshake": "plugin-id-mismatch"}},
		{name: "capability", behavior: map[string]any{"capabilities": []map[string]any{{"type": "placeholder-values", "interfaces": []string{"other"}}}}},
		{name: "no-ready", behavior: map[string]any{"noReady": true}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestPlugin(t, root, testPluginSpec{manifest: testManifest("strict-plugin", "严格插件", onDemandHosting()), behavior: testCase.behavior})
			system, err := NewSystem(Config{SourceDir: root, DataDir: filepath.Join(root, "data"), Timeout: 500 * time.Millisecond})
			if err != nil {
				t.Fatalf("NewSystem() error = %v", err)
			}
			t.Cleanup(func() { _ = system.Shutdown(context.Background()) })
			values, problems := system.ResolvePlaceholderValues(context.Background(), []types.SystemPluginPlaceholderSource{testSource("strict-plugin", "value", "strict")})
			if len(values) != 0 || len(problems) != 1 || problems[0].Type != types.PlaceholderProblemPluginFailed {
				t.Fatalf("values = %#v, problems = %#v", values, problems)
			}
			if failure := realSystemOf(system).getFailure("strict-plugin"); failure == "" {
				t.Fatalf("expected failure fact for handshake case")
			}
		})
	}
}

// 调用超时是致命失败：通道被丢弃，后续请求会重新建立。
func TestInvokeTimeoutDropsSession(t *testing.T) {
	root := t.TempDir()
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("slow-plugin", "慢插件", onDemandHosting()),
		behavior: map[string]any{"invokeDelayMs": 3000, "values": map[string]string{"value": "late"}},
	})
	system, err := NewSystem(Config{SourceDir: root, DataDir: filepath.Join(root, "data"), Timeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	t.Cleanup(func() { _ = system.Shutdown(context.Background()) })

	values, problems := system.ResolvePlaceholderValues(context.Background(), []types.SystemPluginPlaceholderSource{testSource("slow-plugin", "value", "slow")})
	if len(values) != 0 || len(problems) != 1 || problems[0].Type != types.PlaceholderProblemPluginFailed {
		t.Fatalf("values = %#v, problems = %#v", values, problems)
	}
	if failure := realSystemOf(system).getFailure("slow-plugin"); !strings.Contains(failure, "超时") {
		t.Fatalf("failure = %q", failure)
	}
}

// 事件经控制通道被宿主接收；宿主只做转发，不做持久化。
func TestPluginEventsReachObserver(t *testing.T) {
	root := t.TempDir()
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("event-plugin", "事件插件", residentHosting(types.SystemPluginStartBoot)),
		behavior: map[string]any{
			"values":    map[string]string{"value": "event"},
			"emitEvent": map[string]any{"name": "ready-notice", "payload": map[string]any{"state": "warm"}},
		},
	})
	events := make(chan string, 8)
	system, err := NewSystem(Config{
		SourceDir: root,
		DataDir:   filepath.Join(root, "data"),
		Timeout:   testPluginTimeout(),
		OnEvent: func(pluginID string, event string, _ map[string]any) {
			events <- pluginID + ":" + event
		},
	})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	t.Cleanup(func() { _ = system.Shutdown(context.Background()) })
	if err := system.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case event := <-events:
		if event != "event-plugin:ready-notice" {
			t.Fatalf("event = %q", event)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("没有收到插件事件")
	}
}
