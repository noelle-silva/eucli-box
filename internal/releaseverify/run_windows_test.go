//go:build windows

package releaseverify

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"eucli-box/pkg/workspace"
)

func TestPrepareRunRejectsReparsePointRunRoot(t *testing.T) {
	repositoryRoot := t.TempDir()
	stageRoot := workspace.VerificationStageRoot(repositoryRoot, "01")
	runRoot := filepath.Join(stageRoot, "run-reparse")
	externalRoot := filepath.Join(repositoryRoot, "outside")
	if err := os.MkdirAll(externalRoot, 0o755); err != nil {
		t.Fatalf("create external root: %v", err)
	}
	marker := filepath.Join(externalRoot, "marker.txt")
	if err := os.WriteFile(marker, []byte("must-remain"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		t.Fatalf("create stage root: %v", err)
	}
	if output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", runRoot, externalRoot).CombinedOutput(); err != nil {
		t.Fatalf("create junction: %v\n%s", err, output)
	}
	t.Cleanup(func() {
		_ = os.Remove(runRoot)
		_ = os.RemoveAll(externalRoot)
	})

	if _, err := prepareRun(repositoryRoot, runRoot, "01"); err == nil {
		t.Fatal("expected reparse point run root rejection")
	}
	if payload, err := os.ReadFile(marker); err != nil || string(payload) != "must-remain" {
		t.Fatalf("external marker changed: %q, %v", payload, err)
	}
}

func TestPrepareRunRejectsReparsePointInRunPath(t *testing.T) {
	repositoryRoot := t.TempDir()
	externalRoot := t.TempDir()
	workspaceRoot := filepath.Join(repositoryRoot, workspace.WorkspaceDirectory)
	if err := os.WriteFile(filepath.Join(externalRoot, "marker.txt"), []byte("must-remain"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", workspaceRoot, externalRoot).CombinedOutput(); err != nil {
		t.Fatalf("create junction: %v\n%s", err, output)
	}
	t.Cleanup(func() { _ = os.Remove(workspaceRoot) })

	runRoot := filepath.Join(workspace.VerificationStageRoot(repositoryRoot, "01"), "run-parent-reparse")
	if _, err := prepareRun(repositoryRoot, runRoot, "01"); err == nil {
		t.Fatal("expected reparse point run path rejection")
	}
	marker := filepath.Join(externalRoot, "marker.txt")
	if payload, err := os.ReadFile(marker); err != nil || string(payload) != "must-remain" {
		t.Fatalf("external marker changed: %q, %v", payload, err)
	}
}
