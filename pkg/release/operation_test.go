package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func testOperationRecord(t *testing.T, workDirectory string) OperationRecord {
	t.Helper()
	record, err := NewOperationRecord("op-123", types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"}, OperationActionInstall, "0.1.0", workDirectory)
	if err != nil {
		t.Fatalf("NewOperationRecord() error = %v", err)
	}
	return record
}

func TestOperationRecordWriteReadRoundTrip(t *testing.T) {
	record := testOperationRecord(t, t.TempDir())
	path := filepath.Join(t.TempDir(), "operation.json")
	if err := WriteOperationRecord(path, record); err != nil {
		t.Fatalf("WriteOperationRecord() error = %v", err)
	}
	read, err := ReadOperationRecord(path)
	if err != nil {
		t.Fatalf("ReadOperationRecord() error = %v", err)
	}
	if read.OperationID != record.OperationID || read.Phase != types.ArtifactPhaseCandidate || read.Result != OperationResultRunning {
		t.Fatalf("read record = %#v", read)
	}
}

func TestOperationRecordRejectsDamagedPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operation.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ReadOperationRecord(path); err == nil {
		t.Fatal("ReadOperationRecord() error = nil")
	}
	if err := os.WriteFile(path, []byte(`{broken`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ReadOperationRecord(path); err == nil {
		t.Fatal("ReadOperationRecord() error = nil")
	}
}

func TestOperationRecordRejectsUnknownFields(t *testing.T) {
	record := testOperationRecord(t, t.TempDir())
	path := filepath.Join(t.TempDir(), "operation.json")
	if err := WriteOperationRecord(path, record); err != nil {
		t.Fatalf("WriteOperationRecord() error = %v", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	modified := strings.TrimSuffix(string(payload), "\n") + "\n  \"extra\": 1\n}\n"
	if err := os.WriteFile(path, []byte(modified), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ReadOperationRecord(path); err == nil {
		t.Fatal("ReadOperationRecord() error = nil")
	}
}

func TestOperationPhaseRecoveryClassification(t *testing.T) {
	for _, phase := range []string{
		types.ArtifactPhaseCandidate, types.ArtifactPhaseCompatibility, types.ArtifactPhaseActivity,
		types.ArtifactPhaseDownload, types.ArtifactPhaseManifest, types.ArtifactPhaseArchive,
		types.ArtifactPhasePackage, types.ArtifactPhasePrepare,
	} {
		if !OperationPhaseIsPreSwitch(phase) || OperationPhaseIsPostSwitch(phase) {
			t.Fatalf("phase %s classification wrong", phase)
		}
	}
	for _, phase := range []string{types.ArtifactPhaseSwitch, types.ArtifactPhaseProbe, types.ArtifactPhaseRestore, types.ArtifactPhaseRefresh} {
		if OperationPhaseIsPreSwitch(phase) || !OperationPhaseIsPostSwitch(phase) {
			t.Fatalf("phase %s classification wrong", phase)
		}
	}
}

func TestNewOperationRecordRejectsInvalidArguments(t *testing.T) {
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"}
	if _, err := NewOperationRecord("../bad", identity, OperationActionInstall, "0.1.0", t.TempDir()); err == nil {
		t.Fatal("NewOperationRecord() with bad operation id error = nil")
	}
	if _, err := NewOperationRecord("op-1", identity, "upgrade", "0.1.0", t.TempDir()); err == nil {
		t.Fatal("NewOperationRecord() with bad action error = nil")
	}
	if _, err := NewOperationRecord("op-1", identity, OperationActionInstall, "not-a-version", t.TempDir()); err == nil {
		t.Fatal("NewOperationRecord() with bad version error = nil")
	}
}

func TestReadOperationRecordMissingFile(t *testing.T) {
	if _, err := ReadOperationRecord(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("ReadOperationRecord() error = nil")
	}
}
