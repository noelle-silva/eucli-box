package releaseverify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareRunRejectsOutsideDirectory(t *testing.T) {
	repositoryRoot := t.TempDir()
	runRoot := filepath.Join(t.TempDir(), "run-outside")
	if _, err := prepareRun(repositoryRoot, runRoot, "01"); err == nil {
		t.Fatal("expected outside directory rejection")
	}
}

func TestPrepareRunRejectsExistingUnknownContent(t *testing.T) {
	repositoryRoot := t.TempDir()
	runRoot := filepath.Join(repositoryRoot, ".release", "verification", "stage-01", "run-existing")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatalf("create run root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runRoot, "unknown.txt"), []byte("unknown"), 0o644); err != nil {
		t.Fatalf("create unknown file: %v", err)
	}
	if _, err := prepareRun(repositoryRoot, runRoot, "01"); err == nil {
		t.Fatal("expected existing content rejection")
	}
}
