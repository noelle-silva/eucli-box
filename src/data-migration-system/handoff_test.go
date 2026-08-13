package datamigration

import (
	"context"
	"os"
	"testing"
	"time"

	datastorage "eucli-box/src/data-storage-system"
)

func writeTestStatus(t *testing.T, w workspace, state State, completed bool) {
	t.Helper()
	record, err := newStatusRecord(Outcome{State: state, From: "1.0.0", To: "1.2.0", Detail: "测试状态"}, "1.2.0", []string{"1.0.0-to-1.1.0", "1.1.0-to-1.2.0"}, completed)
	if err != nil {
		t.Fatalf("newStatusRecord() error = %v", err)
	}
	if err := writeStatusRecord(w, record); err != nil {
		t.Fatalf("writeStatusRecord() error = %v", err)
	}
}

func writeTestProcessRecord(t *testing.T, w workspace) {
	t.Helper()
	record := newProcessRecord("1.0.0", "1.2.0", []string{"1.0.0-to-1.1.0", "1.1.0-to-1.2.0"}, processBackupInfo{
		RunID:    "20260813T100000.000000000Z",
		Manifest: "backup/run-20260813T100000.000000000Z/manifest.json",
		Verified: true,
	})
	if err := writeProcessRecord(w, record); err != nil {
		t.Fatalf("writeProcessRecord() error = %v", err)
	}
}

func writeTestVersion(t *testing.T, dataDir string, version string) {
	t.Helper()
	if err := datastorage.WriteStorageVersion(context.Background(), dataDir, datastorage.StorageVersion{Version: version, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("WriteStorageVersion() error = %v", err)
	}
}

func TestReadHandoffWithoutAnyFacts(t *testing.T) {
	dataDir := t.TempDir()
	handoff, err := ReadHandoff(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("ReadHandoff() error = %v", err)
	}
	if handoff.StatusPresent || handoff.Completed || handoff.ProcessPending || handoff.CurrentDataVersion != "" {
		t.Fatalf("handoff = %#v", handoff)
	}
	if handoff.Outcome.State != "" || handoff.Outcome.From != "" || handoff.Outcome.To != "" || handoff.Outcome.Detail != "" {
		t.Fatalf("handoff outcome = %#v", handoff.Outcome)
	}
}

func TestReadHandoffEachOutcome(t *testing.T) {
	states := []struct {
		state   State
		content string
	}{
		{StateDataUnchanged, "data-unchanged"},
		{StateMigrated, "migrated"},
		{StateRecovered, "recovered"},
		{StateRecoveryFailed, "recovery-failed"},
	}
	for _, item := range states {
		t.Run(string(item.state), func(t *testing.T) {
			dataDir := t.TempDir()
			w := testWorkspace(t, dataDir)
			writeTestStatus(t, w, item.state, true)
			writeTestVersion(t, dataDir, "1.2.0")
			handoff, err := ReadHandoff(context.Background(), dataDir)
			if err != nil {
				t.Fatalf("ReadHandoff() error = %v", err)
			}
			if !handoff.StatusPresent || handoff.Outcome.State != item.state || handoff.Outcome.From != "1.0.0" || handoff.Outcome.To != "1.2.0" {
				t.Fatalf("handoff = %#v", handoff)
			}
			if handoff.CurrentDataVersion != "1.2.0" || !handoff.Completed || handoff.ProcessPending {
				t.Fatalf("handoff = %#v", handoff)
			}
		})
	}
}

func TestReadHandoffCompletedFlag(t *testing.T) {
	dataDir := t.TempDir()
	w := testWorkspace(t, dataDir)
	writeTestStatus(t, w, StateMigrated, false)
	writeTestVersion(t, dataDir, "1.2.0")
	handoff, err := ReadHandoff(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("ReadHandoff() error = %v", err)
	}
	if handoff.Completed {
		t.Fatalf("completed must be false, handoff = %#v", handoff)
	}
}

func TestReadHandoffProcessPending(t *testing.T) {
	dataDir := t.TempDir()
	w := testWorkspace(t, dataDir)
	writeTestProcessRecord(t, w)
	handoff, err := ReadHandoff(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("ReadHandoff() error = %v", err)
	}
	if !handoff.ProcessPending {
		t.Fatalf("process pending must be true, handoff = %#v", handoff)
	}
}

func TestReadHandoffMissingVersionFile(t *testing.T) {
	dataDir := t.TempDir()
	w := testWorkspace(t, dataDir)
	writeTestStatus(t, w, StateDataUnchanged, true)
	handoff, err := ReadHandoff(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("ReadHandoff() error = %v", err)
	}
	if handoff.CurrentDataVersion != "" {
		t.Fatalf("current data version = %q, want empty", handoff.CurrentDataVersion)
	}
}

func TestReadHandoffCorruptedStatus(t *testing.T) {
	dataDir := t.TempDir()
	w := testWorkspace(t, dataDir)
	if err := os.WriteFile(w.statusFile(), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ReadHandoff(context.Background(), dataDir)
	assertMigrationErrorCode(t, err, "migration.status_unknown")
}

func TestReadHandoffUnknownOutcomeWord(t *testing.T) {
	dataDir := t.TempDir()
	w := testWorkspace(t, dataDir)
	payload := `{"schemaVersion":1,"outcome":"half-migrated","currentDataVersion":"1.0.0","updatedAt":"2026-08-13T10:00:00.000000000Z"}`
	if err := os.WriteFile(w.statusFile(), []byte(payload), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ReadHandoff(context.Background(), dataDir)
	assertMigrationErrorCode(t, err, "migration.status_unknown")
}
