package toolkit

import (
	"encoding/json"
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
