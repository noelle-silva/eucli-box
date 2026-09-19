package systemplugin

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

const onDemandPluginSource = `
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type input struct {
	Action              string            ` + "`json:\"action\"`" + `
	PluginID            string            ` + "`json:\"pluginId\"`" + `
	PluginDataDirectory string            ` + "`json:\"pluginDataDirectory\"`" + `
}

func main() {
	var in input
	_ = json.NewDecoder(os.Stdin).Decode(&in)
	if in.Action == "sleep" {
		time.Sleep(2 * time.Second)
	}
	if _, err := os.Stat(filepath.Join(in.PluginDataDirectory, "sleep-marker.txt")); err == nil {
		time.Sleep(2 * time.Second)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "success", "values": map[string]string{"value": in.PluginID + "-value"}})
}
`

const persistentPluginSource = `
package main

import (
	"encoding/json"
	"os"
)

func main() {
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var request map[string]any
		if err := decoder.Decode(&request); err != nil {
			return
		}
		_ = encoder.Encode(map[string]any{"status": "success", "values": map[string]string{"value": "persistent-value"}})
	}
}
`

type fakePluginCandidates struct {
	candidates map[string]*releasecheck.ReleaseCandidate
}

func (f *fakePluginCandidates) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	candidate, ok := f.candidates[identity.ID]
	if !ok {
		return nil, fmt.Errorf("no official candidate for %s", identity.ID)
	}
	return candidate, nil
}

type pluginOperationFixture struct {
	t            *testing.T
	system       System
	programRoot  string
	sourceDir    string
	dataRoot     string
	server       *httptest.Server
	manifest     types.ReleaseManifest
	manifestJSON []byte
	archiveBytes []byte
	candidates   *fakePluginCandidates
	realSystem   *system

	mu       sync.Mutex
	archives map[string][]byte
	gate     chan struct{}
	chunks   int
	chunkGap time.Duration
}

func newPluginOperationFixture(t *testing.T) *pluginOperationFixture {
	t.Helper()
	dataRoot := t.TempDir()
	programRoot := filepath.Join(t.TempDir(), "program")
	sourceDir := filepath.Join(programRoot, "system-plugins")
	fixture := &pluginOperationFixture{
		t:           t,
		programRoot: programRoot,
		sourceDir:   sourceDir,
		dataRoot:    dataRoot,
		candidates:  &fakePluginCandidates{candidates: map[string]*releasecheck.ReleaseCandidate{}},
		archives:    map[string][]byte{},
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.servePackage))
	t.Cleanup(fixture.server.Close)
	t.Cleanup(fixture.releaseDownloads)
	created, err := NewSystem(Config{
		SourceDir:   sourceDir,
		DataDir:     filepath.Join(dataRoot, "system-plugins"),
		Timeout:     10 * time.Second,
		BoxVersion:  "0.1.0",
		ProgramRoot: programRoot,
		Candidates:  fixture.candidates,
		HTTPClient:  fixture.server.Client(),
	})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	fixture.system = created
	fixture.realSystem = created.(*system)
	fixture.realSystem.updateWaitTimeout = 500 * time.Millisecond
	t.Cleanup(func() { _ = fixture.system.Shutdown(context.Background()) })
	return fixture
}

func (f *pluginOperationFixture) markSleep(pluginID string) {
	f.t.Helper()
	dataDir := filepath.Join(f.dataRoot, "system-plugins", pluginID)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		f.t.Fatalf("mkdir data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "sleep-marker.txt"), []byte("sleep"), 0o644); err != nil {
		f.t.Fatalf("write marker: %v", err)
	}
}

func (f *pluginOperationFixture) makePluginCandidate(id string, version string, lifecycle string, brokenBinary bool) {
	f.t.Helper()
	binaryPayload := buildPluginBinary(f.t, lifecycle)
	if brokenBinary {
		binaryPayload = []byte("not an executable")
	}
	contentDir := f.t.TempDir()
	binaryPath := filepath.ToSlash(filepath.Join("binary", id+".exe"))
	manifest := types.SystemPluginManifest{
		ID:                    id,
		Name:                  "Demo " + id,
		Description:           "demo plugin",
		Version:               version,
		EucliBoxCompatibility: types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		LifecycleType:         lifecycle,
		Binaries:              []types.SystemPluginBinary{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Path: binaryPath}},
		PlaceholderInterfaces: []types.SystemPluginPlaceholderInterface{{ID: "value", DefaultName: "demo value", Description: "demo"}},
	}
	if lifecycle == types.SystemPluginLifecycleCachedHeartbeat {
		manifest.HeartbeatIntervalMs = 60
	}
	manifestPayload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		f.t.Fatalf("marshal manifest: %v", err)
	}
	files := map[string][]byte{
		"manifest.json": manifestPayload,
		binaryPath:      binaryPayload,
		"config.json":   []byte("{}\n"),
		"README.md":     []byte("# " + id + "\n"),
		"CHANGELOG.md":  []byte("## " + version + "\n"),
	}
	for name, payload := range files {
		path := filepath.Join(contentDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			f.t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			f.t.Fatalf("write %s: %v", path, err)
		}
	}
	product := types.ReleaseProductRecord{
		SchemaVersion:  release.ReleaseManifestSchemaVersion,
		Artifact:       types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: id},
		Version:        version,
		Platform:       types.ReleasePlatformWindowsX64,
		OfficialSource: "https://github.com/noelle-silva/eucli-box-system-plugins",
		Compatibility:  &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		Source:         types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
	}
	productPayload, err := json.MarshalIndent(product, "", "  ")
	if err != nil {
		f.t.Fatalf("marshal product: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "release-product.json"), productPayload, 0o644); err != nil {
		f.t.Fatalf("write product: %v", err)
	}
	f.archiveBytes = zipPluginBytes(f.t, contentDir)
	f.mu.Lock()
	f.archives[id] = f.archiveBytes
	f.mu.Unlock()
	f.candidates.candidates[id] = &releasecheck.ReleaseCandidate{
		Artifact:         product.Artifact,
		Version:          product.Version,
		SourceRevision:   product.Source.Commit,
		SourceRepository: product.Source.Repository,
		Compatibility:    product.Compatibility,
		ReleaseNotes:     "release notes " + version,
		OfficialSource:   product.OfficialSource,
		ArchiveURL:       f.server.URL + "/archive/" + id,
		SizeBytes:        int64(len(f.archiveBytes)),
		SHA256:           release.SHA256(f.archiveBytes),
	}
}

// holdDownloads 阻断所有下载直到 releaseDownloads；用于观察运行中状态与取消。
// releaseDownloads 幂等，测试结束由 Cleanup 兜底释放。
func (f *pluginOperationFixture) holdDownloads() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gate == nil {
		f.gate = make(chan struct{})
	}
}

func (f *pluginOperationFixture) releaseDownloads() {
	f.mu.Lock()
	gate := f.gate
	f.gate = nil
	f.mu.Unlock()
	if gate != nil {
		close(gate)
	}
}

// setSlowDownload 让压缩包按块缓慢写出，用于观察下载进度上报。
func (f *pluginOperationFixture) setSlowDownload(chunks int, gap time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chunks = chunks
	f.chunkGap = gap
}

func (f *pluginOperationFixture) servePackage(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/archive/") {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/archive/")
	f.mu.Lock()
	archive := f.archives[id]
	gate := f.gate
	chunks := f.chunks
	chunkGap := f.chunkGap
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if chunks <= 1 {
		_, _ = w.Write(archive)
		return
	}
	chunkSize := len(archive)/chunks + 1
	for offset := 0; offset < len(archive); offset += chunkSize {
		end := offset + chunkSize
		if end > len(archive) {
			end = len(archive)
		}
		_, _ = w.Write(archive[offset:end])
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(chunkGap)
	}
}

// waitPluginTerminal 等待插件操作进入终态；用于异步任务测试。
func (f *pluginOperationFixture) waitPluginTerminal(t *testing.T, pluginID string) types.ArtifactInstallState {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		state, err := f.system.PluginInstallState(context.Background(), pluginID)
		if err != nil {
			t.Fatalf("PluginInstallState() error = %v", err)
		}
		if isTerminalArtifactStatus(state.Status) {
			return state
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("plugin operation did not reach terminal state in time")
	return types.ArtifactInstallState{}
}

// installPluginAndWait 发起安装并等待终态（同步失败直接返回）。
func (f *pluginOperationFixture) installPluginAndWait(t *testing.T, pluginID string) types.ArtifactInstallState {
	t.Helper()
	state, err := f.system.InstallPlugin(context.Background(), pluginID)
	if err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
	if isTerminalArtifactStatus(state.Status) {
		return state
	}
	return f.waitPluginTerminal(t, pluginID)
}

// updatePluginAndWait 发起更新并等待终态（同步失败直接返回）。
func (f *pluginOperationFixture) updatePluginAndWait(t *testing.T, pluginID string) types.ArtifactInstallState {
	t.Helper()
	state, err := f.system.UpdatePlugin(context.Background(), pluginID)
	if err != nil {
		t.Fatalf("UpdatePlugin() error = %v", err)
	}
	if isTerminalArtifactStatus(state.Status) {
		return state
	}
	return f.waitPluginTerminal(t, pluginID)
}

// waitPluginPhase 等待插件任务进入指定阶段。
func (f *pluginOperationFixture) waitPluginPhase(t *testing.T, pluginID string, phase string) types.ArtifactInstallState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state, err := f.system.PluginInstallState(context.Background(), pluginID)
		if err != nil {
			t.Fatalf("PluginInstallState() error = %v", err)
		}
		if state.Phase == phase {
			return state
		}
		if isTerminalArtifactStatus(state.Status) {
			t.Fatalf("operation reached terminal status before phase %s: %#v", phase, state)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("plugin operation did not reach phase %s in time", phase)
	return types.ArtifactInstallState{}
}

func isTerminalArtifactStatus(status string) bool {
	switch status {
	case types.ArtifactStatusNotInstalled, types.ArtifactStatusActive, types.ArtifactStatusUnavailable,
		types.ArtifactStatusFailed, types.ArtifactStatusBlocked, types.ArtifactStatusCancelled:
		return true
	}
	return false
}

func buildPluginBinary(t *testing.T, lifecycle string) []byte {
	t.Helper()
	source := onDemandPluginSource
	if lifecycle == types.SystemPluginLifecyclePersistent {
		source = persistentPluginSource
	}
	dir := t.TempDir()
	sourceFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(sourceFile, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	exe := filepath.Join(dir, "plugin")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, sourceFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build plugin failed: %v\n%s", err, output)
	}
	payload, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read plugin binary: %v", err)
	}
	return payload
}

func zipPluginBytes(t *testing.T, root string) []byte {
	t.Helper()
	output, err := os.CreateTemp(t.TempDir(), "plugin-*.zip")
	if err != nil {
		t.Fatalf("create temp zip: %v", err)
	}
	writer := zip.NewWriter(output)
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := writer.Create(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		_, err = file.Write(payload)
		return err
	})
	closeErr := writer.Close()
	_ = output.Close()
	if walkErr != nil {
		t.Fatalf("zip walk: %v", walkErr)
	}
	if closeErr != nil {
		t.Fatalf("close zip: %v", closeErr)
	}
	payload, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	return payload
}

// assertWorkRootEmpty 断言某一插件 work/ 下没有任何残留条目；目录不存在同样视为通过。
func (f *pluginOperationFixture) assertWorkRootEmpty(t *testing.T, pluginID string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(f.sourceDir, pluginID, "work"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		t.Fatalf("read work root: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("work root not empty: %v", names)
	}
}

func TestInstallPluginCompletesAndBecomesActive(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	state := fixture.installPluginAndWait(t, "demo")
	if state.Status != types.ArtifactStatusActive || state.CurrentVersion != "0.1.0" || !state.Installed {
		t.Fatalf("state = %#v", state)
	}
	plugins, err := fixture.system.ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("ListPlugins() error = %v", err)
	}
	if len(plugins) != 1 || plugins[0].ID != "demo" || !plugins[0].Installed {
		t.Fatalf("plugins = %#v", plugins)
	}
	values, problems := fixture.system.ResolvePlaceholderValues(context.Background())
	if len(problems) != 0 || len(values) != 1 || values[0].Value != "demo-value" {
		t.Fatalf("values = %#v problems = %#v", values, problems)
	}
}

func TestUpdatePluginClearsStaleFailureAfterSwitch(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.installPluginAndWait(t, "demo")
	fixture.realSystem.setFailure("demo", "旧版本的失败记录")
	fixture.makePluginCandidate("demo", "0.1.1", types.SystemPluginLifecycleOnDemand, false)
	state := fixture.updatePluginAndWait(t, "demo")
	if state.Status != types.ArtifactStatusActive || state.CurrentVersion != "0.1.1" {
		t.Fatalf("state = %#v", state)
	}
	if failure := fixture.realSystem.getFailure("demo"); failure != "" {
		t.Fatalf("stale failure not cleared: %q", failure)
	}
	plugins, err := fixture.system.ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("ListPlugins() error = %v", err)
	}
	if len(plugins) != 1 || plugins[0].Status != types.SystemPluginStatusActive {
		t.Fatalf("plugins = %#v", plugins)
	}
}

func TestInstallPluginRejectsIncompatibleCandidateBeforeDownload(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	compatibility := types.EucliBoxCompatibility{MinimumVersion: "0.5.0", MaximumVersionExclusive: "0.6.0"}
	fixture.candidates.candidates["demo"].Compatibility = &compatibility
	state, err := fixture.system.InstallPlugin(context.Background(), "demo")
	if err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
	if state.Status != types.ArtifactStatusBlocked || state.Error.Code != types.ArtifactErrorCompatibility {
		t.Fatalf("state = %#v", state)
	}
	if _, err := os.Stat(filepath.Join(fixture.sourceDir, "demo", "work")); err == nil {
		t.Fatal("download started for incompatible candidate")
	}
}

func TestUpdatePluginRestoresPreviousVersionOnProbeFailure(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.installPluginAndWait(t, "demo")
	fixture.makePluginCandidate("demo", "0.1.1", types.SystemPluginLifecycleOnDemand, true)
	state := fixture.updatePluginAndWait(t, "demo")
	if state.Status != types.ArtifactStatusActive || state.Error.Code != types.ArtifactErrorProbeFailed {
		t.Fatalf("state = %#v", state)
	}
	state, err := fixture.system.PluginInstallState(context.Background(), "demo")
	if err != nil {
		t.Fatalf("PluginInstallState() error = %v", err)
	}
	if state.CurrentVersion != "0.1.0" || state.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", state)
	}
}

// TestInstallPluginReclaimsWorkDirOnSuccess 验证操作进入成功终态后本轮工作目录立即回收。
func TestInstallPluginReclaimsWorkDirOnSuccess(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	state := fixture.installPluginAndWait(t, "demo")
	if state.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", state)
	}
	fixture.assertWorkRootEmpty(t, "demo")
	if _, err := os.Stat(filepath.Join(fixture.sourceDir, "demo", "operation.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful operation record should be removed, err = %v", err)
	}
}

// TestUpdatePluginReclaimsWorkDirOnProbeFailure 验证失败终态同样回收工作目录，失败记录保留供状态展示。
func TestUpdatePluginReclaimsWorkDirOnProbeFailure(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.installPluginAndWait(t, "demo")
	fixture.makePluginCandidate("demo", "0.1.1", types.SystemPluginLifecycleOnDemand, true)
	state := fixture.updatePluginAndWait(t, "demo")
	if state.Status != types.ArtifactStatusActive || state.Error.Code != types.ArtifactErrorProbeFailed {
		t.Fatalf("state = %#v", state)
	}
	fixture.assertWorkRootEmpty(t, "demo")
	record, err := release.ReadOperationRecord(filepath.Join(fixture.sourceDir, "demo", "operation.json"))
	if err != nil {
		t.Fatalf("failed operation record missing: %v", err)
	}
	if record.Result != release.OperationResultFailed || record.ErrorCode != types.ArtifactErrorProbeFailed {
		t.Fatalf("record = %#v", record)
	}
}

// TestPluginOperationSweepsStaleWorkDirs 验证下一次操作开始时清扫遗留的操作工作目录，
// 且不触碰非操作前缀的条目。
func TestPluginOperationSweepsStaleWorkDirs(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	workRoot := filepath.Join(fixture.sourceDir, "demo", "work")
	staleDir := filepath.Join(workRoot, "plugin-operation-stale")
	if err := os.MkdirAll(filepath.Join(staleDir, "extracted"), 0o755); err != nil {
		t.Fatalf("mkdir stale work dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "extracted", "leftover.bin"), []byte("junk"), 0o644); err != nil {
		t.Fatalf("write leftover: %v", err)
	}
	keepDir := filepath.Join(workRoot, "unrelated")
	if err := os.MkdirAll(keepDir, 0o755); err != nil {
		t.Fatalf("mkdir unrelated dir: %v", err)
	}
	keepFile := filepath.Join(workRoot, "notes.txt")
	if err := os.WriteFile(keepFile, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}

	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.installPluginAndWait(t, "demo")

	if _, err := os.Stat(staleDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale operation work dir not swept: %v", err)
	}
	if _, err := os.Stat(keepDir); err != nil {
		t.Fatalf("unrelated directory removed: %v", err)
	}
	if _, err := os.Stat(keepFile); err != nil {
		t.Fatalf("unrelated file removed: %v", err)
	}
}

func TestUpdatePluginBlockedByOnDemandActivity(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.installPluginAndWait(t, "demo")
	fixture.makePluginCandidate("demo", "0.1.1", types.SystemPluginLifecycleOnDemand, false)
	fixture.markSleep("demo")
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = fixture.system.ResolvePlaceholderValues(context.Background())
	}()
	time.Sleep(300 * time.Millisecond)
	state, err := fixture.system.UpdatePlugin(context.Background(), "demo")
	if err != nil {
		t.Fatalf("UpdatePlugin() error = %v", err)
	}
	if state.Status != types.ArtifactStatusBlocked || state.Error.Code != types.ArtifactErrorPluginActive {
		t.Fatalf("state = %#v", state)
	}
	<-done
}

func TestUpdatePluginStopsPersistentProcessAndRestarts(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecyclePersistent, false)
	if err := fixture.system.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fixture.installPluginAndWait(t, "demo")
	fixture.waitPersistentProcess(t, "demo")
	fixture.makePluginCandidate("demo", "0.1.1", types.SystemPluginLifecyclePersistent, false)
	state := fixture.updatePluginAndWait(t, "demo")
	if state.Status != types.ArtifactStatusActive || state.CurrentVersion != "0.1.1" {
		t.Fatalf("state = %#v", state)
	}
	fixture.waitPersistentProcess(t, "demo")
}

func TestPluginInstallStateRecoversInterruptedSwitch(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.installPluginAndWait(t, "demo")
	record, err := release.NewOperationRecord("interrupted-op", types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "demo"}, release.OperationActionUpdate, "0.1.1", filepath.Join(fixture.sourceDir, "demo", "work", "interrupted-op"))
	if err != nil {
		t.Fatalf("NewOperationRecord() error = %v", err)
	}
	record.CurrentVersion = "0.1.0"
	record.Phase = types.ArtifactPhaseSwitch
	if err := release.WriteOperationRecord(filepath.Join(fixture.sourceDir, "demo", "operation.json"), record); err != nil {
		t.Fatalf("WriteOperationRecord() error = %v", err)
	}
	state, err := fixture.system.PluginInstallState(context.Background(), "demo")
	if err != nil {
		t.Fatalf("PluginInstallState() error = %v", err)
	}
	if state.CurrentVersion != "0.1.0" || state.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", state)
	}
}

// waitPersistentProcess 等待插件的 persistent 进程被生命周期恢复启动。
func (f *pluginOperationFixture) waitPersistentProcess(t *testing.T, pluginID string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		f.realSystem.mu.Lock()
		process := f.realSystem.persistent[pluginID]
		f.realSystem.mu.Unlock()
		if process != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("persistent process for %s did not start in time", pluginID)
}

func TestPluginActivityReportsOnDemand(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.installPluginAndWait(t, "demo")
	activity, err := fixture.system.PluginActivity(context.Background(), "demo")
	if err != nil {
		t.Fatalf("PluginActivity() error = %v", err)
	}
	if activity.Active || activity.ActiveRequests != 0 {
		t.Fatalf("activity = %#v", activity)
	}
}

// TestInstallPluginReturnsRunningStateImmediately 验证安装请求立即返回运行态，
// 不再等待整个下载与切换流程结束。
func TestInstallPluginReturnsRunningStateImmediately(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.holdDownloads()
	state, err := fixture.system.InstallPlugin(context.Background(), "demo")
	if err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
	if isTerminalArtifactStatus(state.Status) {
		t.Fatalf("expected running state immediately, got %#v", state)
	}
	if state.OperationID == "" {
		t.Fatalf("operationId missing: %#v", state)
	}
	fixture.releaseDownloads()
	terminal := fixture.waitPluginTerminal(t, "demo")
	if terminal.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", terminal)
	}
}

// TestPluginInstallProgressReported 验证下载阶段的已下载字节与总量被状态查询上报。
func TestPluginInstallProgressReported(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.setSlowDownload(6, 30*time.Millisecond)
	if _, err := fixture.system.InstallPlugin(context.Background(), "demo"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	seen := false
	for time.Now().Before(deadline) {
		state, err := fixture.system.PluginInstallState(context.Background(), "demo")
		if err != nil {
			t.Fatalf("PluginInstallState() error = %v", err)
		}
		if state.Progress.ReceivedBytes > 0 {
			if state.Progress.TotalBytes != int64(len(fixture.archiveBytes)) {
				t.Fatalf("progress total = %d, want %d", state.Progress.TotalBytes, len(fixture.archiveBytes))
			}
			seen = true
			break
		}
		if isTerminalArtifactStatus(state.Status) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !seen {
		t.Fatal("download progress was not reported")
	}
	terminal := fixture.waitPluginTerminal(t, "demo")
	if terminal.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", terminal)
	}
}

// TestCancelPluginOperationDuringDownload 验证下载阶段取消：终态为已取消、
// 工作目录清理干净，且同一条目可重新安装成功。
func TestCancelPluginOperationDuringDownload(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.holdDownloads()
	state, err := fixture.system.InstallPlugin(context.Background(), "demo")
	if err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
	if isTerminalArtifactStatus(state.Status) {
		t.Fatalf("expected running state, got %#v", state)
	}
	fixture.waitPluginPhase(t, "demo", types.ArtifactPhaseDownload)
	cancelled, err := fixture.system.CancelPluginOperation(context.Background(), "demo")
	if err != nil {
		t.Fatalf("CancelPluginOperation() error = %v", err)
	}
	fixture.releaseDownloads()
	if cancelled.Status != types.ArtifactStatusCancelled {
		t.Fatalf("state = %#v", cancelled)
	}
	fixture.assertWorkRootEmpty(t, "demo")
	restoreState := fixture.installPluginAndWait(t, "demo")
	if restoreState.Status != types.ArtifactStatusActive {
		t.Fatalf("re-install state = %#v", restoreState)
	}
}

// TestCancelPluginOperationRejectedAfterSwitch 验证进入切换阶段后拒绝取消。
func TestCancelPluginOperationRejectedAfterSwitch(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.installPluginAndWait(t, "demo")
	activity := fixture.realSystem.activityFor("demo")
	if code := activity.beginUpdate("manual-switch-op", nil, fixture.realSystem.updateWaitTimeout); code != "" {
		t.Fatalf("beginUpdate() = %s", code)
	}
	defer activity.endUpdate()
	activity.updatePhase(types.ArtifactPhaseSwitch)
	state, err := fixture.system.CancelPluginOperation(context.Background(), "demo")
	if err != nil {
		t.Fatalf("CancelPluginOperation() error = %v", err)
	}
	if state.Status != types.ArtifactStatusBlocked || state.Error.Code != types.ArtifactErrorCancelRejected {
		t.Fatalf("state = %#v", state)
	}
}

// TestInstallPluginContinuesAfterRequestContextCancelled 验证任务与请求生命周期解绑：
// 请求 context 取消后，后台安装继续推进到成功终态。
func TestInstallPluginContinuesAfterRequestContextCancelled(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.setSlowDownload(4, 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := fixture.system.InstallPlugin(ctx, "demo"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
	cancel()
	terminal := fixture.waitPluginTerminal(t, "demo")
	if terminal.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", terminal)
	}
}

// TestConcurrentInstallsOfDifferentPlugins 验证不同插件的安装各自独立、可同时进行。
func TestConcurrentInstallsOfDifferentPlugins(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("alpha", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.makePluginCandidate("beta", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.holdDownloads()
	if _, err := fixture.system.InstallPlugin(context.Background(), "alpha"); err != nil {
		t.Fatalf("InstallPlugin(alpha) error = %v", err)
	}
	if _, err := fixture.system.InstallPlugin(context.Background(), "beta"); err != nil {
		t.Fatalf("InstallPlugin(beta) error = %v", err)
	}
	fixture.waitPluginPhase(t, "alpha", types.ArtifactPhaseDownload)
	fixture.waitPluginPhase(t, "beta", types.ArtifactPhaseDownload)
	fixture.releaseDownloads()
	alphaState := fixture.waitPluginTerminal(t, "alpha")
	betaState := fixture.waitPluginTerminal(t, "beta")
	if alphaState.Status != types.ArtifactStatusActive || betaState.Status != types.ArtifactStatusActive {
		t.Fatalf("alpha = %#v, beta = %#v", alphaState, betaState)
	}
}

// TestListPluginOperationsReturnsRunningAndTerminal 验证批量操作查询同时返回
// 运行中的任务与落盘的终态记录。
func TestListPluginOperationsReturnsRunningAndTerminal(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.installPluginAndWait(t, "demo")
	fixture.makePluginCandidate("demo", "0.1.1", types.SystemPluginLifecycleOnDemand, true)
	fixture.updatePluginAndWait(t, "demo")

	fixture.makePluginCandidate("slow", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.holdDownloads()
	if _, err := fixture.system.InstallPlugin(context.Background(), "slow"); err != nil {
		t.Fatalf("InstallPlugin(slow) error = %v", err)
	}
	fixture.waitPluginPhase(t, "slow", types.ArtifactPhaseDownload)

	operations, err := fixture.system.ListPluginOperations(context.Background())
	if err != nil {
		t.Fatalf("ListPluginOperations() error = %v", err)
	}
	byID := map[string]types.ArtifactInstallState{}
	for _, operation := range operations {
		byID[operation.Artifact.ID] = operation
	}
	if byID["slow"].Status != types.ArtifactStatusDownloading {
		t.Fatalf("slow operation = %#v", byID["slow"])
	}
	if _, ok := byID["demo"]; !ok {
		t.Fatal("terminal operation record for demo missing")
	}
	fixture.releaseDownloads()
	fixture.waitPluginTerminal(t, "slow")
}

// TestInstallPluginRejectsDuplicateWhileRunning 验证运行中的任务被重复发起时拒绝，
// 且不会误触中断恢复清理运行中的工作目录。
func TestInstallPluginRejectsDuplicateWhileRunning(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	fixture.holdDownloads()
	first, err := fixture.system.InstallPlugin(context.Background(), "demo")
	if err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
	if isTerminalArtifactStatus(first.Status) {
		t.Fatalf("expected running state, got %#v", first)
	}
	fixture.waitPluginPhase(t, "demo", types.ArtifactPhaseDownload)
	second, err := fixture.system.InstallPlugin(context.Background(), "demo")
	if err != nil {
		t.Fatalf("InstallPlugin() duplicate error = %v", err)
	}
	if second.Status != types.ArtifactStatusBlocked || second.Error.Code != types.ArtifactErrorUpdateInProgress {
		t.Fatalf("duplicate state = %#v", second)
	}
	fixture.releaseDownloads()
	terminal := fixture.waitPluginTerminal(t, "demo")
	if terminal.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", terminal)
	}
}

var _ = strings.TrimSpace
