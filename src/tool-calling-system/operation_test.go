package toolcalling

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

	mu       sync.Mutex
	archives map[string][]byte
	gate     chan struct{}
	chunks   int
	chunkGap time.Duration
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
	fixture := &toolOperationFixture{t: t, programRoot: programRoot, dataRoot: dataRoot, candidates: &fakeCandidateReader{candidates: map[string]*releasecheck.ReleaseCandidate{}}, archives: map[string][]byte{}}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.servePackage))
	t.Cleanup(fixture.server.Close)
	t.Cleanup(fixture.releaseDownloads)
	fixture.system, err = NewSystem(Config{
		BoxVersion:  "0.1.0",
		ProgramRoot: programRoot,
		Candidates:  fixture.candidates,
		HTTPClient:  fixture.server.Client(),
	}, &fakePermission{}, storage)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	fixture.realSystem = fixture.system.(*system)
	return fixture
}

// makeToolCandidate 构造工具成品并注册为官方候选；binary 为空字符串时写入损坏二进制。
func (f *toolOperationFixture) makeToolCandidate(id string, version string, brokenBinary bool) {
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
func (f *toolOperationFixture) holdDownloads() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gate == nil {
		f.gate = make(chan struct{})
	}
}

func (f *toolOperationFixture) releaseDownloads() {
	f.mu.Lock()
	gate := f.gate
	f.gate = nil
	f.mu.Unlock()
	if gate != nil {
		close(gate)
	}
}

// setSlowDownload 让压缩包按块缓慢写出，用于观察下载进度上报。
func (f *toolOperationFixture) setSlowDownload(chunks int, gap time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chunks = chunks
	f.chunkGap = gap
}

func (f *toolOperationFixture) servePackage(w http.ResponseWriter, r *http.Request) {
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

// waitToolTerminal 等待工具操作进入终态；用于异步任务测试。
func (f *toolOperationFixture) waitToolTerminal(t *testing.T, toolID string) types.ArtifactInstallState {
	t.Helper()
	return waitSystemToolTerminal(t, f.system, toolID)
}

func waitSystemToolTerminal(t *testing.T, system System, toolID string) types.ArtifactInstallState {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		state, err := system.ToolInstallState(context.Background(), toolID)
		if err != nil {
			t.Fatalf("ToolInstallState() error = %v", err)
		}
		if isTerminalArtifactStatus(state.Status) {
			return state
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("tool operation did not reach terminal state in time")
	return types.ArtifactInstallState{}
}

// installToolAndWait 发起安装并等待终态（同步失败直接返回）。
func (f *toolOperationFixture) installToolAndWait(t *testing.T, toolID string) types.ArtifactInstallState {
	t.Helper()
	state, err := f.system.InstallTool(context.Background(), toolID)
	if err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	if isTerminalArtifactStatus(state.Status) {
		return state
	}
	return f.waitToolTerminal(t, toolID)
}

// updateToolAndWait 发起更新并等待终态（同步失败直接返回）。
func (f *toolOperationFixture) updateToolAndWait(t *testing.T, toolID string) types.ArtifactInstallState {
	t.Helper()
	state, err := f.system.UpdateTool(context.Background(), toolID)
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}
	if isTerminalArtifactStatus(state.Status) {
		return state
	}
	return f.waitToolTerminal(t, toolID)
}

// waitToolPhase 等待工具任务进入指定阶段。
func (f *toolOperationFixture) waitToolPhase(t *testing.T, toolID string, phase string) types.ArtifactInstallState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state, err := f.system.ToolInstallState(context.Background(), toolID)
		if err != nil {
			t.Fatalf("ToolInstallState() error = %v", err)
		}
		if state.Phase == phase {
			return state
		}
		if isTerminalArtifactStatus(state.Status) {
			t.Fatalf("operation reached terminal status before phase %s: %#v", phase, state)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("tool operation did not reach phase %s in time", phase)
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

// TestInstallToolFromLocalStore 验证本地商店候选读取器驱动的完整安装：
// 本地货架成品 zip 经 AcquireAndValidatePackage（Local 放行）入程序区并激活。
func TestInstallToolFromLocalStore(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("dev-demo", "0.1.1", false)
	storeRoot := filepath.Join(t.TempDir(), "local-store")
	versionDir := filepath.Join(storeRoot, "ai-tools", "dev-demo", "0.1.1")
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
		SchemaVersion:  release.ReleaseManifestSchemaVersion,
		Artifact:       types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "dev-demo"},
		Version:        "0.1.1",
		Platform:       types.ReleasePlatformWindowsX64,
		OfficialSource: "https://github.com/noelle-silva/eucli-box-ai-tools",
		Compatibility:  &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		Source:         types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
		TagName:        "v0.1.1",
		Archive:        types.ReleaseFileRecord{Name: archiveName, Size: size, SHA256: sha256},
		Files:          fileRecords,
	}
	manifestPayload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "tool-dev-demo_0.1.1_windows-x64.manifest.json"), manifestPayload, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	reader, err := releasecheck.NewLocalSourceReader(storeRoot)
	if err != nil {
		t.Fatalf("NewLocalSourceReader() error = %v", err)
	}
	devSystem, err := NewSystem(Config{
		BoxVersion:  "0.1.0",
		ProgramRoot: fixture.programRoot,
		Candidates:  reader,
		HTTPClient:  http.DefaultClient,
	}, &fakePermission{}, fixture.realSystem.storage)
	if err != nil {
		t.Fatalf("NewSystem(dev) error = %v", err)
	}
	state, err := devSystem.InstallTool(context.Background(), "dev-demo")
	if err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	if !isTerminalArtifactStatus(state.Status) {
		state = waitSystemToolTerminal(t, devSystem, "dev-demo")
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
	fixture.makeToolCandidate("demo", "0.1.0", false)
	state := fixture.installToolAndWait(t, "demo")
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

// TestInstallToolReclaimsWorkDirOnSuccess 验证操作进入成功终态后本轮工作目录立即回收。
func TestInstallToolReclaimsWorkDirOnSuccess(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false)
	state := fixture.installToolAndWait(t, "demo")
	if state.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", state)
	}
	fixture.assertWorkRootEmpty(t, "demo")
	if _, err := os.Stat(filepath.Join(fixture.programRoot, "demo", "operation.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful operation record should be removed, err = %v", err)
	}
}

// TestUpdateToolReclaimsWorkDirOnProbeFailure 验证失败终态同样回收工作目录，失败记录保留供状态展示。
func TestUpdateToolReclaimsWorkDirOnProbeFailure(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false)
	fixture.installToolAndWait(t, "demo")
	fixture.makeToolCandidate("demo", "0.1.1", true)
	state := fixture.updateToolAndWait(t, "demo")
	if state.Status != types.ArtifactStatusActive || state.Error.Code != types.ArtifactErrorProbeFailed {
		t.Fatalf("state = %#v", state)
	}
	fixture.assertWorkRootEmpty(t, "demo")
	record, err := release.ReadOperationRecord(filepath.Join(fixture.programRoot, "demo", "operation.json"))
	if err != nil {
		t.Fatalf("failed operation record missing: %v", err)
	}
	if record.Result != release.OperationResultFailed || record.ErrorCode != types.ArtifactErrorProbeFailed {
		t.Fatalf("record = %#v", record)
	}
}

// TestToolOperationSweepsStaleWorkDirs 验证下一次操作开始时清扫遗留的操作工作目录，
// 且不触碰非操作前缀的条目。
func TestToolOperationSweepsStaleWorkDirs(t *testing.T) {
	fixture := newToolOperationFixture(t)
	workRoot := filepath.Join(fixture.programRoot, "demo", "work")
	staleDir := filepath.Join(workRoot, "tool-operation-stale")
	if err := os.MkdirAll(filepath.Join(staleDir, "download"), 0o755); err != nil {
		t.Fatalf("mkdir stale work dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "download", "leftover.bin"), []byte("junk"), 0o644); err != nil {
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

	fixture.makeToolCandidate("demo", "0.1.0", false)
	fixture.installToolAndWait(t, "demo")

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

func TestInstallToolRejectsIncompatibleCandidateBeforeDownload(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false)
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
	fixture.makeToolCandidate("demo", "0.1.0", false)
	if _, err := fixture.system.InstallTool(context.Background(), "demo"); err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	fixture.makeToolCandidate("demo", "0.1.1", false)
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

// assertWorkRootEmpty 断言某一工具 work/ 下没有任何残留条目；目录不存在同样视为通过。
func (f *toolOperationFixture) assertWorkRootEmpty(t *testing.T, toolID string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(f.programRoot, toolID, "work"))
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

// TestActivityBeginUpdateReportsOccupancyImmediately 验证 035：有真实执行时
// 更新闸门立即报告占用，不等待、不自动排队。
func TestActivityBeginUpdateReportsOccupancyImmediately(t *testing.T) {
	activity := &toolActivity{}
	activity.acquire()
	started := time.Now()
	code := activity.beginUpdate("op-1", nil)
	if code != types.ArtifactErrorToolActive {
		t.Fatalf("code = %s", code)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("returned too slowly: %s", time.Since(started))
	}
	activity.release()
	updatingCode := activity.beginUpdate("op-2", nil)
	if updatingCode != "" {
		t.Fatalf("update after release code = %s", updatingCode)
	}
	activity.endUpdate()
}

func TestUpdateToolRestoresPreviousVersionOnProbeFailure(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false)
	fixture.installToolAndWait(t, "demo")
	fixture.makeToolCandidate("demo", "0.1.1", true)
	state := fixture.updateToolAndWait(t, "demo")
	if state.Status != types.ArtifactStatusActive || state.Error.Code != types.ArtifactErrorProbeFailed {
		t.Fatalf("state = %#v", state)
	}
	if err := fixture.runTool(t, "demo", false); err != nil {
		t.Fatalf("previous version cannot run: %v", err)
	}
	state, err := fixture.system.ToolInstallState(context.Background(), "demo")
	if err != nil {
		t.Fatalf("ToolInstallState() error = %v", err)
	}
	if state.CurrentVersion != "0.1.0" || state.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", state)
	}
}

func TestToolInstallStateRecoversInterruptedSwitch(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false)
	fixture.installToolAndWait(t, "demo")
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
	fixture.makeToolCandidate("demo", "0.1.0", false)
	fixture.installToolAndWait(t, "demo")
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

// TestInstallToolReturnsRunningStateImmediately 验证安装请求立即返回运行态，
// 不再等待整个下载与切换流程结束。
func TestInstallToolReturnsRunningStateImmediately(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false)
	fixture.holdDownloads()
	state, err := fixture.system.InstallTool(context.Background(), "demo")
	if err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	if isTerminalArtifactStatus(state.Status) {
		t.Fatalf("expected running state immediately, got %#v", state)
	}
	if state.OperationID == "" {
		t.Fatalf("operationId missing: %#v", state)
	}
	fixture.releaseDownloads()
	terminal := fixture.waitToolTerminal(t, "demo")
	if terminal.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", terminal)
	}
}

// TestToolInstallProgressReported 验证下载阶段的已下载字节与总量被状态查询上报。
func TestToolInstallProgressReported(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false)
	fixture.setSlowDownload(6, 30*time.Millisecond)
	if _, err := fixture.system.InstallTool(context.Background(), "demo"); err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	seen := false
	for time.Now().Before(deadline) {
		state, err := fixture.system.ToolInstallState(context.Background(), "demo")
		if err != nil {
			t.Fatalf("ToolInstallState() error = %v", err)
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
	terminal := fixture.waitToolTerminal(t, "demo")
	if terminal.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", terminal)
	}
}

// TestCancelToolOperationDuringDownload 验证下载阶段取消：终态为已取消、
// 工作目录清理干净，且同一条目可重新安装成功。
func TestCancelToolOperationDuringDownload(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false)
	fixture.holdDownloads()
	state, err := fixture.system.InstallTool(context.Background(), "demo")
	if err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	if isTerminalArtifactStatus(state.Status) {
		t.Fatalf("expected running state, got %#v", state)
	}
	fixture.waitToolPhase(t, "demo", types.ArtifactPhaseDownload)
	cancelled, err := fixture.system.CancelToolOperation(context.Background(), "demo")
	if err != nil {
		t.Fatalf("CancelToolOperation() error = %v", err)
	}
	fixture.releaseDownloads()
	if cancelled.Status != types.ArtifactStatusCancelled {
		t.Fatalf("state = %#v", cancelled)
	}
	fixture.assertWorkRootEmpty(t, "demo")
	restoreState := fixture.installToolAndWait(t, "demo")
	if restoreState.Status != types.ArtifactStatusActive {
		t.Fatalf("re-install state = %#v", restoreState)
	}
}

// TestCancelToolOperationRejectedAfterSwitch 验证进入切换阶段后拒绝取消。
func TestCancelToolOperationRejectedAfterSwitch(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false)
	fixture.installToolAndWait(t, "demo")
	activity := fixture.realSystem.activityFor("demo")
	if code := activity.beginUpdate("manual-switch-op", nil); code != "" {
		t.Fatalf("beginUpdate() = %s", code)
	}
	defer activity.endUpdate()
	activity.updatePhase(types.ArtifactPhaseSwitch)
	state, err := fixture.system.CancelToolOperation(context.Background(), "demo")
	if err != nil {
		t.Fatalf("CancelToolOperation() error = %v", err)
	}
	if state.Status != types.ArtifactStatusBlocked || state.Error.Code != types.ArtifactErrorCancelRejected {
		t.Fatalf("state = %#v", state)
	}
}

// TestInstallContinuesAfterRequestContextCancelled 验证任务与请求生命周期解绑：
// 请求 context 取消后，后台安装继续推进到成功终态。
func TestInstallContinuesAfterRequestContextCancelled(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false)
	fixture.setSlowDownload(4, 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := fixture.system.InstallTool(ctx, "demo"); err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	cancel()
	terminal := fixture.waitToolTerminal(t, "demo")
	if terminal.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", terminal)
	}
}

// TestConcurrentInstallsOfDifferentTools 验证不同工具的安装各自独立、可同时进行。
func TestConcurrentInstallsOfDifferentTools(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("alpha", "0.1.0", false)
	fixture.makeToolCandidate("beta", "0.1.0", false)
	fixture.holdDownloads()
	if _, err := fixture.system.InstallTool(context.Background(), "alpha"); err != nil {
		t.Fatalf("InstallTool(alpha) error = %v", err)
	}
	if _, err := fixture.system.InstallTool(context.Background(), "beta"); err != nil {
		t.Fatalf("InstallTool(beta) error = %v", err)
	}
	fixture.waitToolPhase(t, "alpha", types.ArtifactPhaseDownload)
	fixture.waitToolPhase(t, "beta", types.ArtifactPhaseDownload)
	fixture.releaseDownloads()
	alphaState := fixture.waitToolTerminal(t, "alpha")
	betaState := fixture.waitToolTerminal(t, "beta")
	if alphaState.Status != types.ArtifactStatusActive || betaState.Status != types.ArtifactStatusActive {
		t.Fatalf("alpha = %#v, beta = %#v", alphaState, betaState)
	}
}

// TestListToolOperationsReturnsRunningAndTerminal 验证批量操作查询同时返回
// 运行中的任务与落盘的终态记录。
func TestListToolOperationsReturnsRunningAndTerminal(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false)
	fixture.installToolAndWait(t, "demo")
	fixture.makeToolCandidate("demo", "0.1.1", true)
	fixture.updateToolAndWait(t, "demo")

	fixture.makeToolCandidate("slow", "0.1.0", false)
	fixture.holdDownloads()
	if _, err := fixture.system.InstallTool(context.Background(), "slow"); err != nil {
		t.Fatalf("InstallTool(slow) error = %v", err)
	}
	fixture.waitToolPhase(t, "slow", types.ArtifactPhaseDownload)

	operations, err := fixture.system.ListToolOperations(context.Background())
	if err != nil {
		t.Fatalf("ListToolOperations() error = %v", err)
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
	fixture.waitToolTerminal(t, "slow")
}

// TestInstallToolRejectsDuplicateWhileRunning 验证运行中的任务被重复发起时拒绝，
// 且不会误触中断恢复清理运行中的工作目录。
func TestInstallToolRejectsDuplicateWhileRunning(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidate("demo", "0.1.0", false)
	fixture.holdDownloads()
	first, err := fixture.system.InstallTool(context.Background(), "demo")
	if err != nil {
		t.Fatalf("InstallTool() error = %v", err)
	}
	if isTerminalArtifactStatus(first.Status) {
		t.Fatalf("expected running state, got %#v", first)
	}
	fixture.waitToolPhase(t, "demo", types.ArtifactPhaseDownload)
	second, err := fixture.system.InstallTool(context.Background(), "demo")
	if err != nil {
		t.Fatalf("InstallTool() duplicate error = %v", err)
	}
	if second.Status != types.ArtifactStatusBlocked || second.Error.Code != types.ArtifactErrorUpdateInProgress {
		t.Fatalf("duplicate state = %#v", second)
	}
	fixture.releaseDownloads()
	terminal := fixture.waitToolTerminal(t, "demo")
	if terminal.Status != types.ArtifactStatusActive {
		t.Fatalf("state = %#v", terminal)
	}
}

var _ = runtime.GOOS
