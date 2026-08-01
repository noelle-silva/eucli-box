package releaseverify

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReportReplacesExistingReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	first := Report{Stage: "01", Mode: "full", Status: "running", Checks: []Check{}, Cleanup: newCleanup("not_started", nil, nil)}
	second := Report{Stage: "02", Mode: "preflight", Status: "cleanup_pending", Checks: []Check{{Name: "check", Status: "passed"}}, Cleanup: newCleanup("pending", nil, disposableDirectories())}

	if err := writeReport(path, first); err != nil {
		t.Fatalf("write first report: %v", err)
	}
	if err := writeReport(path, second); err != nil {
		t.Fatalf("replace report: %v", err)
	}

	report := readReportForTest(t, path)
	if report.Stage != second.Stage || report.Mode != second.Mode || report.Status != second.Status {
		t.Fatalf("unexpected report after replacement: %#v", report)
	}
	if _, err := os.Stat(path + ".temporary"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary report remains: %v", err)
	}
}

func TestRecorderFinishHandsAllCleanupToCaller(t *testing.T) {
	paths := newRunPathsForTest(t)
	for _, path := range []string{paths.inputs, paths.workspace, paths.environment} {
		if err := os.WriteFile(filepath.Join(path, "sample.txt"), []byte("sample"), 0o644); err != nil {
			t.Fatalf("create sample: %v", err)
		}
	}

	recorder := newRecorder("01", "full", paths.root)
	recorder.pass("sample", "passed")
	if err := recorder.finish(paths); err != nil {
		t.Fatalf("finish verification: %v", err)
	}

	for _, path := range []string{paths.inputs, paths.workspace, paths.environment, paths.temp, paths.cache} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("cleanup directory was not handed to caller: %s: %v", path, err)
		}
	}

	report := readReportForTest(t, filepath.Join(paths.evidence, "report.json"))
	if report.Status != "cleanup_pending" || report.Cleanup.Status != "pending" {
		t.Fatalf("unexpected handoff status: %#v", report)
	}
	if len(report.Cleanup.CompletedDirectories) != 0 {
		t.Fatalf("unexpected completed directories: %#v", report.Cleanup.CompletedDirectories)
	}
	if !sameStringSequence(report.Cleanup.PendingDirectories, disposableDirectories()) {
		t.Fatalf("unexpected pending directories: %#v", report.Cleanup.PendingDirectories)
	}
}

func TestRecorderFinishRetainsFailedVerification(t *testing.T) {
	paths := newRunPathsForTest(t)
	recorder := newRecorder("02", "preflight", paths.root)
	recorder.fail("sample", errors.New("sample failure"))
	if err := recorder.finish(paths); err == nil {
		t.Fatal("expected verification failure")
	}

	report := readReportForTest(t, filepath.Join(paths.evidence, "report.json"))
	if report.Status != "failed" || report.Cleanup.Status != "retained" {
		t.Fatalf("unexpected failure status: %#v", report)
	}
	if !sameStringSequence(report.Cleanup.PendingDirectories, disposableDirectories()) {
		t.Fatalf("unexpected retained directories: %#v", report.Cleanup.PendingDirectories)
	}
	for _, path := range []string{paths.inputs, paths.workspace, paths.environment, paths.temp, paths.cache} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("failed run was not retained: %s: %v", path, err)
		}
	}
}

func newRunPathsForTest(t *testing.T) runPaths {
	t.Helper()
	repositoryRoot := t.TempDir()
	runRoot := filepath.Join(repositoryRoot, ".release", "verification", "stage-01", "run-test")
	paths, err := prepareRun(repositoryRoot, runRoot, "01")
	if err != nil {
		t.Fatalf("prepare run: %v", err)
	}
	return paths
}

func readReportForTest(t *testing.T, path string) Report {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report Report
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	return report
}
