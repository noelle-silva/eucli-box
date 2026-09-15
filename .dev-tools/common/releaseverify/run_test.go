package releaseverify

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"eucli-box/pkg/workspace"
)

func TestPrepareRunRejectsOutsideDirectory(t *testing.T) {
	repositoryRoot := t.TempDir()
	runRoot := filepath.Join(t.TempDir(), "run-outside")
	if _, err := prepareRun(repositoryRoot, runRoot, "verify-release-build"); err == nil {
		t.Fatal("expected outside directory rejection")
	}
}

func TestPrepareRunRejectsExistingUnknownContent(t *testing.T) {
	repositoryRoot := t.TempDir()
	runRoot := filepath.Join(workspace.VerificationToolRoot(repositoryRoot, "verify-release-build"), "run-existing")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatalf("create run root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runRoot, "unknown.txt"), []byte("unknown"), 0o644); err != nil {
		t.Fatalf("create unknown file: %v", err)
	}
	if _, err := prepareRun(repositoryRoot, runRoot, "verify-release-build"); err == nil {
		t.Fatal("expected existing content rejection")
	}
}

func TestPrepareRunRetiresPreviousRuns(t *testing.T) {
	repositoryRoot := t.TempDir()
	toolRoot := workspace.VerificationToolRoot(repositoryRoot, "verify-release-build")
	currentRun := filepath.Join(toolRoot, "run-current")
	oldRun := filepath.Join(toolRoot, "run-old")
	for _, directory := range []string{
		filepath.Join(currentRun, "temp"),
		filepath.Join(currentRun, "cache"),
		filepath.Join(oldRun, "evidence"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create fixture %s: %v", directory, err)
		}
	}

	paths, err := prepareRun(repositoryRoot, currentRun, "verify-release-build")
	if err != nil {
		t.Fatalf("prepareRun() error = %v", err)
	}
	if paths.root != currentRun {
		t.Fatalf("paths.root = %s, want %s", paths.root, currentRun)
	}
	if _, err := os.Stat(oldRun); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous run survived preparation: %v", err)
	}
	for _, path := range []string{paths.inputs, paths.workspace, paths.environment, paths.work, paths.temp, paths.cache, paths.evidence, paths.sharedCache} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("prepared path missing: %s: %v", path, err)
		}
	}
}
