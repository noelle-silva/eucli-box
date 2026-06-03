package worktreeoverlay

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyRefreshClearOverlay(t *testing.T) {
	root := newGitFixture(t)
	source := addWorktree(t, root, "demo", "feature/demo")
	writeFile(t, filepath.Join(source, "app.txt"), "feature v1\n")
	writeFile(t, filepath.Join(source, "new.txt"), "new v1\n")
	removeFile(t, filepath.Join(source, "delete.txt"))

	var output bytes.Buffer
	if err := Run(context.Background(), []string{"apply", "demo"}, Options{WorkDir: root, Stdout: &output}); err != nil {
		t.Fatalf("apply error = %v", err)
	}
	assertFileContent(t, filepath.Join(root, "app.txt"), "feature v1\n")
	assertFileContent(t, filepath.Join(root, "new.txt"), "new v1\n")
	assertNoFile(t, filepath.Join(root, "delete.txt"))

	writeFile(t, filepath.Join(source, "app.txt"), "feature v2\n")
	removeFile(t, filepath.Join(source, "new.txt"))
	writeFile(t, filepath.Join(source, "new2.txt"), "new v2\n")

	output.Reset()
	if err := Run(context.Background(), []string{"refresh"}, Options{WorkDir: root, Stdout: &output}); err != nil {
		t.Fatalf("refresh error = %v", err)
	}
	assertFileContent(t, filepath.Join(root, "app.txt"), "feature v2\n")
	assertNoFile(t, filepath.Join(root, "new.txt"))
	assertFileContent(t, filepath.Join(root, "new2.txt"), "new v2\n")
	assertNoFile(t, filepath.Join(root, "delete.txt"))

	output.Reset()
	if err := Run(context.Background(), []string{"clear"}, Options{WorkDir: root, Stdout: &output}); err != nil {
		t.Fatalf("clear error = %v", err)
	}
	assertFileContent(t, filepath.Join(root, "app.txt"), "base\n")
	assertFileContent(t, filepath.Join(root, "delete.txt"), "delete me\n")
	assertNoFile(t, filepath.Join(root, "new2.txt"))
	assertCleanGitStatus(t, root)
}

func TestApplyKeepsMainOnlyPaths(t *testing.T) {
	root := newGitFixture(t)
	source := addWorktree(t, root, "demo", "feature/demo")
	writeFile(t, filepath.Join(source, "app.txt"), "feature\n")
	writeFile(t, filepath.Join(root, "main-only.txt"), "main only\n")
	runGit(t, root, "add", "main-only.txt")
	runGit(t, root, "commit", "-m", "main only")

	if err := Run(context.Background(), []string{"apply", "demo"}, Options{WorkDir: root, Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatalf("apply error = %v", err)
	}
	assertFileContent(t, filepath.Join(root, "app.txt"), "feature\n")
	assertFileContent(t, filepath.Join(root, "main-only.txt"), "main only\n")
}

func TestApplyRejectsPathChangedOnTarget(t *testing.T) {
	root := newGitFixture(t)
	source := addWorktree(t, root, "demo", "feature/demo")
	writeFile(t, filepath.Join(source, "app.txt"), "feature\n")
	writeFile(t, filepath.Join(root, "app.txt"), "main change\n")
	runGit(t, root, "add", "app.txt")
	runGit(t, root, "commit", "-m", "main changes app")

	err := Run(context.Background(), []string{"apply", "demo"}, Options{WorkDir: root, Stdout: &bytes.Buffer{}})
	if err == nil {
		t.Fatalf("apply succeeded unexpectedly")
	}
	if !strings.Contains(err.Error(), "source and target both changed") {
		t.Fatalf("error = %v", err)
	}
}

func TestClearRejectsEditedOverlay(t *testing.T) {
	root := newGitFixture(t)
	source := addWorktree(t, root, "demo", "feature/demo")
	writeFile(t, filepath.Join(source, "app.txt"), "feature\n")
	if err := Run(context.Background(), []string{"apply", "demo"}, Options{WorkDir: root, Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatalf("apply error = %v", err)
	}
	writeFile(t, filepath.Join(root, "app.txt"), "manual edit\n")
	err := Run(context.Background(), []string{"clear"}, Options{WorkDir: root, Stdout: &bytes.Buffer{}})
	if err == nil {
		t.Fatalf("clear succeeded unexpectedly")
	}
	if !strings.Contains(err.Error(), "changed after apply") {
		t.Fatalf("error = %v", err)
	}
}

func newGitFixture(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	writeFile(t, filepath.Join(root, ".gitignore"), ".worktrees/\n")
	writeFile(t, filepath.Join(root, "app.txt"), "base\n")
	writeFile(t, filepath.Join(root, "delete.txt"), "delete me\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	runGit(t, root, "branch", "-M", "main")
	return root
}

func addWorktree(t *testing.T, root string, name string, branch string) string {
	t.Helper()
	worktreesRoot := filepath.Join(root, ".worktrees")
	if err := os.MkdirAll(worktreesRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", worktreesRoot, err)
	}
	source := filepath.Join(worktreesRoot, name)
	runGit(t, root, "worktree", "add", "-b", branch, source, "main")
	return source
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s error = %v\n%s", strings.Join(args, " "), err, output)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove(%s) error = %v", path, err)
	}
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if string(payload) != want {
		t.Fatalf("ReadFile(%s) = %q, want %q", path, payload, want)
	}
}

func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v, want not exist", path, err)
	}
}

func assertCleanGitStatus(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=normal")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status error = %v\n%s", err, output)
	}
	if strings.TrimSpace(string(output)) != "" {
		t.Fatalf("git status = %q, want clean", output)
	}
}
