package datamigration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProcessRecordRoundTrip(t *testing.T) {
	dataDir := t.TempDir()
	w := testWorkspace(t, dataDir)
	record := newProcessRecord("1.0.0", "1.2.0", []string{"1.0.0-to-1.1.0", "1.1.0-to-1.2.0"}, processBackupInfo{
		RunID:    "20260813T100000.000000000Z",
		Manifest: "backup/run-20260813T100000.000000000Z/manifest.json",
		Verified: true,
	})
	if err := writeProcessRecord(w, record); err != nil {
		t.Fatalf("writeProcessRecord() error = %v", err)
	}
	loaded, exists, err := readProcessRecord(w)
	if err != nil {
		t.Fatalf("readProcessRecord() error = %v", err)
	}
	if !exists || loaded.FromVersion != "1.0.0" || loaded.TargetVersion != "1.2.0" || len(loaded.StepIDs) != 2 || loaded.Directive != directiveContinue {
		t.Fatalf("loaded record = %#v", loaded)
	}
	if loaded.Backup.RunID != "20260813T100000.000000000Z" || !loaded.Backup.Verified {
		t.Fatalf("loaded backup = %#v", loaded.Backup)
	}
	if err := deleteProcessRecord(w); err != nil {
		t.Fatalf("deleteProcessRecord() error = %v", err)
	}
	if _, exists, err := readProcessRecord(w); err != nil || exists {
		t.Fatalf("record still exists after delete: exists=%v err=%v", exists, err)
	}
}

func TestProcessRecordAppendVerifiedStep(t *testing.T) {
	record := newProcessRecord("1.0.0", "1.2.0", []string{"1.0.0-to-1.1.0", "1.1.0-to-1.2.0"}, processBackupInfo{RunID: "20260813T100000.000000000Z", Manifest: "backup/run-20260813T100000.000000000Z/manifest.json", Verified: true})
	step := Step{ID: "1.0.0-to-1.1.0", FromVersion: "1.0.0", ToVersion: "1.1.0", Scope: []string{"meta/counter.json"}}
	record.appendVerifiedStep(step, "2026-08-13T10:00:00.000000000Z")
	if record.CurrentIndex != 1 || len(record.StepResults) != 1 {
		t.Fatalf("record = %#v", record)
	}
	result := record.StepResults[0]
	if result.StepID != step.ID || result.Phase != phaseVerified || result.DataVersionWritten != "1.1.0" {
		t.Fatalf("step result = %#v", result)
	}
	if err := validateProcessRecord(record); err != nil {
		t.Fatalf("validateProcessRecord() error = %v", err)
	}
}

func TestProcessRecordJSONShape(t *testing.T) {
	record := newProcessRecord("1.0.0", "1.2.0", []string{"1.0.0-to-1.1.0", "1.1.0-to-1.2.0"}, processBackupInfo{RunID: "20260813T100000.000000000Z", Manifest: "backup/run-20260813T100000.000000000Z/manifest.json", Verified: true})
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"schemaVersion", "fromVersion", "targetVersion", "stepIDs", "currentIndex", "stepResults", "backup", "directive", "startedAt", "updatedAt"} {
		if _, exists := decoded[field]; !exists {
			t.Fatalf("process record missing field %s", field)
		}
	}
	if decoded["directive"] != directiveContinue {
		t.Fatalf("directive = %#v", decoded["directive"])
	}
	backup, ok := decoded["backup"].(map[string]any)
	if !ok || backup["runID"] != "20260813T100000.000000000Z" || backup["manifest"] != "backup/run-20260813T100000.000000000Z/manifest.json" || backup["verified"] != true {
		t.Fatalf("backup = %#v", decoded["backup"])
	}
}

func TestProcessRecordRejectsInvalidContent(t *testing.T) {
	dataDir := t.TempDir()
	w := testWorkspace(t, dataDir)
	if err := os.WriteFile(w.processFile(), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, err := readProcessRecord(w)
	assertMigrationErrorCode(t, err, "migration.status_unknown")

	record := newProcessRecord("1.0.0", "1.2.0", []string{"1.0.0-to-1.1.0"}, processBackupInfo{RunID: "20260813T100000.000000000Z", Manifest: "backup/run-20260813T100000.000000000Z/manifest.json", Verified: true})
	record.Directive = "resume"
	if err := writeProcessRecord(w, record); err == nil {
		t.Fatal("writeProcessRecord() accepted invalid directive")
	}
}

func TestStatusRecordRoundTrip(t *testing.T) {
	dataDir := t.TempDir()
	w := testWorkspace(t, dataDir)
	record, err := newStatusRecord(Outcome{State: StateMigrated, From: "1.0.0", To: "1.2.0", Detail: "每级迁移均完成核对"}, "1.2.0", []string{"1.0.0-to-1.1.0", "1.1.0-to-1.2.0"}, true)
	if err != nil {
		t.Fatalf("newStatusRecord() error = %v", err)
	}
	if err := writeStatusRecord(w, record); err != nil {
		t.Fatalf("writeStatusRecord() error = %v", err)
	}
	loaded, exists, err := readStatusRecord(w)
	if err != nil {
		t.Fatalf("readStatusRecord() error = %v", err)
	}
	if !exists || loaded.Outcome != string(StateMigrated) || loaded.FromVersion != "1.0.0" || loaded.TargetVersion != "1.2.0" || loaded.CurrentDataVersion != "1.2.0" || !loaded.Completed {
		t.Fatalf("loaded record = %#v", loaded)
	}
}

func TestStatusRecordJSONShape(t *testing.T) {
	record, err := newStatusRecord(Outcome{State: StateMigrated, From: "1.0.0", To: "1.2.0", Detail: "每级迁移均完成核对"}, "1.2.0", []string{"1.0.0-to-1.1.0", "1.1.0-to-1.2.0"}, true)
	if err != nil {
		t.Fatalf("newStatusRecord() error = %v", err)
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"schemaVersion", "outcome", "fromVersion", "targetVersion", "currentDataVersion", "stepIDs", "completed", "detail", "updatedAt"} {
		if _, exists := decoded[field]; !exists {
			t.Fatalf("status record missing field %s", field)
		}
	}
}

func TestStatusRecordRejectsUnknownOutcome(t *testing.T) {
	dataDir := t.TempDir()
	w := testWorkspace(t, dataDir)
	record := statusRecord{
		SchemaVersion:      1,
		Outcome:            "half-migrated",
		FromVersion:        "1.0.0",
		TargetVersion:      "1.2.0",
		CurrentDataVersion: "1.0.0",
		StepIDs:            []string{},
		Completed:          false,
		Detail:             "",
		UpdatedAt:          "2026-08-13T10:00:00.000000000Z",
	}
	if err := writeStatusRecord(w, record); err == nil {
		t.Fatal("writeStatusRecord() accepted unknown outcome")
	}
	if err := os.WriteFile(w.statusFile(), []byte(`{"schemaVersion":1,"outcome":"half-migrated","currentDataVersion":"1.0.0","updatedAt":"2026-08-13T10:00:00.000000000Z"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, err := readStatusRecord(w)
	assertMigrationErrorCode(t, err, "migration.status_unknown")
}

func TestWorkspaceSubpaths(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	w, err := newWorkspace(dataDir)
	if err != nil {
		t.Fatalf("newWorkspace() error = %v", err)
	}
	if w.dir != filepath.Join(filepath.Dir(dataDir), "data.migration") {
		t.Fatalf("workspace dir = %q", w.dir)
	}
	if w.statusFile() != filepath.Join(w.dir, "status.json") {
		t.Fatalf("status file = %q", w.statusFile())
	}
	if w.processFile() != filepath.Join(w.dir, "process.json") {
		t.Fatalf("process file = %q", w.processFile())
	}
	if w.backupRoot() != filepath.Join(w.dir, "backup") {
		t.Fatalf("backup root = %q", w.backupRoot())
	}
	if w.backupRunDir("20260813T100000.000000000Z") != filepath.Join(w.dir, "backup", "run-20260813T100000.000000000Z") {
		t.Fatalf("backup run dir = %q", w.backupRunDir("20260813T100000.000000000Z"))
	}
}
