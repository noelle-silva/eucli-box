package systemplugin

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.servePackage))
	t.Cleanup(fixture.server.Close)
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
	f.candidates.candidates[id] = &releasecheck.ReleaseCandidate{
		Artifact:         product.Artifact,
		Version:          product.Version,
		SourceRevision:   product.Source.Commit,
		SourceRepository: product.Source.Repository,
		Compatibility:    product.Compatibility,
		ReleaseNotes:     "release notes " + version,
		OfficialSource:   product.OfficialSource,
		ArchiveURL:       f.server.URL + "/archive",
		SizeBytes:        int64(len(f.archiveBytes)),
		SHA256:           release.SHA256(f.archiveBytes),
	}
}

func (f *pluginOperationFixture) servePackage(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/archive":
		_, _ = w.Write(f.archiveBytes)
	default:
		http.NotFound(w, r)
	}
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

func TestInstallPluginCompletesAndBecomesActive(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	state, err := fixture.system.InstallPlugin(context.Background(), "demo")
	if err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
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
	if _, err := fixture.system.InstallPlugin(context.Background(), "demo"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
	fixture.realSystem.setFailure("demo", "旧版本的失败记录")
	fixture.makePluginCandidate("demo", "0.1.1", types.SystemPluginLifecycleOnDemand, false)
	state, err := fixture.system.UpdatePlugin(context.Background(), "demo")
	if err != nil {
		t.Fatalf("UpdatePlugin() error = %v", err)
	}
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
	if _, err := fixture.system.InstallPlugin(context.Background(), "demo"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
	fixture.makePluginCandidate("demo", "0.1.1", types.SystemPluginLifecycleOnDemand, true)
	state, err := fixture.system.UpdatePlugin(context.Background(), "demo")
	if err != nil {
		t.Fatalf("UpdatePlugin() error = %v", err)
	}
	if state.Status != types.ArtifactStatusFailed || state.Error.Code != types.ArtifactErrorProbeFailed {
		t.Fatalf("state = %#v", state)
	}
	state, err = fixture.system.PluginInstallState(context.Background(), "demo")
	if err != nil {
		t.Fatalf("PluginInstallState() error = %v", err)
	}
	if state.CurrentVersion != "0.1.0" || state.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", state)
	}
}

func TestUpdatePluginBlockedByOnDemandActivity(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	if _, err := fixture.system.InstallPlugin(context.Background(), "demo"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
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
	if _, err := fixture.system.InstallPlugin(context.Background(), "demo"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
	fixture.realSystem.mu.Lock()
	process := fixture.realSystem.persistent["demo"]
	fixture.realSystem.mu.Unlock()
	if process == nil {
		t.Fatal("persistent process not started")
	}
	fixture.makePluginCandidate("demo", "0.1.1", types.SystemPluginLifecyclePersistent, false)
	state, err := fixture.system.UpdatePlugin(context.Background(), "demo")
	if err != nil {
		t.Fatalf("UpdatePlugin() error = %v", err)
	}
	if state.Status != types.ArtifactStatusActive || state.CurrentVersion != "0.1.1" {
		t.Fatalf("state = %#v", state)
	}
	fixture.realSystem.mu.Lock()
	process = fixture.realSystem.persistent["demo"]
	fixture.realSystem.mu.Unlock()
	if process == nil {
		t.Fatal("persistent process not restarted after update")
	}
}

func TestPluginInstallStateRecoversInterruptedSwitch(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	if _, err := fixture.system.InstallPlugin(context.Background(), "demo"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
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

func TestPluginActivityReportsOnDemand(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("demo", "0.1.0", types.SystemPluginLifecycleOnDemand, false)
	if _, err := fixture.system.InstallPlugin(context.Background(), "demo"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
	activity, err := fixture.system.PluginActivity(context.Background(), "demo")
	if err != nil {
		t.Fatalf("PluginActivity() error = %v", err)
	}
	if activity.Active || activity.ActiveRequests != 0 {
		t.Fatalf("activity = %#v", activity)
	}
}

var _ = strings.TrimSpace
