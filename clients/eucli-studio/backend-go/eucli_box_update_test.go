//go:build windows && eucli_box_update

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

const (
	updateHarnessEnvironment     = "EUCLI_BOX_UPDATE_HARNESS"
	updateClientDataEnvironment  = "EUCLI_BOX_UPDATE_CLIENT_DATA_DIR"
	updateWorkEnvironment        = "EUCLI_BOX_UPDATE_WORK_DIR"
	updateBoxArchiveEnvironment  = "EUCLI_BOX_UPDATE_BOX_ARCHIVE"
	updateBoxManifestEnvironment = "EUCLI_BOX_UPDATE_BOX_MANIFEST"
	updateExperienceEnvironment  = "EUCLI_BOX_UPDATE_EXPERIENCE"
)

type boxHarnessScenario struct {
	ActiveWork           int
	ShutdownStatus       int
	ExitAfterReady       bool
	MigrationBehavior    string
	RecoverOnSecondStart bool
	ClientCompatibility  *types.EucliBoxCompatibility
}

// updateBoxFixture 是一次业务端更新验证的隔离资料。
type updateBoxFixture struct {
	t            *testing.T
	paths        localBoxPaths
	harnessExe   string
	workRoot     string
	realArchive  string
	realManifest string
	managers     []*localBoxManager
}

func newUpdateBoxFixture(t *testing.T) *updateBoxFixture {
	t.Helper()
	harnessExe := os.Getenv(updateHarnessEnvironment)
	clientDataBase := os.Getenv(updateClientDataEnvironment)
	workRoot := os.Getenv(updateWorkEnvironment)
	if harnessExe == "" || clientDataBase == "" || workRoot == "" {
		t.Fatal("业务端更新验证缺少替身、隔离客户端数据目录或工作目录资料")
	}
	name := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(t.Name())
	paths, err := newLocalBoxPaths(filepath.Join(clientDataBase, name))
	if err != nil {
		t.Fatal(err)
	}
	fixture := &updateBoxFixture{
		t: t, paths: paths, harnessExe: harnessExe, workRoot: workRoot,
		realArchive:  os.Getenv(updateBoxArchiveEnvironment),
		realManifest: os.Getenv(updateBoxManifestEnvironment),
	}
	t.Cleanup(fixture.cleanup)
	return fixture
}

// cleanup 停止测试期间启动的全部业务端进程，避免占用验证目录。
func (f *updateBoxFixture) cleanup() {
	for _, manager := range f.managers {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, _ = manager.stop(ctx)
		cancel()
	}
}

func (f *updateBoxFixture) track(manager *localBoxManager) *localBoxManager {
	f.managers = append(f.managers, manager)
	return manager
}

// makeHarnessBox 建立替身成品目录（eucli-box.exe + scenario.json + release-product.json）。
func (f *updateBoxFixture) makeHarnessBox(version string, initialDataVersion string, dataVersion string, cfg boxHarnessScenario) string {
	f.t.Helper()
	caseDir := filepath.Join(f.workRoot, strings.ReplaceAll(f.t.Name(), " ", "-"))
	boxDir := filepath.Join(caseDir, "box-"+version)
	if err := os.MkdirAll(boxDir, 0o755); err != nil {
		f.t.Fatalf("mkdir box dir: %v", err)
	}
	payload, err := os.ReadFile(f.harnessExe)
	if err != nil {
		f.t.Fatalf("read harness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(boxDir, "eucli-box.exe"), payload, 0o755); err != nil {
		f.t.Fatalf("write harness exe: %v", err)
	}
	scenario := map[string]any{
		"version": version, "initialDataVersion": initialDataVersion, "dataVersion": dataVersion,
		"activeWork":           cfg.ActiveWork,
		"shutdownStatus":       cfg.ShutdownStatus,
		"exitAfterReady":       cfg.ExitAfterReady,
		"migrationBehavior":    cfg.MigrationBehavior,
		"recoverOnSecondStart": cfg.RecoverOnSecondStart,
	}
	if cfg.ClientCompatibility != nil {
		scenario["clientCompatibility"] = cfg.ClientCompatibility
	}
	scenarioPayload, err := json.MarshalIndent(scenario, "", "  ")
	if err != nil {
		f.t.Fatalf("marshal scenario: %v", err)
	}
	if err := os.WriteFile(filepath.Join(boxDir, "scenario.json"), scenarioPayload, 0o644); err != nil {
		f.t.Fatalf("write scenario: %v", err)
	}
	product := types.ReleaseProductRecord{
		SchemaVersion:  1,
		Artifact:       types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox},
		Version:        version,
		Platform:       types.ReleasePlatformWindowsX64,
		OfficialSource: "https://github.com/noelle-silva/eucli-box",
		Source: types.ReleaseSourceRecord{
			Repository: "https://github.com/noelle-silva/eucli-box",
			Commit:     "0123456789abcdef0123456789abcdef01234567",
			Recorded:   true,
		},
		DataVersion:      dataVersion,
		VerificationOnly: true,
	}
	productPayload, err := json.MarshalIndent(product, "", "  ")
	if err != nil {
		f.t.Fatalf("marshal product: %v", err)
	}
	if err := os.WriteFile(filepath.Join(boxDir, "release-product.json"), productPayload, 0o644); err != nil {
		f.t.Fatalf("write product record: %v", err)
	}
	if err := os.WriteFile(filepath.Join(boxDir, "README.md"), []byte("# boxharness\n"), 0o644); err != nil {
		f.t.Fatalf("write readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(boxDir, "CHANGELOG.md"), []byte("# 变更记录\n"), 0o644); err != nil {
		f.t.Fatalf("write changelog: %v", err)
	}
	return boxDir
}

// packageHarnessRelease 把替身成品目录打包成发行压缩包并生成开发来源清单。
func (f *updateBoxFixture) packageHarnessRelease(boxDir string, version string) (manifestPath string, archivePath string) {
	f.t.Helper()
	caseDir := filepath.Dir(boxDir)
	archiveName := release.ArchiveFileName(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox}, version)
	archivePath = filepath.Join(caseDir, archiveName)
	archivePayload := f.zipHarnessBox(boxDir)
	if err := os.WriteFile(archivePath, archivePayload, 0o644); err != nil {
		f.t.Fatalf("write archive: %v", err)
	}
	manifest, err := release.DecodeReleaseProductRecord(mustReadFile(f.t, filepath.Join(boxDir, "release-product.json")))
	if err != nil {
		f.t.Fatalf("decode product record: %v", err)
	}
	files := []types.ReleaseFileRecord{
		{Name: "CHANGELOG.md", Size: int64(len(mustReadFile(f.t, filepath.Join(boxDir, "CHANGELOG.md")))), SHA256: release.SHA256(mustReadFile(f.t, filepath.Join(boxDir, "CHANGELOG.md")))},
		{Name: "README.md", Size: int64(len(mustReadFile(f.t, filepath.Join(boxDir, "README.md")))), SHA256: release.SHA256(mustReadFile(f.t, filepath.Join(boxDir, "README.md")))},
		{Name: "eucli-box.exe", Size: int64(len(mustReadFile(f.t, filepath.Join(boxDir, "eucli-box.exe")))), SHA256: release.SHA256(mustReadFile(f.t, filepath.Join(boxDir, "eucli-box.exe")))},
		{Name: "release-product.json", Size: int64(len(mustReadFile(f.t, filepath.Join(boxDir, "release-product.json")))), SHA256: release.SHA256(mustReadFile(f.t, filepath.Join(boxDir, "release-product.json")))},
		{Name: "scenario.json", Size: int64(len(mustReadFile(f.t, filepath.Join(boxDir, "scenario.json")))), SHA256: release.SHA256(mustReadFile(f.t, filepath.Join(boxDir, "scenario.json")))},
	}
	manifestRecord := types.ReleaseManifest{
		SchemaVersion:    1,
		Artifact:         manifest.Artifact,
		Version:          manifest.Version,
		Platform:         manifest.Platform,
		TagName:          "v" + manifest.Version,
		OfficialSource:   manifest.OfficialSource,
		Source:           manifest.Source,
		DataVersion:      manifest.DataVersion,
		VerificationOnly: true,
		Archive: types.ReleaseFileRecord{
			Name: archiveName, Size: int64(len(archivePayload)), SHA256: release.SHA256(archivePayload),
		},
		Files: files,
	}
	manifestPayload, err := json.MarshalIndent(manifestRecord, "", "  ")
	if err != nil {
		f.t.Fatalf("marshal manifest: %v", err)
	}
	manifestPath = filepath.Join(caseDir, "manifest-"+version+".json")
	if err := os.WriteFile(manifestPath, manifestPayload, 0o644); err != nil {
		f.t.Fatalf("write manifest: %v", err)
	}
	return manifestPath, archivePath
}

func (f *updateBoxFixture) zipHarnessBox(boxDir string) []byte {
	f.t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range []string{"CHANGELOG.md", "README.md", "eucli-box.exe", "release-product.json", "scenario.json"} {
		payload := mustReadFile(f.t, filepath.Join(boxDir, name))
		entry, err := writer.Create(name)
		if err != nil {
			f.t.Fatalf("create zip entry: %v", err)
		}
		if _, err := entry.Write(payload); err != nil {
			f.t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		f.t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}

// managerFor 用指定开发来源构造管理器。
func (f *updateBoxFixture) managerFor(manifestPath string, archivePath string) *localBoxManager {
	f.t.Helper()
	source := newDevelopmentArtifactSource(manifestPath, archivePath)
	return f.track(newLocalBoxManager(f.paths, source, nil, nil, nil))
}

func (f *updateBoxFixture) installBox(manager *localBoxManager, manifestPath string, archivePath string) {
	f.t.Helper()
	manager.source = newDevelopmentArtifactSource(manifestPath, archivePath)
	state, err := manager.install(context.Background())
	if err != nil {
		f.t.Fatalf("install error = %v", err)
	}
	if !state.Installed || !state.Connected || state.Status != localBoxStatusConnected {
		f.t.Fatalf("install state = %#v", state)
	}
}

func (f *updateBoxFixture) switchSource(manager *localBoxManager, manifestPath string, archivePath string) {
	f.t.Helper()
	manager.source = newDevelopmentArtifactSource(manifestPath, archivePath)
}

func (f *updateBoxFixture) store() release.ProgramStore {
	f.t.Helper()
	store, err := f.managerStore()
	if err != nil {
		f.t.Fatalf("program store error = %v", err)
	}
	return store
}

func (f *updateBoxFixture) managerStore() (release.ProgramStore, error) {
	return release.NewProgramStore(f.paths.programStoreDir,
		types.ReleaseArtifactIdentity{Kind: localBoxArtifactID, ID: localBoxArtifactID})
}

func (f *updateBoxFixture) assertCurrentVersion(want string) {
	f.t.Helper()
	current, err := f.store().Current()
	if err != nil {
		f.t.Fatalf("store.Current() error = %v", err)
	}
	if current.Version != want {
		f.t.Fatalf("current version = %q, want %q", current.Version, want)
	}
}

func (f *updateBoxFixture) assertNoOperationRecord() {
	f.t.Helper()
	if _, err := os.Stat(filepath.Join(f.paths.programStoreDir, "operation.json")); !errors.Is(err, os.ErrNotExist) {
		f.t.Fatalf("operation record must not exist: %v", err)
	}
}

func (f *updateBoxFixture) assertNoUpdateWorkDirs() {
	f.t.Helper()
	entries, err := os.ReadDir(f.paths.workDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		f.t.Fatalf("read work dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "update-") {
			f.t.Fatalf("update work dir exists: %s", entry.Name())
		}
	}
}

func (f *updateBoxFixture) assertStateError(state localBoxState, wantCode string) {
	f.t.Helper()
	if state.Error.Code != wantCode {
		f.t.Fatalf("state error code = %q, want %q (message: %s)", state.Error.Code, wantCode, state.Error.Message)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return payload
}

func TestBoxUpdateScenarioSuccessful(t *testing.T) {
	fixture := newUpdateBoxFixture(t)
	manager := fixture.managerFor("", "")
	oldBox := fixture.makeHarnessBox("0.1.0", "1.0.0", "1.0.0", boxHarnessScenario{})
	oldManifest, oldArchive := fixture.packageHarnessRelease(oldBox, "0.1.0")
	fixture.installBox(manager, oldManifest, oldArchive)
	dataBefore := mustReadFile(t, filepath.Join(fixture.paths.dataDir, "meta", "version.json"))

	newBox := fixture.makeHarnessBox("0.2.0", "1.0.0", "1.0.0", boxHarnessScenario{})
	newManifest, newArchive := fixture.packageHarnessRelease(newBox, "0.2.0")
	fixture.switchSource(manager, newManifest, newArchive)
	state, err := manager.update(context.Background(), testClientRelease())
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	if !state.Connected || state.Status != localBoxStatusConnected || state.CurrentVersion != "0.2.0" {
		t.Fatalf("update state = %#v", state)
	}
	fixture.assertCurrentVersion("0.2.0")
	if _, err := os.Stat(filepath.Join(fixture.paths.programStoreDir, "versions", "0.1.0")); err != nil {
		t.Fatalf("previous version directory must be kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.paths.programStoreDir, "versions", "0.2.0")); err != nil {
		t.Fatalf("new version directory must exist: %v", err)
	}
	fixture.assertNoOperationRecord()
	fixture.assertNoUpdateWorkDirs()
	dataAfter := mustReadFile(t, filepath.Join(fixture.paths.dataDir, "meta", "version.json"))
	if !bytes.Equal(dataBefore, dataAfter) {
		t.Fatalf("data version file was rewritten by update flow")
	}
}

func TestBoxUpdateScenarioNoNewVersion(t *testing.T) {
	fixture := newUpdateBoxFixture(t)
	manager := fixture.managerFor("", "")
	oldBox := fixture.makeHarnessBox("0.1.0", "1.0.0", "1.0.0", boxHarnessScenario{})
	oldManifest, oldArchive := fixture.packageHarnessRelease(oldBox, "0.1.0")
	fixture.installBox(manager, oldManifest, oldArchive)
	state, err := manager.update(context.Background(), testClientRelease())
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	if state.Error.Code != "" || state.CurrentVersion != "0.1.0" {
		t.Fatalf("update state = %#v", state)
	}
	fixture.assertNoOperationRecord()
	fixture.assertNoUpdateWorkDirs()
}

func TestBoxUpdateScenarioBlockedByActiveWork(t *testing.T) {
	fixture := newUpdateBoxFixture(t)
	manager := fixture.managerFor("", "")
	oldBox := fixture.makeHarnessBox("0.1.0", "1.0.0", "1.0.0", boxHarnessScenario{ActiveWork: 2})
	oldManifest, oldArchive := fixture.packageHarnessRelease(oldBox, "0.1.0")
	fixture.installBox(manager, oldManifest, oldArchive)
	newBox := fixture.makeHarnessBox("0.2.0", "1.0.0", "1.0.0", boxHarnessScenario{})
	newManifest, newArchive := fixture.packageHarnessRelease(newBox, "0.2.0")
	fixture.switchSource(manager, newManifest, newArchive)
	state, err := manager.update(context.Background(), testClientRelease())
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	fixture.assertStateError(state, "LOCAL_BOX_UPDATE_BLOCKED")
	if !strings.Contains(state.Error.Message, "2") {
		t.Fatalf("blocked message must mention work count: %s", state.Error.Message)
	}
	fixture.assertNoOperationRecord()
	fixture.assertNoUpdateWorkDirs()
	fixture.assertCurrentVersion("0.1.0")
	if manager.currentProcess() == nil {
		t.Fatalf("old box process must keep running")
	}
}

func TestBoxUpdateScenarioCorruptDownload(t *testing.T) {
	fixture := newUpdateBoxFixture(t)
	manager := fixture.managerFor("", "")
	oldBox := fixture.makeHarnessBox("0.1.0", "1.0.0", "1.0.0", boxHarnessScenario{})
	oldManifest, oldArchive := fixture.packageHarnessRelease(oldBox, "0.1.0")
	fixture.installBox(manager, oldManifest, oldArchive)
	// 新版压缩包内容与清单不一致：篡改压缩包字节但保留清单
	newBox := fixture.makeHarnessBox("0.2.0", "1.0.0", "1.0.0", boxHarnessScenario{})
	_, newArchive := fixture.packageHarnessRelease(newBox, "0.2.0")
	tampered := append([]byte(nil), mustReadFile(t, newArchive)...)
	tampered[len(tampered)-1] ^= 0xff
	if err := os.WriteFile(newArchive, tampered, 0o644); err != nil {
		t.Fatalf("tamper archive: %v", err)
	}
	newManifestPath := filepath.Join(filepath.Dir(newArchive), "manifest-0.2.0.json")
	fixture.switchSource(manager, newManifestPath, newArchive)
	state, err := manager.update(context.Background(), testClientRelease())
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	if state.Error.Code != "LOCAL_BOX_DEV_ARTIFACT_INVALID" && state.Error.Code != "LOCAL_BOX_PACKAGE_INVALID" {
		t.Fatalf("state error code = %q (message: %s)", state.Error.Code, state.Error.Message)
	}
	fixture.assertCurrentVersion("0.1.0")
	fixture.assertNoOperationRecord()
}

func TestBoxUpdateScenarioStopRejected(t *testing.T) {
	fixture := newUpdateBoxFixture(t)
	manager := fixture.managerFor("", "")
	oldBox := fixture.makeHarnessBox("0.1.0", "1.0.0", "1.0.0", boxHarnessScenario{ShutdownStatus: http.StatusConflict})
	oldManifest, oldArchive := fixture.packageHarnessRelease(oldBox, "0.1.0")
	fixture.installBox(manager, oldManifest, oldArchive)
	newBox := fixture.makeHarnessBox("0.2.0", "1.0.0", "1.0.0", boxHarnessScenario{})
	newManifest, newArchive := fixture.packageHarnessRelease(newBox, "0.2.0")
	fixture.switchSource(manager, newManifest, newArchive)
	state, err := manager.update(context.Background(), testClientRelease())
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	fixture.assertStateError(state, "LOCAL_BOX_UPDATE_BLOCKED")
	fixture.assertCurrentVersion("0.1.0")
	if manager.currentProcess() == nil {
		t.Fatalf("old box process must keep running after rejected stop")
	}
}

func TestBoxUpdateScenarioProbeFailureRestoresPrevious(t *testing.T) {
	fixture := newUpdateBoxFixture(t)
	manager := fixture.managerFor("", "")
	oldBox := fixture.makeHarnessBox("0.1.0", "1.0.0", "1.0.0", boxHarnessScenario{})
	oldManifest, oldArchive := fixture.packageHarnessRelease(oldBox, "0.1.0")
	fixture.installBox(manager, oldManifest, oldArchive)
	// 新版启动即退出且无迁移痕迹：数据未改变 → 自动恢复上一版
	newBox := fixture.makeHarnessBox("0.2.0", "1.0.0", "1.0.0", boxHarnessScenario{ExitAfterReady: true})
	newManifest, newArchive := fixture.packageHarnessRelease(newBox, "0.2.0")
	fixture.switchSource(manager, newManifest, newArchive)
	state, err := manager.update(context.Background(), testClientRelease())
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	if !state.Connected || state.CurrentVersion != "0.1.0" {
		t.Fatalf("update state = %#v", state)
	}
	fixture.assertCurrentVersion("0.1.0")
	record, err := release.ReadOperationRecord(filepath.Join(fixture.paths.programStoreDir, "operation.json"))
	if err != nil {
		t.Fatalf("operation record must be kept with failed result: %v", err)
	}
	if record.Result != release.OperationResultFailed {
		t.Fatalf("operation result = %q, want failed", record.Result)
	}
}

func TestBoxUpdateScenarioProbeFailureKeepsNewVersion(t *testing.T) {
	fixture := newUpdateBoxFixture(t)
	manager := fixture.managerFor("", "")
	oldBox := fixture.makeHarnessBox("0.1.0", "1.0.0", "1.0.0", boxHarnessScenario{})
	oldManifest, oldArchive := fixture.packageHarnessRelease(oldBox, "0.1.0")
	fixture.installBox(manager, oldManifest, oldArchive)
	// 新版迁移完成（数据已到 1.2.0）后启动失败：保持新版不回滚
	newBox := fixture.makeHarnessBox("0.2.0", "1.0.0", "1.2.0", boxHarnessScenario{MigrationBehavior: "migrate-then-exit"})
	newManifest, newArchive := fixture.packageHarnessRelease(newBox, "0.2.0")
	fixture.switchSource(manager, newManifest, newArchive)
	state, err := manager.update(context.Background(), testClientRelease())
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	fixture.assertStateError(state, "LOCAL_BOX_UPDATE_FAILED")
	fixture.assertCurrentVersion("0.2.0")
}

func TestBoxUpdateScenarioMigrationInterruptedRecovered(t *testing.T) {
	fixture := newUpdateBoxFixture(t)
	manager := fixture.managerFor("", "")
	oldBox := fixture.makeHarnessBox("0.1.0", "1.0.0", "1.0.0", boxHarnessScenario{})
	oldManifest, oldArchive := fixture.packageHarnessRelease(oldBox, "0.1.0")
	fixture.installBox(manager, oldManifest, oldArchive)
	// 新版迁移中断（写 process.json 后崩溃）；第二次启动完成恢复后 ready
	newBox := fixture.makeHarnessBox("0.2.0", "1.0.0", "1.2.0", boxHarnessScenario{MigrationBehavior: "crash-with-process", RecoverOnSecondStart: true})
	newManifest, newArchive := fixture.packageHarnessRelease(newBox, "0.2.0")
	fixture.switchSource(manager, newManifest, newArchive)
	state, err := manager.update(context.Background(), testClientRelease())
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	if !state.Connected || state.CurrentVersion != "0.2.0" {
		t.Fatalf("update state = %#v", state)
	}
	if state.Error.Code != "LOCAL_BOX_MIGRATION_RECOVERED" {
		t.Fatalf("state error code = %q, want LOCAL_BOX_MIGRATION_RECOVERED", state.Error.Code)
	}
	fixture.assertNoOperationRecord()
}

func TestBoxUpdateScenarioRecoveryFailedUnsafe(t *testing.T) {
	fixture := newUpdateBoxFixture(t)
	manager := fixture.managerFor("", "")
	oldBox := fixture.makeHarnessBox("0.1.0", "1.0.0", "1.0.0", boxHarnessScenario{})
	oldManifest, oldArchive := fixture.packageHarnessRelease(oldBox, "0.1.0")
	fixture.installBox(manager, oldManifest, oldArchive)
	// 迁移中断且恢复也失败：不启动任何版本，现场完整保留
	newBox := fixture.makeHarnessBox("0.2.0", "1.0.0", "1.2.0", boxHarnessScenario{MigrationBehavior: "recovery-failed"})
	newManifest, newArchive := fixture.packageHarnessRelease(newBox, "0.2.0")
	fixture.switchSource(manager, newManifest, newArchive)
	state, err := manager.update(context.Background(), testClientRelease())
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	fixture.assertStateError(state, "LOCAL_BOX_DATA_UNSAFE")
	if manager.currentProcess() != nil {
		t.Fatalf("no version may be running after unsafe result")
	}
	workspaceDir := filepath.Join(filepath.Dir(fixture.paths.dataDir), filepath.Base(fixture.paths.dataDir)+".migration")
	if _, statErr := os.Stat(filepath.Join(workspaceDir, "process.json")); statErr != nil {
		t.Fatalf("process record must be retained: %v", statErr)
	}
}

func TestBoxUpdateScenarioRealProductChain(t *testing.T) {
	fixture := newUpdateBoxFixture(t)
	if fixture.realArchive == "" || fixture.realManifest == "" {
		t.Skip("缺少真实成品资料，跳过真实成品更新链")
	}
	manager := fixture.managerFor("", "")
	oldBox := fixture.makeHarnessBox("0.0.1", "1.0.0", "1.0.0", boxHarnessScenario{})
	oldManifest, oldArchive := fixture.packageHarnessRelease(oldBox, "0.0.1")
	fixture.installBox(manager, oldManifest, oldArchive)
	fixture.switchSource(manager, fixture.realManifest, fixture.realArchive)
	state, err := manager.update(context.Background(), testClientRelease())
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	if !state.Connected || state.Status != localBoxStatusConnected {
		t.Fatalf("update state = %#v", state)
	}
	manifest, err := release.DecodeReleaseManifest(mustReadFile(t, fixture.realManifest))
	if err != nil {
		t.Fatalf("decode real manifest: %v", err)
	}
	if state.CurrentVersion != manifest.Version {
		t.Fatalf("current version = %q, want real product %q", state.CurrentVersion, manifest.Version)
	}
}

func TestBoxUpdateScenarioPostUpdateCompatibilityRerun(t *testing.T) {
	fixture := newUpdateBoxFixture(t)
	if fixture.realArchive == "" || fixture.realManifest == "" {
		t.Skip("缺少真实成品资料，跳过适用重判")
	}
	// 预置声明不适用范围的工具样例
	seedIncompatibleTool(t, fixture.paths.rootDir)
	manager := fixture.managerFor("", "")
	oldBox := fixture.makeHarnessBox("0.0.1", "1.0.0", "1.0.0", boxHarnessScenario{})
	oldManifest, oldArchive := fixture.packageHarnessRelease(oldBox, "0.0.1")
	fixture.installBox(manager, oldManifest, oldArchive)
	fixture.switchSource(manager, fixture.realManifest, fixture.realArchive)
	state, err := manager.update(context.Background(), testClientRelease())
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	if !state.Connected {
		t.Fatalf("update state = %#v", state)
	}
	tools := fetchBoxTools(t, manager.currentConnection())
	found := false
	for _, tool := range tools {
		if tool.ID != "sample-tool" {
			continue
		}
		found = true
		if tool.Status != types.ToolAvailabilityUnavailable {
			t.Fatalf("sample-tool status = %q, want unavailable", tool.Status)
		}
		if tool.Compatibility.Compatible {
			t.Fatalf("sample-tool must be incompatible: %#v", tool.Compatibility)
		}
	}
	if !found {
		t.Fatalf("sample-tool must still be listed: %#v", tools)
	}
}

func TestBoxUpdateScenarioClientIncompatibleMaintenance(t *testing.T) {
	fixture := newUpdateBoxFixture(t)
	config, err := newConfigStore(filepath.Join(fixture.paths.clientDataDir))
	if err != nil {
		t.Fatalf("newConfigStore() error = %v", err)
	}
	oldBox := fixture.makeHarnessBox("0.1.0", "1.0.0", "1.0.0", boxHarnessScenario{
		ClientCompatibility: &types.EucliBoxCompatibility{MinimumVersion: "0.9.0", MaximumVersionExclusive: "1.0.0"},
	})
	oldManifest, oldArchive := fixture.packageHarnessRelease(oldBox, "0.1.0")
	source := newDevelopmentArtifactSource(oldManifest, oldArchive)
	svc, err := newService(config, testClientRelease(), nil, source, fakeClientReleaseChecker{}, "")
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, _ = svc.localBox.stop(ctx)
		cancel()
	})
	state, err := svc.localBox.install(context.Background())
	if err != nil || !state.Connected {
		t.Fatalf("install state=%#v err=%v", state, err)
	}
	bootstrap, err := svc.bootstrap(context.Background())
	if err != nil {
		t.Fatalf("bootstrap error = %v", err)
	}
	if bootstrap.BusinessAvailable {
		t.Fatalf("businessAvailable must be false: %#v", bootstrap)
	}
	_, err = svc.dispatch(context.Background(), "eucli.eb.request", json.RawMessage(`{"method":"GET","path":"/api/roles"}`))
	assertErrorCode(t, err, "EUCLI_BOX_INCOMPATIBLE")
	if _, err := svc.dispatch(context.Background(), "localBox.status", nil); err != nil {
		t.Fatalf("localBox.status must stay available: %v", err)
	}
	if _, err := svc.dispatch(context.Background(), "localBox.update", nil); err != nil {
		t.Fatalf("localBox.update must stay available: %v", err)
	}
}

func TestBoxUpdateExperiencePrep(t *testing.T) {
	if os.Getenv(updateExperienceEnvironment) != "1" {
		t.Skip("仅体验模式执行")
	}
	fixture := newUpdateBoxFixture(t)
	if fixture.realArchive == "" || fixture.realManifest == "" {
		t.Fatal("体验准备缺少真实新成品资料")
	}
	oldBox := fixture.makeHarnessBox("0.0.1", "1.0.0", "1.0.0", boxHarnessScenario{})
	oldManifest, oldArchive := fixture.packageHarnessRelease(oldBox, "0.0.1")
	manager := fixture.managerFor("", "")
	fixture.installBox(manager, oldManifest, oldArchive)
	if manager.currentConnection() == nil {
		t.Fatalf("体验准备必须保持已连接状态")
	}
}

// seedIncompatibleTool 在隔离程序根预置一个声明不适用范围的工具样例。
func seedIncompatibleTool(t *testing.T, boxRoot string) {
	t.Helper()
	toolRoot := filepath.Join(boxRoot, "tools", "sample-tool")
	store, err := release.NewProgramStore(toolRoot, types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "sample-tool"})
	if err != nil {
		t.Fatalf("NewProgramStore() error = %v", err)
	}
	versionDir := filepath.Join(toolRoot, "versions", "0.1.0")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatalf("mkdir version dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "eucli-box.exe"), []byte("fake-tool"), 0o755); err != nil {
		t.Fatalf("write tool binary: %v", err)
	}
	definition := map[string]any{
		"id": "sample-tool", "name": "sample-tool", "description": "不适用样例工具",
		"version":               "0.1.0",
		"eucliBoxCompatibility": map[string]string{"minimumVersion": "0.9.0", "maximumVersionExclusive": "1.0.0"},
		"defaultInvocationMode": "sync", "type": "local", "bodyDirectory": ".",
		"binaries": []map[string]string{{"goos": "windows", "goarch": "amd64", "path": "eucli-box.exe"}},
	}
	definitionPayload, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "definition.json"), definitionPayload, 0o644); err != nil {
		t.Fatalf("write definition: %v", err)
	}
	product := types.ReleaseProductRecord{
		SchemaVersion:  1,
		Artifact:       types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "sample-tool"},
		Version:        "0.1.0",
		Platform:       types.ReleasePlatformWindowsX64,
		OfficialSource: "https://github.com/noelle-silva/eucli-box-ai-tools",
		Compatibility:  &types.EucliBoxCompatibility{MinimumVersion: "0.9.0", MaximumVersionExclusive: "1.0.0"},
		Source: types.ReleaseSourceRecord{
			Repository: "https://github.com/noelle-silva/eucli-box-ai-tools",
			Commit:     "0123456789abcdef0123456789abcdef01234567",
			Recorded:   true,
		},
		VerificationOnly: true,
	}
	productPayload, err := json.MarshalIndent(product, "", "  ")
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "release-product.json"), productPayload, 0o644); err != nil {
		t.Fatalf("write product record: %v", err)
	}
	files, err := release.CollectFileRecords(versionDir)
	if err != nil {
		t.Fatalf("CollectFileRecords() error = %v", err)
	}
	if err := store.Activate(context.Background(), release.PreparedProgram{Version: "0.1.0", Directory: versionDir, Files: files}, ""); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
}

// fetchBoxTools 通过受托连接读取业务端工具列表。
func fetchBoxTools(t *testing.T, connection *boxConnection) []struct {
	ID            string                    `json:"id"`
	Status        string                    `json:"status"`
	Compatibility types.CompatibilityStatus `json:"compatibility"`
} {
	t.Helper()
	if connection == nil {
		t.Fatal("no box connection")
	}
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(connection.BaseURL, "/")+"/api/tools", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+connection.Credential)
	response, err := (&http.Client{}).Do(request)
	if err != nil {
		t.Fatalf("list tools request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list tools status = %d", response.StatusCode)
	}
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read tools response: %v", err)
	}
	var envelope struct {
		Data []struct {
			ID            string                    `json:"id"`
			Status        string                    `json:"status"`
			Compatibility types.CompatibilityStatus `json:"compatibility"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode tools response: %v", err)
	}
	return envelope.Data
}
