package datamigration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func testWorkspace(t *testing.T, dataDir string) workspace {
	t.Helper()
	w, err := newWorkspace(dataDir)
	if err != nil {
		t.Fatalf("newWorkspace() error = %v", err)
	}
	if err := w.ensure(); err != nil {
		t.Fatalf("workspace ensure error = %v", err)
	}
	return w
}

func snapshotDir(t *testing.T, root string) string {
	t.Helper()
	records := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
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
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		digest := sha256.New()
		size, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		records = append(records, fmt.Sprintf("%s\x00%d\x00%s\n", filepath.ToSlash(relative), size, hex.EncodeToString(digest.Sum(nil))))
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	sort.Strings(records)
	digest := sha256.Sum256([]byte(strings.Join(records, "")))
	return hex.EncodeToString(digest[:])
}

func TestEstablishBackupCoversScopeAndWritesManifest(t *testing.T) {
	dataDir := t.TempDir()
	writeTestFile(t, filepath.Join(dataDir, "meta", "version.json"), `{"version":"1.0.0"}`+"\n")
	writeTestFile(t, filepath.Join(dataDir, "meta", "counter.json"), `{"count":0}`+"\n")
	writeTestFile(t, filepath.Join(dataDir, "meta", "stamp.json"), `{"stamp":"old"}`+"\n")
	writeTestFile(t, filepath.Join(dataDir, "sessions", "keep.json"), `{"keep":true}`+"\n")
	w := testWorkspace(t, dataDir)

	scope := []string{"meta/counter.json", versionFileScope}
	manifest, err := establishBackup(context.Background(), dataDir, w, "20260813T100000.000000000Z", scope)
	if err != nil {
		t.Fatalf("establishBackup() error = %v", err)
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("manifest files = %#v", manifest.Files)
	}
	paths := map[string]bool{}
	for _, file := range manifest.Files {
		paths[file.Path] = true
	}
	if !paths["meta/version.json"] || !paths["meta/counter.json"] {
		t.Fatalf("backup scope mismatch: %#v", paths)
	}
	if manifest.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d", manifest.SchemaVersion)
	}
	backupCounter := filepath.Join(w.backupDataDir("20260813T100000.000000000Z"), "meta", "counter.json")
	if _, err := os.Stat(backupCounter); err != nil {
		t.Fatalf("backup copy missing: %v", err)
	}
	if _, err := os.Stat(w.manifestFile("20260813T100000.000000000Z")); err != nil {
		t.Fatalf("manifest file missing: %v", err)
	}
}

func TestRestoreFromBackupReturnsDataToOriginal(t *testing.T) {
	dataDir := t.TempDir()
	writeTestFile(t, filepath.Join(dataDir, "meta", "version.json"), `{"version":"1.0.0"}`+"\n")
	writeTestFile(t, filepath.Join(dataDir, "meta", "counter.json"), `{"count":0}`+"\n")
	writeTestFile(t, filepath.Join(dataDir, "sessions", "keep.json"), `{"keep":true}`+"\n")
	w := testWorkspace(t, dataDir)
	before := snapshotDir(t, dataDir)

	scope := []string{"meta"}
	if _, err := establishBackup(context.Background(), dataDir, w, "20260813T100000.000000000Z", scope); err != nil {
		t.Fatalf("establishBackup() error = %v", err)
	}

	writeTestFile(t, filepath.Join(dataDir, "meta", "counter.json"), `{"count":2}`+"\n")
	writeTestFile(t, filepath.Join(dataDir, "meta", "stamp.json"), `{"stamp":"1.2.0"}`+"\n")
	writeTestFile(t, filepath.Join(dataDir, "meta", "extra", "new.json"), `{"new":true}`+"\n")

	if err := restoreFromBackup(context.Background(), dataDir, w, "20260813T100000.000000000Z", scope); err != nil {
		t.Fatalf("restoreFromBackup() error = %v", err)
	}
	after := snapshotDir(t, dataDir)
	if before != after {
		t.Fatalf("data directory changed after restore:\nbefore=%s\nafter=%s", before, after)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "meta", "stamp.json")); !os.IsNotExist(err) {
		t.Fatalf("migration-created file still exists")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "meta", "extra")); !os.IsNotExist(err) {
		t.Fatalf("migration-created directory still exists")
	}
}

func TestRestoreFromBackupFailureKeepsScene(t *testing.T) {
	dataDir := t.TempDir()
	writeTestFile(t, filepath.Join(dataDir, "meta", "version.json"), `{"version":"1.0.0"}`+"\n")
	writeTestFile(t, filepath.Join(dataDir, "meta", "counter.json"), `{"count":0}`+"\n")
	w := testWorkspace(t, dataDir)

	scope := []string{"meta"}
	runID := "20260813T100000.000000000Z"
	if _, err := establishBackup(context.Background(), dataDir, w, runID, scope); err != nil {
		t.Fatalf("establishBackup() error = %v", err)
	}
	writeTestFile(t, filepath.Join(dataDir, "meta", "counter.json"), `{"count":1}`+"\n")
	if err := os.Remove(filepath.Join(w.backupDataDir(runID), "meta", "counter.json")); err != nil {
		t.Fatalf("remove backup file: %v", err)
	}
	if err := restoreFromBackup(context.Background(), dataDir, w, runID, scope); err == nil {
		t.Fatal("restoreFromBackup() error = nil, want failure")
	}
	if _, err := os.Stat(w.backupRunDir(runID)); err != nil {
		t.Fatalf("backup scene was cleaned despite failure: %v", err)
	}
}

func TestEstablishBackupRejectsReparsePoint(t *testing.T) {
	dataDir := t.TempDir()
	writeTestFile(t, filepath.Join(dataDir, "meta", "version.json"), `{"version":"1.0.0"}`+"\n")
	writeTestFile(t, filepath.Join(dataDir, "meta", "counter.json"), `{"count":0}`+"\n")
	link := filepath.Join(dataDir, "meta", "link")
	if err := os.Symlink(filepath.Join(dataDir, "meta", "counter.json"), link); err != nil {
		t.Skipf("cannot create symlink on this platform: %v", err)
	}
	w := testWorkspace(t, dataDir)
	_, err := establishBackup(context.Background(), dataDir, w, "20260813T100000.000000000Z", []string{"meta"})
	var appErr interface{ Error() string }
	_ = appErr
	if err == nil {
		t.Fatal("establishBackup() error = nil, want reparse point failure")
	}
	assertMigrationErrorCode(t, err, "migration.prepare_failed")
}

func TestReadBackupManifestRejectsUnsafePaths(t *testing.T) {
	dataDir := t.TempDir()
	w := testWorkspace(t, dataDir)
	for _, unsafe := range []string{"../evil.txt", "meta/../../evil.txt", "/absolute.txt", "meta/../evil.txt"} {
		manifest := backupManifest{
			SchemaVersion: 1,
			Files: []backupManifestFile{{
				Path:      unsafe,
				SizeBytes: 1,
				SHA256:    strings.Repeat("a", 64),
			}},
			TotalBytes: 1,
		}
		path := w.manifestFile("20260813T100000.000000000Z")
		if err := writeBackupManifest(path, manifest); err != nil {
			t.Fatalf("writeBackupManifest() error = %v", err)
		}
		if _, err := readBackupManifest(path); err == nil {
			t.Fatalf("manifest path %q was accepted", unsafe)
		}
	}
}

func TestReadBackupManifestRejectsInvalidDigest(t *testing.T) {
	dataDir := t.TempDir()
	w := testWorkspace(t, dataDir)
	manifest := backupManifest{
		SchemaVersion: 1,
		Files: []backupManifestFile{{
			Path:      "meta/version.json",
			SizeBytes: 1,
			SHA256:    "not-a-digest",
		}},
		TotalBytes: 1,
	}
	path := w.manifestFile("20260813T100000.000000000Z")
	if err := writeBackupManifest(path, manifest); err != nil {
		t.Fatalf("writeBackupManifest() error = %v", err)
	}
	if _, err := readBackupManifest(path); err == nil {
		t.Fatal("invalid digest manifest was accepted")
	}
}

func TestCheckDiskSpaceBehavior(t *testing.T) {
	if runtime.GOOS == "windows" {
		if err := checkDiskSpace(t.TempDir(), ^uint64(0)); err == nil {
			t.Fatal("checkDiskSpace() error = nil, want insufficient space failure")
		}
		if err := checkDiskSpace(t.TempDir(), 1); err != nil {
			t.Fatalf("checkDiskSpace(1) error = %v", err)
		}
	} else {
		if err := checkDiskSpace(t.TempDir(), ^uint64(0)); err != nil {
			t.Fatalf("checkDiskSpace() error = %v on non-Windows", err)
		}
	}
}

func TestRemoveBackupRunCleansEmptyBackupRoot(t *testing.T) {
	dataDir := t.TempDir()
	writeTestFile(t, filepath.Join(dataDir, "meta", "version.json"), `{"version":"1.0.0"}`+"\n")
	w := testWorkspace(t, dataDir)
	runID := "20260813T100000.000000000Z"
	if _, err := establishBackup(context.Background(), dataDir, w, runID, []string{"meta"}); err != nil {
		t.Fatalf("establishBackup() error = %v", err)
	}
	if err := removeBackupRun(w, runID); err != nil {
		t.Fatalf("removeBackupRun() error = %v", err)
	}
	if _, err := os.Stat(w.backupRunDir(runID)); !os.IsNotExist(err) {
		t.Fatalf("backup run directory still exists")
	}
	if _, err := os.Stat(w.backupRoot()); !os.IsNotExist(err) {
		t.Fatalf("empty backup root still exists")
	}
}

func TestWorkspaceDirIsSiblingOfDataDir(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "root", "data")
	want := filepath.Join(filepath.Dir(dataDir), "data.migration")
	if got := WorkspaceDir(dataDir); got != want {
		t.Fatalf("WorkspaceDir() = %q, want %q", got, want)
	}
}

func TestScopeMatching(t *testing.T) {
	scope := []string{"meta/counter.json", "meta/stamps"}
	matches := map[string]bool{
		"meta/counter.json":            true,
		"meta/counter.json.bak":        false,
		"meta/stamps/a.json":           true,
		"meta/stamps/deep/b.json":      true,
		"meta/stamps-backup/c.json":    false,
		"meta/version.json":            false,
	}
	for path, want := range matches {
		if got := scopeMatches(scope, path); got != want {
			t.Fatalf("scopeMatches(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestManifestJSONShape(t *testing.T) {
	manifest := backupManifest{
		SchemaVersion: 1,
		Files: []backupManifestFile{{Path: "meta/version.json", SizeBytes: 96, SHA256: strings.Repeat("ab", 32)}},
		TotalBytes:    96,
		CreatedAt:     "2026-08-13T10:00:00.000000000Z",
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["schemaVersion"] != float64(1) {
		t.Fatalf("schemaVersion = %#v", decoded["schemaVersion"])
	}
	files, ok := decoded["files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("files = %#v", decoded["files"])
	}
	first, ok := files[0].(map[string]any)
	if !ok || first["path"] != "meta/version.json" || first["sizeBytes"] != float64(96) {
		t.Fatalf("first file = %#v", files[0])
	}
}
