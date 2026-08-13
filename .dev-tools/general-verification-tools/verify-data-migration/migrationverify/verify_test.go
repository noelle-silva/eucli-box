package migrationverify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"devtools/common/toolkit"
)

func TestStatusVocabularyCheck(t *testing.T) {
	for _, word := range statusVocabulary {
		if !containsString(statusVocabulary, word) {
			t.Fatalf("vocabulary missing %s", word)
		}
	}
	for _, word := range []string{"half-migrated", "", "data-changed"} {
		if containsString(statusVocabulary, word) {
			t.Fatalf("vocabulary wrongly contains %q", word)
		}
	}
	if len(statusVocabulary) != 4 {
		t.Fatalf("vocabulary must have exactly four words")
	}
}

func TestReadMigrationStatusParsesRecord(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "case-x", "data")
	workspaceDir := filepath.Join(filepath.Dir(dataDir), "data.migration")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	payload := `{
  "schemaVersion": 1,
  "outcome": "recovered",
  "fromVersion": "1.0.0",
  "targetVersion": "1.2.0",
  "currentDataVersion": "1.0.0",
  "stepIDs": ["1.0.0-to-1.1.0", "1.1.0-to-1.2.0"],
  "completed": true,
  "detail": "数据已恢复到迁移开始前",
  "updatedAt": "2026-08-13T10:01:00.000000000Z"
}
`
	if err := os.WriteFile(filepath.Join(workspaceDir, "status.json"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	status, err := readMigrationStatus(dataDir)
	if err != nil {
		t.Fatalf("readMigrationStatus() error = %v", err)
	}
	if status.Outcome != "recovered" || status.FromVersion != "1.0.0" || status.TargetVersion != "1.2.0" || status.CurrentDataVersion != "1.0.0" || !status.Completed {
		t.Fatalf("status = %#v", status)
	}
	if !containsString(statusVocabulary, status.Outcome) {
		t.Fatalf("outcome %q outside vocabulary", status.Outcome)
	}
}

func TestReadDataVersionParsesRecord(t *testing.T) {
	dataDir := t.TempDir()
	metaDir := filepath.Join(dataDir, "meta")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	payload := "{\n  \"version\": \"1.2.0\",\n  \"createdAt\": \"2026-08-13T10:00:00Z\",\n  \"updatedAt\": \"2026-08-13T10:00:00Z\"\n}\n"
	if err := os.WriteFile(filepath.Join(metaDir, "version.json"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	version, err := readDataVersion(dataDir)
	if err != nil {
		t.Fatalf("readDataVersion() error = %v", err)
	}
	if version != "1.2.0" {
		t.Fatalf("version = %q", version)
	}
}

func TestContainsReadyMark(t *testing.T) {
	if containsReadyMark("eucli-box v0.1.1 is ready — listening on http://127.0.0.1:1") {
		return
	}
	t.Fatalf("ready mark not detected")
}

func TestVerificationReportFitsFinalizeProtocol(t *testing.T) {
	runRoot := t.TempDir()
	evidenceDir := filepath.Join(runRoot, "evidence")
	disposable := make([]string, 0, 6)
	for _, name := range []string{"inputs", "workspace", "environment", "work", "temp", "cache"} {
		dir := filepath.Join(runRoot, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		disposable = append(disposable, dir)
	}
	recorder := toolkit.NewVerificationRecorder(toolName, defaultMode, runRoot)
	recorder.Pass("场景断言", "通过")
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
	if report["tool"] != toolName || report["mode"] != defaultMode || report["runRoot"] != runRoot {
		t.Fatalf("report identity = %#v", report)
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
	expected := []string{"inputs", "workspace", "environment", "work", "temp", "cache"}
	for index, name := range expected {
		if pending[index] != name {
			t.Fatalf("pendingDirectories[%d] = %#v, want %s", index, pending[index], name)
		}
	}
}
