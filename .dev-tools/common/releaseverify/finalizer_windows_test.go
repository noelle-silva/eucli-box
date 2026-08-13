//go:build windows

package releaseverify

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"eucli-box/pkg/workspace"
)

func TestFinalizerCleansBootstrapDirectoriesAndCompletesReport(t *testing.T) {
	repositoryRoot := repositoryRootForTest(t)
	runRoot := filepath.Join(
		workspace.VerificationStageRoot(repositoryRoot, "02"),
		fmt.Sprintf("run-finalizer-test-%d", time.Now().UnixNano()),
	)
	t.Cleanup(func() { _ = os.RemoveAll(runRoot) })

	evidence := filepath.Join(runRoot, "evidence")
	for _, path := range append([]string{evidence}, disposablePaths(runRoot)...) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create finalizer fixture: %v", err)
		}
	}
	report := Report{
		Tool:   "verify-release-publish",
		Mode:    "preflight",
		RunRoot: runRoot,
		Status:  "cleanup_pending",
		Checks:  []Check{{Name: "sample", Status: "passed", Summary: "passed"}},
		Cleanup: newCleanup("pending", nil, disposableDirectories()),
	}
	if err := writeReport(filepath.Join(evidence, "report.json"), report); err != nil {
		t.Fatalf("write finalizer fixture: %v", err)
	}
	if output, err := runFinalizerForTest(repositoryRoot, runRoot, "verify-release-publish", "preflight"); err != nil {
		t.Fatalf("run finalizer: %v\n%s", err, output)
	}

	for _, path := range disposablePaths(runRoot) {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("bootstrap directory remains: %s: %v", path, err)
		}
	}
	entries, err := os.ReadDir(runRoot)
	if err != nil {
		t.Fatalf("read finalized run: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "evidence" || !entries[0].IsDir() {
		t.Fatalf("unexpected finalized run entries: %#v", entries)
	}

	actual := readReportForTest(t, filepath.Join(evidence, "report.json"))
	if actual.Status != "passed" || actual.Cleanup.Status != "passed" || actual.FinishedAt == nil || actual.Cleanup.FinishedAt == nil {
		t.Fatalf("unexpected finalized report: %#v", actual)
	}
	if !sameStringSequence(actual.Cleanup.CompletedDirectories, disposableDirectories()) || len(actual.Cleanup.PendingDirectories) != 0 {
		t.Fatalf("unexpected cleanup state: %#v", actual.Cleanup)
	}
}

func TestFinalizerPassesChecksAndReportsManualCleanupWhenDeleteFails(t *testing.T) {
	repositoryRoot := repositoryRootForTest(t)
	runRoot := filepath.Join(
		workspace.VerificationStageRoot(repositoryRoot, "02"),
		fmt.Sprintf("run-finalizer-manual-%d", time.Now().UnixNano()),
	)
	t.Cleanup(func() { _ = os.RemoveAll(runRoot) })

	evidence := filepath.Join(runRoot, "evidence")
	for _, path := range append([]string{evidence}, disposablePaths(runRoot)...) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create finalizer fixture: %v", err)
		}
	}
	lockedPath := filepath.Join(runRoot, "workspace", "locked-artifact.dll")
	lockedPathPointer, err := syscall.UTF16PtrFromString(lockedPath)
	if err != nil {
		t.Fatalf("encode locked fixture path: %v", err)
	}
	lockedHandle, err := syscall.CreateFile(
		lockedPathPointer,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0,
		nil,
		syscall.CREATE_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatalf("create locked fixture: %v", err)
	}
	t.Cleanup(func() { _ = syscall.CloseHandle(lockedHandle) })
	report := Report{
		Tool:   "verify-release-publish",
		Mode:    "preflight",
		RunRoot: runRoot,
		Status:  "cleanup_pending",
		Checks:  []Check{{Name: "sample", Status: "passed", Summary: "passed"}},
		Cleanup: newCleanup("pending", nil, disposableDirectories()),
	}
	if err := writeReport(filepath.Join(evidence, "report.json"), report); err != nil {
		t.Fatalf("write finalizer fixture: %v", err)
	}

	if output, err := runFinalizerForTest(repositoryRoot, runRoot, "verify-release-publish", "preflight"); err != nil {
		t.Fatalf("run finalizer: %v\n%s", err, output)
	}
	actual := readReportForTest(t, filepath.Join(evidence, "report.json"))
	if actual.Status != "passed" || actual.Cleanup.Status != "manual_required" || actual.Cleanup.Message == "" || actual.Cleanup.Error != "" {
		t.Fatalf("unexpected manual cleanup report: %#v", actual)
	}
	if _, err := os.Stat(filepath.Join(runRoot, "workspace")); err != nil {
		t.Fatalf("manual cleanup workspace was removed: %v", err)
	}
}

func TestFinalizerCleansLongPathTree(t *testing.T) {
	repositoryRoot := repositoryRootForTest(t)
	runRoot := filepath.Join(
		workspace.VerificationStageRoot(repositoryRoot, "02"),
		fmt.Sprintf("run-finalizer-long-path-%d", time.Now().UnixNano()),
	)
	t.Cleanup(func() { _ = os.RemoveAll(runRoot) })

	evidence := filepath.Join(runRoot, "evidence")
	for _, path := range append([]string{evidence}, disposablePaths(runRoot)...) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create finalizer fixture: %v", err)
		}
	}
	longPath := filepath.Join(runRoot, "workspace")
	segment := strings.Repeat("long-path-segment-", 5)
	for index := 0; index < 4; index++ {
		longPath = filepath.Join(longPath, fmt.Sprintf("%d-%s", index, segment))
	}
	if len(longPath) <= syscall.MAX_PATH {
		t.Fatalf("fixture path is not long enough: %d", len(longPath))
	}
	if err := os.MkdirAll(longPath, 0o755); err != nil {
		t.Fatalf("create long path fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(longPath, "marker.txt"), []byte("temporary"), 0o644); err != nil {
		t.Fatalf("write long path fixture: %v", err)
	}
	report := Report{
		Tool:   "verify-release-publish",
		Mode:    "preflight",
		RunRoot: runRoot,
		Status:  "cleanup_pending",
		Checks:  []Check{{Name: "sample", Status: "passed", Summary: "passed"}},
		Cleanup: newCleanup("pending", nil, disposableDirectories()),
	}
	if err := writeReport(filepath.Join(evidence, "report.json"), report); err != nil {
		t.Fatalf("write finalizer fixture: %v", err)
	}

	if output, err := runFinalizerForTest(repositoryRoot, runRoot, "verify-release-publish", "preflight"); err != nil {
		t.Fatalf("run finalizer: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(runRoot, "workspace")); !os.IsNotExist(err) {
		t.Fatalf("long path workspace remains: %v", err)
	}
	actual := readReportForTest(t, filepath.Join(evidence, "report.json"))
	if actual.Status != "passed" || actual.Cleanup.Status != "passed" {
		t.Fatalf("unexpected long path cleanup report: %#v", actual)
	}
}

func TestFinalizerRejectsMismatchedToolWithoutCleaning(t *testing.T) {
	repositoryRoot := repositoryRootForTest(t)
	runRoot := filepath.Join(
		workspace.VerificationStageRoot(repositoryRoot, "02"),
		fmt.Sprintf("run-finalizer-mismatch-%d", time.Now().UnixNano()),
	)
	t.Cleanup(func() { _ = os.RemoveAll(runRoot) })

	evidence := filepath.Join(runRoot, "evidence")
	for _, path := range append([]string{evidence}, disposablePaths(runRoot)...) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create finalizer fixture: %v", err)
		}
	}
	report := Report{
		Tool:   "verify-release-publish",
		Mode:    "preflight",
		RunRoot: runRoot,
		Status:  "cleanup_pending",
		Checks:  []Check{},
		Cleanup: newCleanup("pending", nil, disposableDirectories()),
	}
	if err := writeReport(filepath.Join(evidence, "report.json"), report); err != nil {
		t.Fatalf("write finalizer fixture: %v", err)
	}

	if output, err := runFinalizerForTest(repositoryRoot, runRoot, "verify-release-build", "preflight"); err == nil {
		t.Fatalf("expected tool mismatch rejection\n%s", output)
	}
	for _, path := range disposablePaths(runRoot) {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("mismatched run was modified: %s: %v", path, err)
		}
	}
}

func TestFinalizerRejectsReparsePointWithoutCleaning(t *testing.T) {
	repositoryRoot := repositoryRootForTest(t)
	verificationRoot := workspace.VerificationRoot(repositoryRoot)
	identifier := time.Now().UnixNano()
	runRoot := filepath.Join(verificationRoot, "stage-02", fmt.Sprintf("run-finalizer-reparse-%d", identifier))
	externalRoot := filepath.Join(verificationRoot, fmt.Sprintf("finalizer-external-%d", identifier))
	junction := filepath.Join(runRoot, "workspace", "external-link")
	t.Cleanup(func() {
		_ = os.Remove(junction)
		_ = os.RemoveAll(runRoot)
		_ = os.RemoveAll(externalRoot)
	})

	evidence := filepath.Join(runRoot, "evidence")
	for _, path := range append([]string{evidence}, disposablePaths(runRoot)...) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create finalizer fixture: %v", err)
		}
	}
	if err := os.MkdirAll(externalRoot, 0o755); err != nil {
		t.Fatalf("create external fixture: %v", err)
	}
	marker := filepath.Join(externalRoot, "must-remain.txt")
	if err := os.WriteFile(marker, []byte("external"), 0o644); err != nil {
		t.Fatalf("write external fixture: %v", err)
	}
	if output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", junction, externalRoot).CombinedOutput(); err != nil {
		t.Fatalf("create junction fixture: %v\n%s", err, output)
	}
	report := Report{
		Tool:   "verify-release-publish",
		Mode:    "preflight",
		RunRoot: runRoot,
		Status:  "cleanup_pending",
		Checks:  []Check{{Name: "sample", Status: "passed", Summary: "passed"}},
		Cleanup: newCleanup("pending", nil, disposableDirectories()),
	}
	if err := writeReport(filepath.Join(evidence, "report.json"), report); err != nil {
		t.Fatalf("write finalizer fixture: %v", err)
	}

	if output, err := runFinalizerForTest(repositoryRoot, runRoot, "verify-release-publish", "preflight"); err == nil {
		t.Fatalf("expected reparse point rejection\n%s", output)
	}
	for _, path := range disposablePaths(runRoot) {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("cleanup started before reparse rejection: %s: %v", path, err)
		}
	}
	if payload, err := os.ReadFile(marker); err != nil || string(payload) != "external" {
		t.Fatalf("external fixture was changed: %q, %v", payload, err)
	}
	actual := readReportForTest(t, filepath.Join(evidence, "report.json"))
	if actual.Status != "failed" || actual.Cleanup.Status != "retained" || actual.Error == "" {
		t.Fatalf("unexpected safety failure report: %#v", actual)
	}
}

func runFinalizerForTest(repositoryRoot string, runRoot string, tool string, mode string) ([]byte, error) {
	script := filepath.Join(repositoryRoot, "scripts", "release", "finalize-verification.ps1")
	command := exec.Command(
		"powershell.exe",
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		script,
		"-RepositoryRoot",
		repositoryRoot,
		"-RunRoot",
		runRoot,
		"-Tool",
		tool,
		"-Mode",
		mode,
	)
	return command.CombinedOutput()
}

func disposablePaths(runRoot string) []string {
	paths := make([]string, 0, len(disposableDirectories()))
	for _, name := range disposableDirectories() {
		paths = append(paths, filepath.Join(runRoot, name))
	}
	return paths
}

func repositoryRootForTest(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	repositoryRoot, err := filepath.Abs(filepath.Join(workingDirectory, "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repositoryRoot, "go.mod")); err != nil {
		t.Fatalf("repository root is invalid: %v", err)
	}
	return repositoryRoot
}
