package toolkit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectorySnapshotAbsentAndStable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	snapshot, err := DirectorySnapshot(root)
	if err != nil {
		t.Fatalf("DirectorySnapshot() error = %v", err)
	}
	if snapshot != "absent" {
		t.Fatalf("snapshot = %q, want absent", snapshot)
	}
	existing := t.TempDir()
	if err := os.WriteFile(filepath.Join(existing, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	first, err := DirectorySnapshot(existing)
	if err != nil {
		t.Fatalf("DirectorySnapshot() error = %v", err)
	}
	second, err := DirectorySnapshot(existing)
	if err != nil {
		t.Fatalf("DirectorySnapshot() error = %v", err)
	}
	if first != second {
		t.Fatalf("snapshots differ for identical content")
	}
	if err := os.WriteFile(filepath.Join(existing, "a.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	third, err := DirectorySnapshot(existing)
	if err != nil {
		t.Fatalf("DirectorySnapshot() error = %v", err)
	}
	if first == third {
		t.Fatalf("snapshots identical after content change")
	}
}

func TestPrepareVerificationRunUsesStandardBoundary(t *testing.T) {
	repositoryRoot := t.TempDir()
	runRoot := filepath.Join(repositoryRoot, ".dev-workspace", ".dev-tools-runtime", "verify-demo", "run-20260828T000000Z")
	if err := os.MkdirAll(filepath.Join(runRoot, "temp"), 0o755); err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(runRoot, "cache"), 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	run, err := PrepareVerificationRun(repositoryRoot, runRoot, "verify-demo")
	if err != nil {
		t.Fatalf("PrepareVerificationRun() error = %v", err)
	}
	for _, directory := range append(run.DisposableDirectories(), run.Evidence) {
		if _, err := os.Stat(directory); err != nil {
			t.Fatalf("prepared directory %q: %v", directory, err)
		}
	}
}

func TestPrepareVerificationRunRejectsUnexpectedExistingEntry(t *testing.T) {
	repositoryRoot := t.TempDir()
	runRoot := filepath.Join(repositoryRoot, ".dev-workspace", ".dev-tools-runtime", "verify-demo", "run-20260828T000000Z")
	if err := os.MkdirAll(filepath.Join(runRoot, "unexpected"), 0o755); err != nil {
		t.Fatalf("mkdir unexpected: %v", err)
	}
	if _, err := PrepareVerificationRun(repositoryRoot, runRoot, "verify-demo"); err == nil {
		t.Fatal("PrepareVerificationRun() error = nil, want unexpected entry error")
	}
}

func TestRetirePreviousRunsKeepsOnlyCurrentRun(t *testing.T) {
	repositoryRoot := t.TempDir()
	toolRoot := filepath.Join(repositoryRoot, ".dev-workspace", ".dev-tools-runtime", "verify-demo")
	currentRun := filepath.Join(toolRoot, "run-current")
	otherToolRun := filepath.Join(repositoryRoot, ".dev-workspace", ".dev-tools-runtime", "verify-other", "run-kept")
	for _, directory := range []string{
		filepath.Join(toolRoot, "run-old-one", "evidence"),
		filepath.Join(toolRoot, "run-old-two"),
		filepath.Join(toolRoot, "cache", "shared"),
		currentRun,
		otherToolRun,
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", directory, err)
		}
	}
	if err := os.WriteFile(filepath.Join(toolRoot, "report.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolRoot, "run-notes.txt"), []byte("notes"), 0o644); err != nil {
		t.Fatalf("write run notes: %v", err)
	}

	if err := RetirePreviousRuns(repositoryRoot, currentRun, "verify-demo"); err != nil {
		t.Fatalf("RetirePreviousRuns() error = %v", err)
	}

	for _, retired := range []string{filepath.Join(toolRoot, "run-old-one"), filepath.Join(toolRoot, "run-old-two")} {
		if _, err := os.Stat(retired); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("old run %s survived rotation: %v", retired, err)
		}
	}
	for _, kept := range []string{
		currentRun,
		filepath.Join(toolRoot, "cache", "shared"),
		filepath.Join(toolRoot, "report.json"),
		filepath.Join(toolRoot, "run-notes.txt"),
		otherToolRun,
	} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("entry %s must survive rotation: %v", kept, err)
		}
	}
}

func TestRetirePreviousRunsRejectsUncleanCurrentRun(t *testing.T) {
	repositoryRoot := t.TempDir()
	toolRoot := filepath.Join(repositoryRoot, ".dev-workspace", ".dev-tools-runtime", "verify-demo")
	currentRun := filepath.Join(toolRoot, "run-current")
	oldRun := filepath.Join(toolRoot, "run-old")
	for _, directory := range []string{currentRun, oldRun} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", directory, err)
		}
	}
	if err := os.WriteFile(filepath.Join(currentRun, "unexpected.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write unexpected entry: %v", err)
	}

	if err := RetirePreviousRuns(repositoryRoot, currentRun, "verify-demo"); err == nil {
		t.Fatal("RetirePreviousRuns() error = nil, want unclean current run rejection")
	}
	if _, err := os.Stat(oldRun); err != nil {
		t.Fatalf("rotation must not start before validation: %v", err)
	}
}

func TestRetirePreviousRunsRejectsRunRootOutsideToolArea(t *testing.T) {
	repositoryRoot := t.TempDir()
	foreignRun := filepath.Join(repositoryRoot, ".dev-workspace", ".dev-tools-runtime", "verify-other", "run-kept")
	if err := os.MkdirAll(foreignRun, 0o755); err != nil {
		t.Fatalf("mkdir foreign run: %v", err)
	}

	if err := RetirePreviousRuns(repositoryRoot, foreignRun, "verify-demo"); err == nil {
		t.Fatal("RetirePreviousRuns() error = nil, want outside tool area rejection")
	}
	if _, err := os.Stat(foreignRun); err != nil {
		t.Fatalf("foreign tool run must survive rejected rotation: %v", err)
	}
}

func TestPrepareVerificationRunRetiresPreviousRuns(t *testing.T) {
	repositoryRoot := t.TempDir()
	toolRoot := filepath.Join(repositoryRoot, ".dev-workspace", ".dev-tools-runtime", "verify-demo")
	currentRun := filepath.Join(toolRoot, "run-current")
	oldRun := filepath.Join(toolRoot, "run-old")
	for _, directory := range []string{
		filepath.Join(currentRun, "temp"),
		filepath.Join(currentRun, "cache"),
		filepath.Join(oldRun, "evidence"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", directory, err)
		}
	}

	run, err := PrepareVerificationRun(repositoryRoot, currentRun, "verify-demo")
	if err != nil {
		t.Fatalf("PrepareVerificationRun() error = %v", err)
	}
	if _, err := os.Stat(oldRun); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous run survived preparation: %v", err)
	}
	for _, directory := range append(run.DisposableDirectories(), run.Evidence) {
		if _, err := os.Stat(directory); err != nil {
			t.Fatalf("prepared directory %q: %v", directory, err)
		}
	}
}

func TestVerificationRecorderPassProtocol(t *testing.T) {
	runRoot := t.TempDir()
	evidenceDir := filepath.Join(runRoot, "evidence")
	disposable := make([]string, 0, len(verificationDisposableNames()))
	for _, name := range verificationDisposableNames() {
		dir := filepath.Join(runRoot, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		disposable = append(disposable, dir)
	}
	recorder := NewVerificationRecorder("verify-demo", "default", runRoot)
	recorder.Pass("检查一", "通过")
	if err := recorder.Finish(evidenceDir, disposable); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(evidenceDir, "report.json"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if report["status"] != "cleanup_pending" {
		t.Fatalf("status = %#v", report["status"])
	}
	cleanup, ok := report["cleanup"].(map[string]any)
	if !ok || cleanup["status"] != "pending" {
		t.Fatalf("cleanup = %#v", report["cleanup"])
	}
	completed, ok := cleanup["completedDirectories"].([]any)
	if !ok || len(completed) != 0 {
		t.Fatalf("completedDirectories = %#v", cleanup["completedDirectories"])
	}
	pending, ok := cleanup["pendingDirectories"].([]any)
	if !ok || len(pending) != 6 {
		t.Fatalf("pendingDirectories = %#v", cleanup["pendingDirectories"])
	}
	for index, name := range verificationDisposableNames() {
		if pending[index] != name {
			t.Fatalf("pendingDirectories[%d] = %#v, want %s", index, pending[index], name)
		}
	}
}

func TestVerificationRecorderFinishTaskWritesCompletedReport(t *testing.T) {
	runRoot := t.TempDir()
	evidenceDir := filepath.Join(runRoot, "evidence")
	disposable := make([]string, 0, len(verificationDisposableNames()))
	for _, name := range verificationDisposableNames() {
		dir := filepath.Join(runRoot, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		disposable = append(disposable, dir)
	}
	recorder := NewVerificationRecorder("verify-task", "default", runRoot)
	recorder.Pass("检查一", "通过")
	if err := recorder.FinishTask(evidenceDir, disposable); err != nil {
		t.Fatalf("FinishTask() error = %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(evidenceDir, "report.json"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if report["status"] != "passed" {
		t.Fatalf("status = %#v", report["status"])
	}
	cleanup := report["cleanup"].(map[string]any)
	if cleanup["status"] != "manual_required" {
		t.Fatalf("cleanup status = %#v", cleanup["status"])
	}
}

func TestVerificationRecorderFailureRetainsScene(t *testing.T) {
	runRoot := t.TempDir()
	evidenceDir := filepath.Join(runRoot, "evidence")
	disposable := make([]string, 0, len(verificationDisposableNames()))
	for _, name := range verificationDisposableNames() {
		dir := filepath.Join(runRoot, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		disposable = append(disposable, dir)
	}
	recorder := NewVerificationRecorder("verify-demo", "default", runRoot)
	recorder.Fail("检查一", os.ErrPermission)
	if err := recorder.Finish(evidenceDir, disposable); err == nil {
		t.Fatal("Finish() error = nil, want failure")
	}
	payload, err := os.ReadFile(filepath.Join(evidenceDir, "report.json"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if report["status"] != "failed" {
		t.Fatalf("status = %#v", report["status"])
	}
	cleanup, ok := report["cleanup"].(map[string]any)
	if !ok || cleanup["status"] != "retained" {
		t.Fatalf("cleanup = %#v", report["cleanup"])
	}
	if report["error"] == "" {
		t.Fatalf("error is empty")
	}
}

func TestIsolatedEnvironmentReplacesTemporaryVariables(t *testing.T) {
	t.Setenv("TEMP", "C:\\outside\\temp")
	t.Setenv("TMP", "C:\\outside\\tmp")
	t.Setenv("GOTMPDIR", "C:\\outside\\gotmp")
	t.Setenv("GOTELEMETRY", "on")
	env := isolatedEnvironment(filepath.Join("E:", "run", "temp"), map[string]string{"CUSTOM": "custom-value"})
	values := map[string]string{}
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	if values["TEMP"] != filepath.Join("E:", "run", "temp") {
		t.Fatalf("TEMP = %q", values["TEMP"])
	}
	if values["TMP"] != filepath.Join("E:", "run", "temp") {
		t.Fatalf("TMP = %q", values["TMP"])
	}
	if values["GOTMPDIR"] != filepath.Join("E:", "run", "temp", "go") {
		t.Fatalf("GOTMPDIR = %q", values["GOTMPDIR"])
	}
	if values["GOTELEMETRY"] != "off" {
		t.Fatalf("GOTELEMETRY = %q", values["GOTELEMETRY"])
	}
	if values["CUSTOM"] != "custom-value" {
		t.Fatalf("CUSTOM = %q", values["CUSTOM"])
	}
}
