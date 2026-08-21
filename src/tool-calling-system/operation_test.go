package toolcalling

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
	datastorage "eucli-box/src/data-storage-system"
)

const probeToolSource = `
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type input struct {
	ActionID          string ` + "`json:\"actionId\"`" + `
	ToolDataDirectory string ` + "`json:\"toolDataDirectory\"`" + `
}

func main() {
	var in input
	_ = json.NewDecoder(os.Stdin).Decode(&in)
	if in.ActionID == "sleep" {
		_ = os.MkdirAll(in.ToolDataDirectory, 0o755)
		_ = os.WriteFile(filepath.Join(in.ToolDataDirectory, "marker.txt"), []byte("running"), 0o644)
		time.Sleep(2 * time.Second)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "success", "content": "ok"})
}
`

type fakeCandidateReader struct {
	candidates map[string]*releasecheck.ReleaseCandidate
}

func (f *fakeCandidateReader) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	candidate, ok := f.candidates[identity.ID]
	if !ok {
		return nil, fmt.Errorf("no official candidate for %s", identity.ID)
	}
	return candidate, nil
}

type toolOperationFixture struct {
	t            *testing.T
	system       System
	programRoot  string
	dataRoot     string
	server       *httptest.Server
	manifest     types.ReleaseManifest
	manifestJSON []byte
	archiveBytes []byte
	contentDir   string
	candidates   *fakeCandidateReader
	realSystem   *system
}

func newToolOperationFixture(t *testing.T) *toolOperationFixture {
	t.Helper()
	dataRoot := t.TempDir()
	now := time.Now().UTC()
	if err := datastorage.WriteStorageVersion(context.Background(), dataRoot, datastorage.StorageVersion{Version: "1.0.0", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("WriteStorageVersion() error = %v", err)
	}
	programRoot := filepath.Join(t.TempDir(), "program", "tools")
	storage, err := datastorage.NewSystem(datastorage.Config{RootDir: dataRoot, ToolBodiesRoot: programRoot})
	if err != nil {
		t.Fatalf("datastorage.NewSystem() error = %v", err)
	}
	if err := storage.Initialize(context.Background()); err != nil {
		t.Fatalf("storage.Initialize() error = %v", err)
	}
	fixture := &toolOperationFixture{t: t, programRoot: programRoot, dataRoot: dataRoot, candidates: &fakeCandidateReader{candidates: map[string]*releasecheck.ReleaseCandidate{}}}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.servePackage))
	t.Cleanup(fixture.server.Close)
	fixture.system, err = NewSystem(Config{
		LegacyToolTimeout: 15 * time.Second,
		BoxVersion:        "0.1.0",
		ProgramRoot:       programRoot,
		Candidates:        fixture.candidates,
		HTTPClient:        fixture.server.Client(),
	}, &fakePermission{}, storage)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	fixture.realSystem = fixture.system.(*system)
	fixture.realSystem.updateWaitTimeout = 500 * time.Millisecond
	return fixture
}

// makeToolCandidate 构造工具成品并注册为官方候选；binary 为空字符串时写入损坏二进制。
// verificationOnly 控制包内身份资料是否带开发/验证标记（开发候选使用）。
func (f *toolOperationFixture) makeToolCandidate(id string, version string, brokenBinary bool, verificationOnly bool) {
	f.t.Helper()
	exe := buildTool(f.t, probeToolSource)
	binaryPayload, err := os.ReadFile(exe)
	if err != nil {
		f.t.Fatalf("read binary: %v", err)
	}
	if brokenBinary {
		binaryPayload = []byte("not an executable")
	}
	contentDir := f.t.TempDir()
	binaryPath := filepath.ToSlash(filepath.Join("binary", "windows-amd64", id+".exe"))
	definition := types.ToolDefinition{
		ID:                    id,
		Name:                  "Demo " + id,
		Description:           "demo tool",
		Version:               version,
		EucliBoxCompatibility: types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		DefaultInvocationMode: types.ToolInvocationModeSync,
		Type:                  "local",
		BodyDirectory:         ".",
		Binaries:              []types.ToolBinary{{GOOS: "windows", GOARCH: "amd64", Path: binaryPath}},
	}
	definitionPayload, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		f.t.Fatalf("marshal definition: %v", err)
	}
	files := map[string][]byte{
		"definition.json": definitionPayload,
		binaryPath:        binaryPayload,
		"README.md":       []byte("# " + id + "\n"),
		"CHANGELOG.md":    []byte("## " + version + "\n"),
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
		SchemaVersion:    release.ReleaseManifestSchemaVersion,
		Artifact:         types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: id},
		Version:          version,
		Platform:         types.ReleasePlatformWindowsX64,
		OfficialSource:   "https://github.com/noelle-silva/eucli-box-ai-tools",
		Compatibility:    &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		Source:           types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
		VerificationOnly: verificationOnly,
	}
	productPayload, err := json.MarshalIndent(product, "", "  ")
	if err != nil {
		f.t.Fatalf("marshal product: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "release-product.json"), productPayload, 0o644); err != nil {
		f.t.Fatalf("write product: %v", err)
	}
	f.archiveBytes = zipBytes(f.t, contentDir)
	f.contentDir = contentDir
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

func (f *toolOperationFixture) servePackage(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/archive":
		_, _ = w.Write(f.archiveBytes)
	default:
		http.NotFound(w, r)
	}
}

// TestInstallToolFromDevelopmentSource 验证开发源候选读取器驱动的完整安装：
// 本地成品 zip 经 AcquireAndValidatePackage（Development 放行）入程序区并激活。
func TestInstallToolFromDevelopmentSource(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("dev-demo", "0.1.1", false, true)
	packageRoot := filepath.Join(t.TempDir(), "dev-packages")
	versionDir := filepath.Join(packageRoot, "tool-dev-demo", "0.1.1")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatalf("mkdir version dir: %v", err)
	}
	archiveName := "tool-dev-demo_0.1.1_windows-x64.zip"
	if err := os.WriteFile(filepath.Join(versionDir, archiveName), fixture.archiveBytes, 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	size, sha256, err := release.RecordForFile(filepath.Join(versionDir, archiveName))
	if err != nil {
		t.Fatalf("record for file: %v", err)
	}
	fileRecords, err := release.CollectFileRecords(fixture.contentDir)
	if err != nil {
		t.Fatalf("collect file records: %v", err)
	}
	manifest := types.ReleaseManifest{
		SchemaVersion:    release.ReleaseManifestSchemaVersion,
		Artifact:         types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "dev-demo"},
		Version:          "0.1.1",
		Platform:         types.ReleasePlatformWindowsX64,
		OfficialSource:   "https://github.com/noelle-silva/eucli-box-ai-tools",
		Compatibility:    &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		Source:           types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
		VerificationOnly: true,
		TagName:          "v0.1.1",
		Archive:          types.ReleaseFileRecord{Name: archiveName, Size: size, SHA256: sha256},
		Files:            fileRecords,
	}
	manifestPayload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "tool-dev-demo_0.1.1_windows-x64.manifest.json"), manifestPayload, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	reader, err := releasecheck.NewDevelopmentSourceReader("1", packageRoot)
	if err != nil {
		t.Fatalf("NewDevelopmentSourceReader() error = %v", err)
	}
	devSystem, err := NewSystem(Config{
		LegacyToolTimeout: 15 * time.Second,
		BoxVersion:        "0.1.0",
		ProgramRoot:       fixture.programRoot,
		Candidates:        reader,
		HTTPClient:        http.DefaultClient,
	}, &fakePermission{}, fixture.realSystem.storage)
	if err != nil {
		t.Fatalf("NewSystem(dev) error = %v", err)
	}
	state, err := devSystem.InstallTool(context.Background(), "dev-demo")
	if err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	if state.Artifact.ID != "dev-demo" || state.Status != types.ArtifactStatusActive || state.CurrentVersion != "0.1.1" {
		t.Fatalf("install state = %#v", state)
	}
}

func zipBytes(t *testing.T, root string) []byte {
	t.Helper()
	var buffer strings.Builder
	_ = buffer
	output, err := os.CreateTemp(t.TempDir(), "tool-*.zip")
	if err != nil {
		t.Fatalf("create temp zip: %v", err)
	}
	defer output.Close()
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

func (f *toolOperationFixture) loadedTool(t *testing.T, toolID string) types.ToolDefinition {
	t.Helper()
	tool, err := f.system.LoadTool(context.Background(), toolID)
	if err != nil {
		t.Fatalf("LoadTool() error = %v", err)
	}
	return tool
}

func (f *toolOperationFixture) runTool(t *testing.T, toolID string, sleep bool) error {
	t.Helper()
	tool := f.loadedTool(t, toolID)
	actionID := "a1"
	if sleep {
		actionID = "sleep"
	}
	plan := allowedPlan(tool, "")
	plan.Action.ID = actionID
	executable, err := selectExecutable(tool)
	if err != nil {
		return err
	}
	plan.Executable = executable
	result, err := f.system.Execute(context.Background(), plan)
	if err != nil {
		return err
	}
	if result.Status != types.ToolStatusSuccess {
		return fmt.Errorf("tool result status = %s: %s", result.Status, result.Error)
	}
	return nil
}

func TestInstallToolCompletesAndBecomesActive(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false, false)
	state, err := fixture.system.InstallTool(context.Background(), "demo")
	if err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	if state.Status != types.ArtifactStatusActive || state.CurrentVersion != "0.1.0" || !state.Installed {
		t.Fatalf("state = %#v", state)
	}
	if _, err := os.Stat(filepath.Join(fixture.programRoot, "demo", "current.json")); err != nil {
		t.Fatalf("current.json missing: %v", err)
	}
	versions, err := os.ReadDir(filepath.Join(fixture.programRoot, "demo", "versions"))
	if err != nil || len(versions) != 1 || versions[0].Name() != "0.1.0" {
		t.Fatalf("versions = %v, %v", versions, err)
	}
	if err := fixture.runTool(t, "demo", false); err != nil {
		t.Fatalf("execute after install error = %v", err)
	}
}

func TestInstallToolRejectsIncompatibleCandidateBeforeDownload(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false, false)
	compatibility := types.EucliBoxCompatibility{MinimumVersion: "0.5.0", MaximumVersionExclusive: "0.6.0"}
	fixture.candidates.candidates["demo"].Compatibility = &compatibility
	state, err := fixture.system.InstallTool(context.Background(), "demo")
	if err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	if state.Status != types.ArtifactStatusBlocked || state.Error.Code != types.ArtifactErrorCompatibility {
		t.Fatalf("state = %#v", state)
	}
	if _, err := os.Stat(filepath.Join(fixture.programRoot, "demo", "work")); err == nil {
		t.Fatal("download started for incompatible candidate")
	}
}

func TestUpdateToolBlockedByActiveExecution(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false, false)
	if _, err := fixture.system.InstallTool(context.Background(), "demo"); err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	fixture.makeToolCandidate("demo", "0.1.1", false, false)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if runErr := fixture.runTool(t, "demo", true); runErr != nil {
			t.Logf("runTool error: %v", runErr)
		}
	}()
	fixture.waitForMarker(t, "demo")
	state, err := fixture.system.UpdateTool(context.Background(), "demo")
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}
	if state.Status != types.ArtifactStatusBlocked || state.Error.Code != types.ArtifactErrorToolActive {
		t.Fatalf("state = %#v", state)
	}
	<-done
}

func (f *toolOperationFixture) waitForMarker(t *testing.T, toolID string) {
	t.Helper()
	marker := filepath.Join(f.dataRoot, "tool-data", toolID, "marker.txt")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("tool execution did not start within deadline")
}

func TestActivityBeginUpdateTimesOutWhileExecuting(t *testing.T) {
	activity := &toolActivity{}
	activity.acquire()
	started := time.Now()
	code := activity.beginUpdate("op-1", 200*time.Millisecond)
	if code != types.ArtifactErrorToolActive {
		t.Fatalf("code = %s", code)
	}
	if time.Since(started) < 150*time.Millisecond {
		t.Fatalf("returned too early: %s", time.Since(started))
	}
	activity.release()
	if activity.state().Updating {
		t.Fatal("update gate still open after timeout")
	}
}

func TestUpdateToolRestoresPreviousVersionOnProbeFailure(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false, false)
	if _, err := fixture.system.InstallTool(context.Background(), "demo"); err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	fixture.makeToolCandidate("demo", "0.1.1", true, false)
	state, err := fixture.system.UpdateTool(context.Background(), "demo")
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}
	if state.Status != types.ArtifactStatusFailed || state.Error.Code != types.ArtifactErrorProbeFailed {
		t.Fatalf("state = %#v", state)
	}
	if err := fixture.runTool(t, "demo", false); err != nil {
		t.Fatalf("previous version cannot run: %v", err)
	}
	state, err = fixture.system.ToolInstallState(context.Background(), "demo")
	if err != nil {
		t.Fatalf("ToolInstallState() error = %v", err)
	}
	if state.CurrentVersion != "0.1.0" || state.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", state)
	}
}

func TestToolInstallStateRecoversInterruptedSwitch(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false, false)
	if _, err := fixture.system.InstallTool(context.Background(), "demo"); err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	record, err := release.NewOperationRecord("interrupted-op", types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"}, release.OperationActionUpdate, "0.1.1", filepath.Join(fixture.programRoot, "demo", "work", "interrupted-op"))
	if err != nil {
		t.Fatalf("NewOperationRecord() error = %v", err)
	}
	record.CurrentVersion = "0.1.0"
	record.Phase = types.ArtifactPhaseSwitch
	if err := release.WriteOperationRecord(filepath.Join(fixture.programRoot, "demo", "operation.json"), record); err != nil {
		t.Fatalf("WriteOperationRecord() error = %v", err)
	}
	state, err := fixture.system.ToolInstallState(context.Background(), "demo")
	if err != nil {
		t.Fatalf("ToolInstallState() error = %v", err)
	}
	if state.CurrentVersion != "0.1.0" {
		t.Fatalf("state = %#v", state)
	}
	if err := fixture.runTool(t, "demo", false); err != nil {
		t.Fatalf("restored version cannot run: %v", err)
	}
}

func TestToolActivityStateReportsExecution(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false, false)
	if _, err := fixture.system.InstallTool(context.Background(), "demo"); err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = fixture.runTool(t, "demo", true)
	}()
	time.Sleep(300 * time.Millisecond)
	activity, err := fixture.system.ToolActivity(context.Background(), "demo")
	if err != nil {
		t.Fatalf("ToolActivity() error = %v", err)
	}
	if !activity.Active || activity.ActiveRequests < 1 {
		t.Fatalf("activity = %#v", activity)
	}
	<-done
	activity, err = fixture.system.ToolActivity(context.Background(), "demo")
	if err != nil {
		t.Fatalf("ToolActivity() error = %v", err)
	}
	if activity.Active || activity.ActiveRequests != 0 {
		t.Fatalf("activity after release = %#v", activity)
	}
}

func TestToolOperationIDValidation(t *testing.T) {
	if _, err := cleanToolID("../bad"); err == nil {
		t.Fatal("cleanToolID() with escape error = nil")
	}
	if _, err := cleanToolID(""); err == nil {
		t.Fatal("cleanToolID() with empty error = nil")
	}
	if _, err := cleanToolID("demo"); err != nil {
		t.Fatalf("cleanToolID() error = %v", err)
	}
}

var _ = runtime.GOOS
