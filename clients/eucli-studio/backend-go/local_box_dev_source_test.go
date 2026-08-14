//go:build windows && eucli_devbox

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/localrun"
	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

// devBoxFixture 装载开发体验测试的当前源码成品。
// 成品和环境变量由验证工具 verify-dev-box 按真实开发入口的命名注入。
type devBoxFixture struct {
	manifest     types.ReleaseManifest
	manifestPath string
	archivePath  string
}

func loadDevBoxFixture(t *testing.T) devBoxFixture {
	t.Helper()
	manifestPath := strings.TrimSpace(os.Getenv(devManifestEnvironment))
	archivePath := strings.TrimSpace(os.Getenv(devArchiveEnvironment))
	if manifestPath == "" || archivePath == "" {
		t.Fatal("开发业务端验证缺少本地成品资料")
	}
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := release.DecodeReleaseManifest(payload)
	if err != nil {
		t.Fatal(err)
	}
	return devBoxFixture{manifest: manifest, manifestPath: manifestPath, archivePath: archivePath}
}

// devBoxTestLayout 为每个测试建立独立的开发资料目录：
// 业务端资料根和客户端数据目录都从验证工具注入的开发根派生，测试之间互不共享。
type devBoxTestLayout struct {
	paths         localBoxPaths
	boxRoot       string
	clientDataDir string
}

func newDevBoxTestLayout(t *testing.T) devBoxTestLayout {
	t.Helper()
	boxRootEnvironment := strings.TrimSpace(os.Getenv(devBoxRootEnvironment))
	clientDataEnvironment := strings.TrimSpace(os.Getenv("FW_APP_DATA_DIR"))
	if boxRootEnvironment == "" || clientDataEnvironment == "" {
		t.Fatal("开发业务端验证缺少隔离资料根")
	}
	name := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(t.Name())
	boxRoot := filepath.Join(boxRootEnvironment, "tests", name)
	clientDataDir := filepath.Join(clientDataEnvironment, name)
	for _, directory := range []string{boxRoot, clientDataDir} {
		if err := os.RemoveAll(directory); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := newLocalBoxPathsWithRoot(clientDataDir, boxRoot)
	if err != nil {
		t.Fatal(err)
	}
	return devBoxTestLayout{paths: paths, boxRoot: boxRoot, clientDataDir: clientDataDir}
}

func (layout devBoxTestLayout) developmentManager(fixture devBoxFixture, manifestPath string, archivePath string) *localBoxManager {
	return newLocalBoxManager(layout.paths, newDevelopmentArtifactSource(manifestPath, archivePath), nil, nil, nil)
}

// TestDevBoxResolveSourceRequiresExplicitFlag 验证开发来源只能由显式环境变量开启：
// 无开发标记时保持正式来源；有开发标记但资料缺失时仍然建立开发来源，绝不回退正式核对。
func TestDevBoxResolveSourceRequiresExplicitFlag(t *testing.T) {
	fixture := loadDevBoxFixture(t)
	t.Setenv(devSourceEnvironment, "")
	t.Setenv(devManifestEnvironment, "")
	t.Setenv(devArchiveEnvironment, "")
	t.Setenv(devBoxRootEnvironment, "")
	source, checker, boxRoot, err := resolveLocalBoxSource()
	if err != nil {
		t.Fatal(err)
	}
	if source != nil || checker != nil || boxRoot != "" {
		t.Fatalf("无开发标记时应保持正式来源：source=%v checker=%v boxRoot=%q", source, checker, boxRoot)
	}

	t.Setenv(devSourceEnvironment, devSourceEnabled)
	source, checker, boxRoot, err = resolveLocalBoxSource()
	if err != nil {
		t.Fatal(err)
	}
	if source == nil || checker != nil {
		t.Fatalf("有开发标记时应建立开发来源且不回退正式核对：source=%v checker=%v", source, checker)
	}
	if source.Kind() != localBoxSourceDevelopment {
		t.Fatalf("source kind = %s", source.Kind())
	}

	t.Setenv(devManifestEnvironment, fixture.manifestPath)
	t.Setenv(devArchiveEnvironment, fixture.archivePath)
	t.Setenv(devBoxRootEnvironment, filepath.Dir(fixture.manifestPath))
	source, _, _, err = resolveLocalBoxSource()
	if err != nil {
		t.Fatal(err)
	}
	devSource, ok := source.(*developmentArtifactSource)
	if !ok || devSource.manifestPath == "" || devSource.archivePath == "" {
		t.Fatalf("开发来源未携带本地成品资料：%#v", source)
	}
}

// TestDevBoxLocalSourceInstallLifecycle 验证开发来源完成一次完整安装：
// 安装当前源码成品、来源为开发、安装记录落在开发资料根内、数据独占、真正退出。
func TestDevBoxLocalSourceInstallLifecycle(t *testing.T) {
	fixture := loadDevBoxFixture(t)
	layout := newDevBoxTestLayout(t)
	manager := layout.developmentManager(fixture, fixture.manifestPath, fixture.archivePath)

	states := make([]localBoxState, 0)
	manager.onState = func(state localBoxState) { states = append(states, state) }

	state, err := manager.install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Installed || !state.Connected || state.Status != localBoxStatusConnected {
		t.Fatalf("install state = %#v", state)
	}
	if current := manager.currentState(); current.Source != string(localBoxSourceDevelopment) {
		t.Fatalf("state source = %q, want development", current.Source)
	}
	if state.CurrentVersion != fixture.manifest.Version {
		t.Fatalf("installed version = %q, want %q", state.CurrentVersion, fixture.manifest.Version)
	}

	record := readDevBoxInstallRecord(t, layout.paths)
	if record == nil {
		t.Fatal("install record missing")
	}
	if record.Source != string(localBoxSourceDevelopment) {
		t.Fatalf("record source = %q, want development", record.Source)
	}
	if record.DataDir != layout.paths.dataDir || record.RuntimeDir != layout.paths.runtimeDir {
		t.Fatalf("record dirs = %q / %q", record.DataDir, record.RuntimeDir)
	}
	if !devBoxPathWithin(layout.boxRoot, record.DataDir) || !devBoxPathWithin(layout.boxRoot, record.RuntimeDir) {
		t.Fatalf("开发业务端资料越出开发资料根：%q", layout.boxRoot)
	}

	lock, err := localrun.AcquireDataLock(layout.paths.dataDir)
	if lock != nil {
		_ = lock.Release()
	}
	if err == nil || !strings.Contains(err.Error(), "LOCAL_BOX_DATA_IN_USE") {
		t.Fatalf("second data lock error = %v, want LOCAL_BOX_DATA_IN_USE", err)
	}

	state, err = manager.stop(context.Background())
	if err != nil || state.Status != localBoxStatusStopped {
		t.Fatalf("stop state=%#v err=%v", state, err)
	}
	if _, err := os.Stat(layout.paths.registrationPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("registration still exists: %v", err)
	}
}

// TestDevBoxMissingArtifactFailsWithoutFallback 验证本地成品缺失时明确失败：
// 不产生已安装记录，也不会去读取任何官方来源（管理器只有开发来源）。
func TestDevBoxMissingArtifactFailsWithoutFallback(t *testing.T) {
	layout := newDevBoxTestLayout(t)
	manager := layout.developmentManager(devBoxFixture{}, filepath.Join(layout.boxRoot, "missing.manifest.json"), filepath.Join(layout.boxRoot, "missing.zip"))
	state, err := manager.install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Error.Code != "LOCAL_BOX_DEV_ARTIFACT_UNAVAILABLE" || state.Installed {
		t.Fatalf("missing artifact state = %#v", state)
	}
	assertNoDevBoxInstallRecord(t, layout.paths)
}

// TestDevBoxArchiveTamperedFails 验证本地压缩包摘要与清单不一致时明确失败，
// 不留下已安装记录。
func TestDevBoxArchiveTamperedFails(t *testing.T) {
	fixture := loadDevBoxFixture(t)
	layout := newDevBoxTestLayout(t)
	tamperedArchive := devBoxTamperedCopy(t, fixture.archivePath)
	manager := layout.developmentManager(fixture, fixture.manifestPath, tamperedArchive)
	state, err := manager.install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Error.Code != "LOCAL_BOX_DEV_ARTIFACT_INVALID" || state.Installed {
		t.Fatalf("tampered archive state = %#v", state)
	}
	assertNoDevBoxInstallRecord(t, layout.paths)
}

// TestDevBoxManifestTamperedFails 验证本地清单丢失验证标记时明确失败，
// 不留下已安装记录。
func TestDevBoxManifestTamperedFails(t *testing.T) {
	fixture := loadDevBoxFixture(t)
	layout := newDevBoxTestLayout(t)
	tamperedManifest := devBoxModifiedManifestCopy(t, fixture.manifestPath, func(manifest *types.ReleaseManifest) {
		manifest.VerificationOnly = false
	})
	manager := layout.developmentManager(fixture, tamperedManifest, fixture.archivePath)
	state, err := manager.install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Error.Code != "LOCAL_BOX_DEV_ARTIFACT_INVALID" || state.Installed {
		t.Fatalf("tampered manifest state = %#v", state)
	}
	assertNoDevBoxInstallRecord(t, layout.paths)
}

// TestDevBoxSourceMismatchRejectsOfficialProgram 验证已安装开发来源业务端后，
// 官方来源不能继续使用该程序：状态明确报告来源不匹配，不静默顶替。
func TestDevBoxSourceMismatchRejectsOfficialProgram(t *testing.T) {
	fixture := loadDevBoxFixture(t)
	layout := newDevBoxTestLayout(t)
	devManager := layout.developmentManager(fixture, fixture.manifestPath, fixture.archivePath)
	state, err := devManager.install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Installed {
		t.Fatalf("dev install state = %#v", state)
	}
	state, err = devManager.stop(context.Background())
	if err != nil || state.Status != localBoxStatusStopped {
		t.Fatalf("dev stop state=%#v err=%v", state, err)
	}

	officialManager := newLocalBoxManager(layout.paths, devBoxOfficialStubSource{}, nil, nil, nil)
	state, err = officialManager.status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Error.Code != "LOCAL_BOX_SOURCE_MISMATCH" || state.Connected || state.Error.Phase != localBoxStatusStopped {
		t.Fatalf("official status on dev record = %#v", state)
	}
}

// devBoxOfficialStubSource 是一个绝不参与真实读取的官方来源桩：
// 任何被调用的读取动作都直接失败，用于证明官方来源不会在开发场景中被偷偷使用。
type devBoxOfficialStubSource struct{}

func (devBoxOfficialStubSource) Kind() localBoxSourceKind {
	return localBoxSourceOfficial
}

func (devBoxOfficialStubSource) LatestCandidate(context.Context, types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	return nil, errors.New("official stub should not be reached")
}

func (devBoxOfficialStubSource) AcquireArchive(context.Context, *releasecheck.ReleaseCandidate, string, func(localBoxProgress)) (string, error) {
	return "", errors.New("official stub should not be reached")
}

func (devBoxOfficialStubSource) ExpectedProduct(context.Context, *releasecheck.ReleaseCandidate) (types.ReleaseProductRecord, error) {
	return types.ReleaseProductRecord{}, errors.New("official stub should not be reached")
}

func readDevBoxInstallRecord(t *testing.T, paths localBoxPaths) *localBoxInstallRecord {
	t.Helper()
	payload, err := os.ReadFile(paths.installPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		t.Fatal(err)
	}
	var record localBoxInstallRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	return &record
}

func assertNoDevBoxInstallRecord(t *testing.T, paths localBoxPaths) {
	t.Helper()
	if _, err := os.Stat(paths.installPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("install record exists: %v", err)
	}
}

func devBoxPathWithin(root string, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	return candidate == root || strings.HasPrefix(candidate, root+string(filepath.Separator))
}

func devBoxTamperedCopy(t *testing.T, sourcePath string) string {
	t.Helper()
	payload, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := append([]byte(nil), payload...)
	tampered[len(tampered)-1] ^= 0xff
	target := filepath.Join(t.TempDir(), filepath.Base(sourcePath))
	if err := os.WriteFile(target, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	return target
}

func devBoxModifiedManifestCopy(t *testing.T, sourcePath string, modify func(*types.ReleaseManifest)) string {
	t.Helper()
	payload, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := release.DecodeReleaseManifest(payload)
	if err != nil {
		t.Fatal(err)
	}
	modify(&manifest)
	modified, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), filepath.Base(sourcePath))
	if err := os.WriteFile(target, modified, 0o600); err != nil {
		t.Fatal(err)
	}
	return target
}
