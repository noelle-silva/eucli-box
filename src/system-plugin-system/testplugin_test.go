package systemplugin

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/systemplugin"
	"eucli-box/pkg/types"
)

//go:embed testdata/stub/main.go
var stubSource string

var stubExecutable string

func TestMain(m *testing.M) {
	directory, err := buildStubPlugin()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(directory)
	os.Exit(code)
}

// buildStubPlugin 每个测试进程只编译一次桩插件；桩独立手写协议，反向校验宿主实现。
func buildStubPlugin() (string, error) {
	directory, err := os.MkdirTemp("", "eucli-system-plugin-stub-")
	if err != nil {
		return "", err
	}
	source := filepath.Join(directory, "main.go")
	if err := os.WriteFile(source, []byte(stubSource), 0o644); err != nil {
		return "", err
	}
	executable := filepath.Join(directory, "stub")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", executable, source)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build stub plugin: %w\n%s", err, output)
	}
	stubExecutable = executable
	return directory, nil
}

type testPluginSpec struct {
	directory     string
	manifest      types.SystemPluginManifest
	defaultConfig map[string]any
	behavior      map[string]any
}

func testPlaceholderInterface(id string, defaultName string) types.SystemPluginPlaceholderInterface {
	return types.SystemPluginPlaceholderInterface{ID: id, DefaultName: defaultName, Description: defaultName + " description"}
}

func testManifest(pluginID string, name string, hosting types.SystemPluginHosting, interfaces ...types.SystemPluginPlaceholderInterface) types.SystemPluginManifest {
	if len(interfaces) == 0 {
		interfaces = []types.SystemPluginPlaceholderInterface{testPlaceholderInterface("value", "value")}
	}
	return types.SystemPluginManifest{
		ProtocolVersion:       systemplugin.ProtocolVersion,
		ID:                    pluginID,
		Name:                  name,
		Description:           name + " 用于测试",
		Version:               "0.1.0",
		EucliBoxCompatibility: types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		Hosting:               hosting,
		Capabilities:          []types.SystemPluginCapability{{Type: systemplugin.CapabilityPlaceholderValues, Interfaces: interfaces}},
	}
}

func onDemandHosting() types.SystemPluginHosting {
	return types.SystemPluginHosting{Start: types.SystemPluginStartLazy, Resident: false, Restart: types.SystemPluginRestartNever, StopTimeoutMs: 2000}
}

func residentHosting(start string) types.SystemPluginHosting {
	return types.SystemPluginHosting{Start: start, Resident: true, Restart: types.SystemPluginRestartOnFailure, StopTimeoutMs: 2000}
}

// writeTestPlugin 写出一个可由宿主启动的桩插件目录。
func writeTestPlugin(t *testing.T, root string, spec testPluginSpec) string {
	t.Helper()
	directoryName := spec.directory
	if directoryName == "" {
		directoryName = spec.manifest.ID
	}
	if directoryName == "" {
		t.Fatalf("test plugin directory is required")
	}
	directory := filepath.Join(root, directoryName)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("Mkdir plugin dir error = %v", err)
	}
	executableName := "plugin"
	if runtime.GOOS == "windows" {
		executableName += ".exe"
	}
	manifest := spec.manifest
	manifest.Binaries = []types.SystemPluginBinary{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Path: executableName}}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("Marshal manifest error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("Write manifest error = %v", err)
	}
	defaultConfig := spec.defaultConfig
	if defaultConfig == nil {
		defaultConfig = map[string]any{}
	}
	configPayload, err := json.MarshalIndent(defaultConfig, "", "  ")
	if err != nil {
		t.Fatalf("Marshal config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "config.json"), append(configPayload, '\n'), 0o644); err != nil {
		t.Fatalf("Write config error = %v", err)
	}
	behavior := spec.behavior
	if behavior == nil {
		behavior = map[string]any{}
	}
	behaviorPayload, err := json.MarshalIndent(behavior, "", "  ")
	if err != nil {
		t.Fatalf("Marshal behavior error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "stub.json"), append(behaviorPayload, '\n'), 0o644); err != nil {
		t.Fatalf("Write behavior error = %v", err)
	}
	executablePayload, err := os.ReadFile(stubExecutable)
	if err != nil {
		t.Fatalf("Read stub executable error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, executableName), executablePayload, 0o755); err != nil {
		t.Fatalf("Write stub executable error = %v", err)
	}
	return directory
}

func newTestSystem(t *testing.T, root string) System {
	t.Helper()
	system, err := NewSystem(Config{SourceDir: root, DataDir: filepath.Join(root, "data"), Timeout: testPluginTimeout()})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	t.Cleanup(func() { _ = system.Shutdown(context.Background()) })
	return system
}

func testPluginTimeout() time.Duration {
	if runtime.GOOS == "windows" {
		return 10 * time.Second
	}
	return 3 * time.Second
}

func testSource(pluginID string, interfaceID string, name string) types.SystemPluginPlaceholderSource {
	return types.SystemPluginPlaceholderSource{PluginID: pluginID, InterfaceID: interfaceID, Name: name}
}

func readTrackEntries(t *testing.T, path string) []map[string]any {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		t.Fatalf("Read track file error = %v", err)
	}
	entries := []map[string]any{}
	for _, line := range strings.Split(string(payload), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry := map[string]any{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("Decode track line error = %v", err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func trackEntriesOfKind(t *testing.T, path string, kind string) []map[string]any {
	t.Helper()
	out := []map[string]any{}
	for _, entry := range readTrackEntries(t, path) {
		if entry["kind"] == kind {
			out = append(out, entry)
		}
	}
	return out
}

// waitForTrackEntries 轮询等待指定类别的记录达到数量；用于等待插件异步处理。
func waitForTrackEntries(t *testing.T, path string, kind string, count int) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		entries := trackEntriesOfKind(t, path, kind)
		if len(entries) >= count {
			return entries
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待桩插件记录超时：kind=%s count=%d got=%d", kind, count, len(entries))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func valuesByInterface(values []types.SystemPluginPlaceholderValue) map[string]string {
	out := map[string]string{}
	for _, value := range values {
		out[value.InterfaceID] = value.Value
	}
	return out
}

func problemTypes(problems []types.PlaceholderProblem) map[string]string {
	out := map[string]string{}
	for _, problem := range problems {
		out[problem.Name] = problem.Type
	}
	return out
}

func waitForSessionCount(created System, pluginID string, expected int) bool {
	deadline := time.Now().Add(5 * time.Second)
	for {
		if hasResidentSession(created, pluginID) == (expected == 1) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func hasResidentSession(created System, pluginID string) bool {
	real := realSystemOf(created)
	real.mu.Lock()
	defer real.mu.Unlock()
	return real.residents[pluginID] != nil
}

func realSystemOf(created System) *system {
	return created.(*system)
}
