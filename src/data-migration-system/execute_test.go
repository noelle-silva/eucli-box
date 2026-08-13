package datamigration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	datastorage "eucli-box/src/data-storage-system"
)

const testTargetVersion = "1.2.0"

func readTestCounter(dataDir string) (int, error) {
	payload, err := os.ReadFile(filepath.Join(dataDir, "meta", "counter.json"))
	if err != nil {
		return 0, err
	}
	var value struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return 0, err
	}
	return value.Count, nil
}

func writeTestCounter(dataDir string, count int) error {
	payload, err := json.MarshalIndent(map[string]any{"count": count}, "", "  ")
	if err != nil {
		return err
	}
	return writeTestFileBytes(filepath.Join(dataDir, "meta", "counter.json"), payload)
}

func readTestStamp(dataDir string) (string, error) {
	payload, err := os.ReadFile(filepath.Join(dataDir, "meta", "stamp.json"))
	if err != nil {
		return "", err
	}
	var value struct {
		Stamp string `json:"stamp"`
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return "", err
	}
	return value.Stamp, nil
}

func writeTestStamp(dataDir string, stamp string) error {
	payload, err := json.MarshalIndent(map[string]any{"stamp": stamp}, "", "  ")
	if err != nil {
		return err
	}
	return writeTestFileBytes(filepath.Join(dataDir, "meta", "stamp.json"), payload)
}

func writeTestFileBytes(path string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func seedTestData(t *testing.T, dataDir string, version string) {
	t.Helper()
	if version != "" {
		if err := datastorage.WriteStorageVersion(context.Background(), dataDir, datastorage.StorageVersion{Version: version, CreatedAt: mustNow(t), UpdatedAt: mustNow(t)}); err != nil {
			t.Fatalf("WriteStorageVersion() error = %v", err)
		}
	}
	if err := writeTestCounter(dataDir, 0); err != nil {
		t.Fatalf("writeTestCounter() error = %v", err)
	}
}

func mustNow(t *testing.T) (now time.Time) {
	t.Helper()
	return time.Now().UTC()
}

func registerTestChain(failApply int, failVerify int) {
	resetRegistry()
	Register(Step{
		ID:          "1.0.0-to-1.1.0",
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Scope:       []string{"meta/counter.json"},
		Precheck: func(ctx context.Context, dataDir string) error {
			count, err := readTestCounter(dataDir)
			if err != nil {
				return err
			}
			if count != 0 {
				return fmt.Errorf("counter must be 0 before first step")
			}
			return nil
		},
		Apply: func(ctx context.Context, dataDir string) error {
			if failApply == 1 {
				return fmt.Errorf("injected apply failure at step 1")
			}
			return writeTestCounter(dataDir, 1)
		},
		Verify: func(ctx context.Context, dataDir string) error {
			if failVerify == 1 {
				return fmt.Errorf("injected verify failure at step 1")
			}
			count, err := readTestCounter(dataDir)
			if err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("counter must be 1 after first step")
			}
			return nil
		},
	})
	Register(Step{
		ID:          "1.1.0-to-1.2.0",
		FromVersion: "1.1.0",
		ToVersion:   "1.2.0",
		Scope:       []string{"meta/counter.json", "meta/stamp.json"},
		Precheck: func(ctx context.Context, dataDir string) error {
			count, err := readTestCounter(dataDir)
			if err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("counter must be 1 before second step")
			}
			return nil
		},
		Apply: func(ctx context.Context, dataDir string) error {
			if failApply == 2 {
				return fmt.Errorf("injected apply failure at step 2")
			}
			if err := writeTestCounter(dataDir, 2); err != nil {
				return err
			}
			return writeTestStamp(dataDir, "1.2.0")
		},
		Verify: func(ctx context.Context, dataDir string) error {
			if failVerify == 2 {
				return fmt.Errorf("injected verify failure at step 2")
			}
			count, err := readTestCounter(dataDir)
			if err != nil {
				return err
			}
			if count != 2 {
				return fmt.Errorf("counter must be 2 after second step")
			}
			stamp, err := readTestStamp(dataDir)
			if err != nil {
				return err
			}
			if stamp != "1.2.0" {
				return fmt.Errorf("stamp must be 1.2.0 after second step")
			}
			return nil
		},
	})
}

func readDataVersion(t *testing.T, dataDir string) (string, bool) {
	t.Helper()
	version, exists, err := datastorage.ReadStorageVersion(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("ReadStorageVersion() error = %v", err)
	}
	return version.Version, exists
}

func readSessionStatus(t *testing.T, dataDir string) (statusRecord, bool) {
	t.Helper()
	w, err := newWorkspace(dataDir)
	if err != nil {
		t.Fatalf("newWorkspace() error = %v", err)
	}
	record, exists, err := readStatusRecord(w)
	if err != nil {
		t.Fatalf("readStatusRecord() error = %v", err)
	}
	return record, exists
}

func TestPrepareFirstInstallWritesTargetVersion(t *testing.T) {
	dataDir := t.TempDir()
	session, err := Prepare(context.Background(), dataDir, testTargetVersion)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if session.Outcome().State != StateDataUnchanged {
		t.Fatalf("outcome = %#v", session.Outcome())
	}
	version, exists := readDataVersion(t, dataDir)
	if !exists || version != testTargetVersion {
		t.Fatalf("version = %q exists=%v", version, exists)
	}
	status, exists := readSessionStatus(t, dataDir)
	if !exists || status.Outcome != string(StateDataUnchanged) || status.Completed {
		t.Fatalf("status = %#v exists=%v", status, exists)
	}
	if err := session.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := session.Complete(context.Background()); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	status, _ = readSessionStatus(t, dataDir)
	if !status.Completed || status.Outcome != string(StateDataUnchanged) {
		t.Fatalf("status after complete = %#v", status)
	}
}

func TestPrepareVersionAlreadyAtTargetLeavesDataUntouched(t *testing.T) {
	dataDir := t.TempDir()
	seedTestData(t, dataDir, testTargetVersion)
	writeTestFile(t, filepath.Join(dataDir, "sessions", "keep.json"), `{"keep":true}`)
	before := snapshotDir(t, dataDir)

	session, err := Prepare(context.Background(), dataDir, testTargetVersion)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if session.Outcome().State != StateDataUnchanged {
		t.Fatalf("outcome = %#v", session.Outcome())
	}
	if err := session.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := session.Complete(context.Background()); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if after := snapshotDir(t, dataDir); before != after {
		t.Fatalf("data changed although no migration was needed")
	}
}

func TestPrepareRejectsVersionTooHigh(t *testing.T) {
	dataDir := t.TempDir()
	seedTestData(t, dataDir, "2.0.0")
	_, err := Prepare(context.Background(), dataDir, testTargetVersion)
	assertMigrationErrorCode(t, err, "migration.version_too_high")
	w, wErr := newWorkspace(dataDir)
	if wErr != nil {
		t.Fatalf("newWorkspace() error = %v", wErr)
	}
	if _, err := os.Stat(w.statusFile()); !os.IsNotExist(err) {
		t.Fatalf("status record was written despite version too high")
	}
}

func TestContinuousMigrationAcrossChain(t *testing.T) {
	registerTestChain(0, 0)
	dataDir := t.TempDir()
	seedTestData(t, dataDir, "1.0.0")
	session, err := Prepare(context.Background(), dataDir, testTargetVersion)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := session.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if session.Outcome().State != StateMigrated {
		t.Fatalf("outcome = %#v", session.Outcome())
	}
	version, _ := readDataVersion(t, dataDir)
	if version != testTargetVersion {
		t.Fatalf("version = %q", version)
	}
	count, err := readTestCounter(dataDir)
	if err != nil || count != 2 {
		t.Fatalf("counter = %d err=%v", count, err)
	}
	stamp, err := readTestStamp(dataDir)
	if err != nil || stamp != "1.2.0" {
		t.Fatalf("stamp = %q err=%v", stamp, err)
	}
	w, wErr := newWorkspace(dataDir)
	if wErr != nil {
		t.Fatalf("newWorkspace() error = %v", wErr)
	}
	if _, exists, err := readProcessRecord(w); err != nil || !exists {
		t.Fatalf("process record missing before Complete: exists=%v err=%v", exists, err)
	}
	if err := session.Complete(context.Background()); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	status, exists := readSessionStatus(t, dataDir)
	if !exists || status.Outcome != string(StateMigrated) || !status.Completed || status.FromVersion != "1.0.0" || status.TargetVersion != testTargetVersion || status.CurrentDataVersion != testTargetVersion {
		t.Fatalf("status = %#v", status)
	}
	if _, exists, err := readProcessRecord(w); err != nil || exists {
		t.Fatalf("process record not cleaned after Complete: exists=%v err=%v", exists, err)
	}
	if _, err := os.Stat(w.backupRoot()); !os.IsNotExist(err) {
		t.Fatalf("backup root not cleaned after Complete")
	}
}

func TestMissingChainStepIsRejected(t *testing.T) {
	registerTestChain(0, 0)
	dataDir := t.TempDir()
	seedTestData(t, dataDir, "1.0.0")
	before := snapshotDir(t, dataDir)
	_, err := Prepare(context.Background(), dataDir, "1.3.0")
	assertMigrationErrorCode(t, err, "migration.step_missing")
	if after := snapshotDir(t, dataDir); before != after {
		t.Fatalf("data changed although chain was missing a step")
	}
}

func TestStepFailureAutoRecoversToOriginal(t *testing.T) {
	registerTestChain(2, 0)
	dataDir := t.TempDir()
	seedTestData(t, dataDir, "1.0.0")
	before := snapshotDir(t, dataDir)

	session, err := Prepare(context.Background(), dataDir, testTargetVersion)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := session.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v, want nil after auto recovery", err)
	}
	if session.Outcome().State != StateRecovered {
		t.Fatalf("outcome = %#v", session.Outcome())
	}
	if after := snapshotDir(t, dataDir); before != after {
		t.Fatalf("data not identical to pre-migration state after recovery")
	}
	version, _ := readDataVersion(t, dataDir)
	if version != "1.0.0" {
		t.Fatalf("version = %q, want 1.0.0", version)
	}
	status, exists := readSessionStatus(t, dataDir)
	if !exists || status.Outcome != string(StateRecovered) || status.Completed {
		t.Fatalf("status = %#v", status)
	}
	if err := session.Complete(context.Background()); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	status, _ = readSessionStatus(t, dataDir)
	if !status.Completed {
		t.Fatalf("status not completed after Complete: %#v", status)
	}
}

func TestVerifyFailureAutoRecoversToOriginal(t *testing.T) {
	registerTestChain(0, 2)
	dataDir := t.TempDir()
	seedTestData(t, dataDir, "1.0.0")
	before := snapshotDir(t, dataDir)

	session, err := Prepare(context.Background(), dataDir, testTargetVersion)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := session.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v, want nil after auto recovery", err)
	}
	if session.Outcome().State != StateRecovered {
		t.Fatalf("outcome = %#v", session.Outcome())
	}
	if after := snapshotDir(t, dataDir); before != after {
		t.Fatalf("data not identical to pre-migration state after recovery")
	}
	version, _ := readDataVersion(t, dataDir)
	if version != "1.0.0" {
		t.Fatalf("version = %q, want 1.0.0", version)
	}
}

func TestRecoveryFailureKeepsScene(t *testing.T) {
	registerTestChain(2, 0)
	dataDir := t.TempDir()
	seedTestData(t, dataDir, "1.0.0")

	session, err := Prepare(context.Background(), dataDir, testTargetVersion)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	w, wErr := newWorkspace(dataDir)
	if wErr != nil {
		t.Fatalf("newWorkspace() error = %v", wErr)
	}
	manifestPath := w.manifestFile(session.process.Backup.RunID)
	if err := os.WriteFile(manifestPath, []byte("{corrupt"), 0o644); err != nil {
		t.Fatalf("corrupt manifest: %v", err)
	}
	err = session.Run(context.Background())
	assertMigrationErrorCode(t, err, "migration.recovery_failed")
	if session.Outcome().State != StateRecoveryFailed {
		t.Fatalf("outcome = %#v", session.Outcome())
	}
	status, exists := readSessionStatus(t, dataDir)
	if !exists || status.Outcome != string(StateRecoveryFailed) {
		t.Fatalf("status = %#v exists=%v", status, exists)
	}
	if _, exists, readErr := readProcessRecord(w); readErr != nil || !exists {
		t.Fatalf("process record not retained: exists=%v err=%v", exists, readErr)
	}
	if _, statErr := os.Stat(w.backupRunDir(session.process.Backup.RunID)); statErr != nil {
		t.Fatalf("backup scene not retained: %v", statErr)
	}
	version, _ := readDataVersion(t, dataDir)
	if version != "1.1.0" {
		t.Fatalf("version = %q, scene should stay at failed first-step state", version)
	}
}

func TestExplicitRecoverAfterSuccessfulRun(t *testing.T) {
	registerTestChain(0, 0)
	dataDir := t.TempDir()
	seedTestData(t, dataDir, "1.0.0")
	before := snapshotDir(t, dataDir)

	session, err := Prepare(context.Background(), dataDir, testTargetVersion)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := session.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := session.Recover(context.Background()); err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if session.Outcome().State != StateRecovered {
		t.Fatalf("outcome = %#v", session.Outcome())
	}
	if after := snapshotDir(t, dataDir); before != after {
		t.Fatalf("data not restored after explicit Recover")
	}
	version, _ := readDataVersion(t, dataDir)
	if version != "1.0.0" {
		t.Fatalf("version = %q, want 1.0.0", version)
	}
}

func TestRecoverOnDataUnchangedSessionIsNoop(t *testing.T) {
	dataDir := t.TempDir()
	seedTestData(t, dataDir, testTargetVersion)
	session, err := Prepare(context.Background(), dataDir, testTargetVersion)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := session.Recover(context.Background()); err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if session.Outcome().State != StateDataUnchanged {
		t.Fatalf("outcome = %#v", session.Outcome())
	}
}

func TestUnfinishedProcessRecordTriggersRecovery(t *testing.T) {
	registerTestChain(0, 0)
	dataDir := t.TempDir()
	seedTestData(t, dataDir, "1.0.0")
	before := snapshotDir(t, dataDir)

	w, err := newWorkspace(dataDir)
	if err != nil {
		t.Fatalf("newWorkspace() error = %v", err)
	}
	if err := w.ensure(); err != nil {
		t.Fatalf("workspace ensure: %v", err)
	}
	runID := "20260813T100000.000000000Z"
	scope := backupScope()
	if _, err := establishBackup(context.Background(), dataDir, w, runID, scope); err != nil {
		t.Fatalf("establishBackup() error = %v", err)
	}
	if err := writeTestCounter(dataDir, 1); err != nil {
		t.Fatalf("half-migrate counter: %v", err)
	}
	if err := writeTestStamp(dataDir, "1.1.0"); err != nil {
		t.Fatalf("half-migrate stamp: %v", err)
	}
	record := newProcessRecord("1.0.0", testTargetVersion, []string{"1.0.0-to-1.1.0", "1.1.0-to-1.2.0"}, processBackupInfo{
		RunID:    runID,
		Manifest: "backup/run-" + runID + "/manifest.json",
		Verified: true,
	})
	if err := writeProcessRecord(w, record); err != nil {
		t.Fatalf("writeProcessRecord() error = %v", err)
	}

	session, err := Prepare(context.Background(), dataDir, testTargetVersion)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if session.Outcome().State != StateRecovered {
		t.Fatalf("outcome = %#v", session.Outcome())
	}
	if after := snapshotDir(t, dataDir); before != after {
		t.Fatalf("data not restored to pre-migration state")
	}
	version, _ := readDataVersion(t, dataDir)
	if version != "1.0.0" {
		t.Fatalf("version = %q, want 1.0.0", version)
	}
	if _, exists, readErr := readProcessRecord(w); readErr != nil || exists {
		t.Fatalf("process record not cleaned: exists=%v err=%v", exists, readErr)
	}
	if err := session.Complete(context.Background()); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	status, _ := readSessionStatus(t, dataDir)
	if status.Outcome != string(StateRecovered) || !status.Completed {
		t.Fatalf("status = %#v", status)
	}
}

const crashHelperEnv = "EUCLI_MIGRATION_CRASH_HELPER"
const crashHelperDataDirEnv = "EUCLI_MIGRATION_CRASH_DATA_DIR"

func TestMigrationCrashIsRecoveredOnNextStart(t *testing.T) {
	if os.Getenv(crashHelperEnv) == "1" {
		runCrashHelper()
		return
	}
	registerTestChain(0, 0)
	dataDir := t.TempDir()
	seedTestData(t, dataDir, "1.0.0")
	before := snapshotDir(t, dataDir)

	command := exec.Command(os.Args[0], "-test.run", "TestMigrationCrashIsRecoveredOnNextStart", "-test.v")
	command.Env = append(os.Environ(), crashHelperEnv+"=1", crashHelperDataDirEnv+"="+dataDir)
	_, err := command.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 17 {
		t.Fatalf("helper exited with %v, want exit code 17", err)
	}
	w, wErr := newWorkspace(dataDir)
	if wErr != nil {
		t.Fatalf("newWorkspace() error = %v", wErr)
	}
	if _, exists, readErr := readProcessRecord(w); readErr != nil || !exists {
		t.Fatalf("process record missing after crash: exists=%v err=%v", exists, readErr)
	}

	session, err := Prepare(context.Background(), dataDir, testTargetVersion)
	if err != nil {
		t.Fatalf("Prepare() after crash error = %v", err)
	}
	if session.Outcome().State != StateRecovered {
		t.Fatalf("outcome = %#v", session.Outcome())
	}
	if after := snapshotDir(t, dataDir); before != after {
		t.Fatalf("data not restored after crash recovery")
	}
	version, _ := readDataVersion(t, dataDir)
	if version != "1.0.0" {
		t.Fatalf("version = %q, want 1.0.0", version)
	}
}

func runCrashHelper() {
	resetRegistry()
	Register(Step{
		ID:          "1.0.0-to-1.1.0",
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Scope:       []string{"meta/counter.json"},
		Precheck:    func(ctx context.Context, dataDir string) error { return nil },
		Apply: func(ctx context.Context, dataDir string) error {
			if err := writeTestCounter(dataDir, 1); err != nil {
				return err
			}
			os.Exit(17)
			return nil
		},
		Verify: func(ctx context.Context, dataDir string) error { return nil },
	})
	Register(Step{
		ID:          "1.1.0-to-1.2.0",
		FromVersion: "1.1.0",
		ToVersion:   "1.2.0",
		Scope:       []string{"meta/counter.json", "meta/stamp.json"},
		Precheck:    func(ctx context.Context, dataDir string) error { return nil },
		Apply: func(ctx context.Context, dataDir string) error {
			return writeTestStamp(dataDir, "1.2.0")
		},
		Verify: func(ctx context.Context, dataDir string) error { return nil },
	})
	dataDir := os.Getenv(crashHelperDataDirEnv)
	session, err := Prepare(context.Background(), dataDir, testTargetVersion)
	if err != nil {
		os.Exit(91)
	}
	if err := session.Run(context.Background()); err != nil {
		os.Exit(92)
	}
	os.Exit(93)
}
