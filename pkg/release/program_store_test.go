package release

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func newTestProgramStore(t *testing.T) ProgramStore {
	t.Helper()
	root := filepath.Join(t.TempDir(), "demo")
	store, err := NewProgramStore(root, types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"})
	if err != nil {
		t.Fatalf("NewProgramStore() error = %v", err)
	}
	return store
}

func prepareTestVersion(t *testing.T, store ProgramStore, version string) (PreparedProgram, types.ReleaseProductRecord, []types.ReleaseFileRecord) {
	t.Helper()
	archivePath, manifest := makeTestToolArchive(t, "demo", version)
	extracted := filepath.Join(t.TempDir(), "extracted")
	if err := EnsureEmptyDirectory(extracted); err != nil {
		t.Fatalf("EnsureEmptyDirectory() error = %v", err)
	}
	if err := ExtractArchive(ExtractArchiveOptions{ArchivePath: archivePath, TargetDir: extracted}); err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}
	files, err := CollectFileRecords(extracted)
	if err != nil {
		t.Fatalf("CollectFileRecords() error = %v", err)
	}
	product := productFromManifest(manifest)
	prepared, err := store.PrepareVersion(context.Background(), extracted, product, files)
	if err != nil {
		t.Fatalf("PrepareVersion() error = %v", err)
	}
	return prepared, product, files
}

func TestProgramStoreRejectsForeignRoot(t *testing.T) {
	if _, err := NewProgramStore(filepath.Join(t.TempDir(), "other"), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"}); err == nil {
		t.Fatal("NewProgramStore() with foreign root error = nil")
	}
}

func TestProgramStoreCurrentUnknownBeforeActivate(t *testing.T) {
	store := newTestProgramStore(t)
	if _, err := store.Current(); err == nil {
		t.Fatal("Current() error = nil")
	}
}

func TestProgramStorePrepareActivateAndCurrent(t *testing.T) {
	store := newTestProgramStore(t)
	prepared, _, _ := prepareTestVersion(t, store, "0.1.0")
	if err := store.Activate(context.Background(), prepared, ""); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	current, err := store.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if current.Version != "0.1.0" || current.ProgramDirectory != prepared.Directory {
		t.Fatalf("current = %#v", current)
	}
}

func TestProgramStoreActivateRequiresPreviousMatch(t *testing.T) {
	store := newTestProgramStore(t)
	first, _, _ := prepareTestVersion(t, store, "0.1.0")
	if err := store.Activate(context.Background(), first, ""); err != nil {
		t.Fatalf("Activate(first) error = %v", err)
	}
	second, _, _ := prepareTestVersion(t, store, "0.1.1")
	if err := store.Activate(context.Background(), second, "0.1.0"); err != nil {
		t.Fatalf("Activate(second) error = %v", err)
	}
	current, _ := store.Current()
	if current.Version != "0.1.1" {
		t.Fatalf("current version = %s", current.Version)
	}
	third, _, _ := prepareTestVersion(t, store, "0.1.2")
	if err := store.Activate(context.Background(), third, "0.1.0"); err == nil {
		t.Fatal("Activate(third) with stale previous error = nil")
	}
}

func TestProgramStoreRestoreRecoversPreviousVersion(t *testing.T) {
	store := newTestProgramStore(t)
	first, _, _ := prepareTestVersion(t, store, "0.1.0")
	if err := store.Activate(context.Background(), first, ""); err != nil {
		t.Fatalf("Activate(first) error = %v", err)
	}
	second, _, _ := prepareTestVersion(t, store, "0.1.1")
	if err := store.Activate(context.Background(), second, "0.1.0"); err != nil {
		t.Fatalf("Activate(second) error = %v", err)
	}
	if err := store.Restore(context.Background(), "0.1.0"); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	current, err := store.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if current.Version != "0.1.0" {
		t.Fatalf("restored version = %s", current.Version)
	}
	if _, err := os.Stat(filepath.Join(store.root, "versions", "0.1.1", "definition.json")); err != nil {
		t.Fatalf("previous version removed: %v", err)
	}
}

func TestProgramStoreRestoreRejectsMissingVersion(t *testing.T) {
	store := newTestProgramStore(t)
	if err := store.Restore(context.Background(), "0.9.9"); err == nil {
		t.Fatal("Restore() with missing version error = nil")
	}
}

func TestProgramStoreRejectsTamperedVersionDirectory(t *testing.T) {
	store := newTestProgramStore(t)
	first, _, _ := prepareTestVersion(t, store, "0.1.0")
	if err := store.Activate(context.Background(), first, ""); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if err := os.Remove(filepath.Join(first.Directory, "release-product.json")); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if _, err := store.Current(); err == nil {
		t.Fatal("Current() with tampered version error = nil")
	}
}

func TestProgramStoreRejectsReuseWithDifferentContent(t *testing.T) {
	store := newTestProgramStore(t)
	archivePath, manifest := makeTestToolArchive(t, "demo", "0.1.0")
	extracted := filepath.Join(t.TempDir(), "extracted")
	if err := EnsureEmptyDirectory(extracted); err != nil {
		t.Fatalf("EnsureEmptyDirectory() error = %v", err)
	}
	if err := ExtractArchive(ExtractArchiveOptions{ArchivePath: archivePath, TargetDir: extracted}); err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}
	files, err := CollectFileRecords(extracted)
	if err != nil {
		t.Fatalf("CollectFileRecords() error = %v", err)
	}
	product := productFromManifest(manifest)
	if _, err := store.PrepareVersion(context.Background(), extracted, product, files); err != nil {
		t.Fatalf("PrepareVersion() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(extracted, "definition.json"), []byte(`{"different":true}`), 0o644); err != nil {
		t.Fatalf("tamper source: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(store.root, "versions", "0.1.0")); err != nil {
		t.Fatalf("remove version: %v", err)
	}
	if _, err := store.PrepareVersion(context.Background(), extracted, product, files); err == nil {
		t.Fatal("PrepareVersion() with different content error = nil")
	}
}

func TestProgramStoreRejectsReparsePointPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("reparse point check is Windows specific")
	}
	target := filepath.Join(t.TempDir(), "real")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	link := filepath.Join(t.TempDir(), "junction")
	if err := makeJunction(t, link, target); err != nil {
		t.Skipf("cannot create junction: %v", err)
	}
	archivePath, manifest := makeTestToolArchive(t, "demo", "0.1.0")
	extracted := filepath.Join(link, "extracted")
	if err := EnsureEmptyDirectory(extracted); err == nil {
		t.Fatal("EnsureEmptyDirectory() through junction error = nil")
	}
	store := newTestProgramStore(t)
	if _, err := store.PrepareVersion(context.Background(), extracted, productFromManifest(manifest), nil); err == nil {
		t.Fatal("PrepareVersion() through junction error = nil")
	}
	_ = archivePath
}

func makeJunction(t *testing.T, link string, target string) error {
	t.Helper()
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	return cmd.Run()
}

func TestProgramStoreUnknownCurrentRefusesBadIdentity(t *testing.T) {
	store := newTestProgramStore(t)
	record := currentProgramRecord{
		SchemaVersion:    currentProgramSchemaVersion,
		Artifact:         types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "other"},
		Version:          "0.1.0",
		Platform:         types.ReleasePlatformWindowsX64,
		ProgramDirectory: filepath.Join(store.root, "versions", "0.1.0"),
		Status:           "active",
	}
	if err := writeJSONAtomic(filepath.Join(store.root, "current.json"), record); err != nil {
		t.Fatalf("write current: %v", err)
	}
	if _, err := store.Current(); err == nil || !strings.Contains(err.Error(), "身份") {
		t.Fatalf("Current() error = %v", err)
	}
}

func TestProgramStoreUnknownCurrentRefusesStaleDirectory(t *testing.T) {
	store := newTestProgramStore(t)
	prepared, _, _ := prepareTestVersion(t, store, "0.1.0")
	if err := store.Activate(context.Background(), prepared, ""); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	record := currentProgramRecord{
		SchemaVersion:    currentProgramSchemaVersion,
		Artifact:         types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"},
		Version:          "0.1.0",
		Platform:         types.ReleasePlatformWindowsX64,
		ProgramDirectory: filepath.Join(t.TempDir(), "elsewhere"),
		Status:           "active",
	}
	if err := writeJSONAtomic(filepath.Join(store.root, "current.json"), record); err != nil {
		t.Fatalf("write current: %v", err)
	}
	if _, err := store.Current(); err == nil {
		t.Fatal("Current() error = nil")
	}
}
