package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	datamigration "eucli-box/src/data-migration-system"
	datastorage "eucli-box/src/data-storage-system"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

func TestResolveUpdateFailureDecisionTable(t *testing.T) {
	validHandoff := func() datamigration.Handoff {
		return datamigration.Handoff{
			StatusPresent:      true,
			Outcome:            datamigration.Outcome{State: datamigration.StateDataUnchanged},
			Completed:          true,
			ProcessPending:     false,
			CurrentDataVersion: "1.0.0",
		}
	}
	tests := []struct {
		name         string
		handoff      datamigration.Handoff
		handoffErr   error
		previousData string
		want         updateFailureDecision
	}{
		{name: "handoff error is unsafe", handoff: validHandoff(), handoffErr: errors.New("read failed"), previousData: "1.0.0", want: decisionUnsafe},
		{name: "process pending is recovery-start", handoff: func() datamigration.Handoff { h := validHandoff(); h.ProcessPending = true; return h }(), previousData: "1.0.0", want: decisionRecoveryStart},
		{name: "recovery-failed and not completed is unsafe", handoff: func() datamigration.Handoff {
			h := validHandoff()
			h.Outcome.State = datamigration.StateRecoveryFailed
			h.Completed = false
			return h
		}(), previousData: "1.0.0", want: decisionUnsafe},
		{name: "recovery-failed and completed is restore-previous", handoff: func() datamigration.Handoff {
			h := validHandoff()
			h.Outcome.State = datamigration.StateRecoveryFailed
			h.Completed = true
			return h
		}(), previousData: "1.0.0", want: decisionRestorePrevious},
		{name: "missing current data version is unsafe", handoff: func() datamigration.Handoff { h := validHandoff(); h.CurrentDataVersion = ""; return h }(), previousData: "1.0.0", want: decisionUnsafe},
		{name: "missing previous data version is unsafe", handoff: validHandoff(), previousData: "", want: decisionUnsafe},
		{name: "current lower than previous is restore-previous", handoff: func() datamigration.Handoff { h := validHandoff(); h.CurrentDataVersion = "0.9.0"; return h }(), previousData: "1.0.0", want: decisionRestorePrevious},
		{name: "current equal to previous is restore-previous", handoff: validHandoff(), previousData: "1.0.0", want: decisionRestorePrevious},
		{name: "current higher than previous is keep-new-version", handoff: func() datamigration.Handoff { h := validHandoff(); h.CurrentDataVersion = "1.2.0"; return h }(), previousData: "1.0.0", want: decisionKeepNewVersion},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveUpdateFailure(test.handoff, test.handoffErr, "0.1.0", test.previousData)
			if got != test.want {
				t.Fatalf("decision = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPreviousDataVersionFromProgram(t *testing.T) {
	root := seedProgramStore(t, "1.0.0", "1.2.0")
	if got := previousDataVersionFromProgram(root, "1.0.0"); got != "1.2.0" {
		t.Fatalf("data version = %q, want 1.2.0", got)
	}
	if got := previousDataVersionFromProgram(root, "missing"); got != "" {
		t.Fatalf("missing version data = %q, want empty", got)
	}
	if got := previousDataVersionFromProgram(root, ""); got != "" {
		t.Fatalf("empty version data = %q, want empty", got)
	}
}

// seedProgramStore 建立包含指定版本与数据目标的程序版本仓库，返回 programStoreDir。
func seedProgramStore(t *testing.T, version string, dataVersion string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "program", "eucli-box")
	seedProgramStoreVersion(t, root, version, dataVersion)
	return root
}

func seedProgramStoreVersion(t *testing.T, root string, version string, dataVersion string) {
	t.Helper()
	versionDir := filepath.Join(root, "versions", version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatalf("mkdir version dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "eucli-box.exe"), []byte("fake"), 0o644); err != nil {
		t.Fatalf("write program file: %v", err)
	}
	product := types.ReleaseProductRecord{
		SchemaVersion:  1,
		Artifact:       types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox},
		Version:        version,
		Platform:       types.ReleasePlatformWindowsX64,
		OfficialSource: "https://github.com/noelle-silva/eucli-box",
		Source: types.ReleaseSourceRecord{
			Repository: "https://github.com/noelle-silva/eucli-box",
			Commit:     "0123456789abcdef0123456789abcdef01234567",
			Recorded:   true,
		},
		DataVersion: dataVersion,
	}
	payload, err := json.MarshalIndent(product, "", "  ")
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "release-product.json"), payload, 0o644); err != nil {
		t.Fatalf("write product record: %v", err)
	}
}

// errorCodeOf 提取带编码错误中的错误码；无编码时返回空串。
func errorCodeOf(err error) string {
	var coded codedError
	if errors.As(err, &coded) {
		return coded.Code()
	}
	return ""
}

func assertErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error is nil, want code %s", want)
	}
	if got := errorCodeOf(err); got != want {
		t.Fatalf("error code = %q, want %q (message: %v)", got, want, err)
	}
}

func activateProgramStoreVersion(t *testing.T, root string, version string) {
	t.Helper()
	store, err := release.NewProgramStore(root, types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox})
	if err != nil {
		t.Fatalf("NewProgramStore() error = %v", err)
	}
	versionDir := filepath.Join(root, "versions", version)
	files, err := release.CollectFileRecords(versionDir)
	if err != nil {
		t.Fatalf("CollectFileRecords() error = %v", err)
	}
	prepared := release.PreparedProgram{Version: version, Directory: versionDir, Files: files}
	if err := store.Activate(context.Background(), prepared, ""); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
}

func newRecoverTestManager(t *testing.T) (*localBoxManager, localBoxPaths) {
	t.Helper()
	paths, err := newLocalBoxPaths(filepath.Join(t.TempDir()))
	if err != nil {
		t.Fatalf("newLocalBoxPaths() error = %v", err)
	}
	return newLocalBoxManager(paths, nil, nil, nil, nil), paths
}

func writeOperationRecord(t *testing.T, paths localBoxPaths, record release.OperationRecord) {
	t.Helper()
	record.UpdatedAt = timeNowUTC()
	record.StartedAt = timeNowUTC()
	if err := release.WriteOperationRecord(filepath.Join(paths.programStoreDir, "operation.json"), record); err != nil {
		t.Fatalf("WriteOperationRecord() error = %v", err)
	}
}

func timeNowUTC() (value time.Time) { return time.Now().UTC() }

func operationRecord(phase string, result string, currentVersion string, targetVersion string, workDir string) release.OperationRecord {
	return release.OperationRecord{
		SchemaVersion:  1,
		OperationID:    "update-20260813T100000.000000000Z",
		Artifact:       types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox},
		Action:         release.OperationActionUpdate,
		TargetVersion:  targetVersion,
		Phase:          phase,
		Result:         result,
		CurrentVersion: currentVersion,
		WorkDirectory:  workDir,
	}
}

func TestRecoverPendingUpdateWithoutRecord(t *testing.T) {
	manager, _ := newRecoverTestManager(t)
	if err := manager.recoverPendingUpdate(context.Background()); err != nil {
		t.Fatalf("recoverPendingUpdate() error = %v", err)
	}
}

func TestRecoverPendingUpdateFailedRecordIsKept(t *testing.T) {
	manager, paths := newRecoverTestManager(t)
	workDir := filepath.Join(paths.workDir, "update-20260813T100000.000000000Z")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	writeOperationRecord(t, paths, operationRecord(types.ArtifactPhaseProbe, release.OperationResultFailed, "0.1.0", "0.2.0", workDir))
	if err := manager.recoverPendingUpdate(context.Background()); err != nil {
		t.Fatalf("recoverPendingUpdate() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.programStoreDir, "operation.json")); err != nil {
		t.Fatalf("failed operation record must be kept: %v", err)
	}
	if _, err := os.Stat(workDir); err != nil {
		t.Fatalf("failed operation work dir must be kept: %v", err)
	}
}

func TestRecoverPendingUpdatePreSwitchPhases(t *testing.T) {
	for _, phase := range []string{
		types.ArtifactPhaseCandidate, types.ArtifactPhaseCompatibility, types.ArtifactPhaseActivity,
		types.ArtifactPhaseDownload, types.ArtifactPhaseManifest, types.ArtifactPhaseArchive,
		types.ArtifactPhasePackage, types.ArtifactPhasePrepare,
	} {
		t.Run(phase, func(t *testing.T) {
			manager, paths := newRecoverTestManager(t)
			workDir := filepath.Join(paths.workDir, "update-20260813T100000.000000000Z")
			if err := os.MkdirAll(workDir, 0o755); err != nil {
				t.Fatalf("mkdir work: %v", err)
			}
			writeOperationRecord(t, paths, operationRecord(phase, release.OperationResultRunning, "0.1.0", "0.2.0", workDir))
			if err := manager.recoverPendingUpdate(context.Background()); err != nil {
				t.Fatalf("recoverPendingUpdate() error = %v", err)
			}
			if _, err := os.Stat(filepath.Join(paths.programStoreDir, "operation.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("operation record must be removed: %v", err)
			}
			if _, err := os.Stat(workDir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("work dir must be removed: %v", err)
			}
		})
	}
}

func TestRecoverPendingUpdatePostSwitchSucceeded(t *testing.T) {
	manager, paths := newRecoverTestManager(t)
	storeRoot := paths.programStoreDir
	seedProgramStoreVersion(t, storeRoot, "0.1.0", "1.0.0")
	seedProgramStoreVersion(t, storeRoot, "0.2.0", "1.0.0")
	activateProgramStoreVersion(t, storeRoot, "0.2.0")
	workDir := filepath.Join(paths.workDir, "update-20260813T100000.000000000Z")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	writeOperationRecord(t, paths, operationRecord(types.ArtifactPhaseProbe, release.OperationResultRunning, "0.1.0", "0.2.0", workDir))
	if err := datastorage.WriteStorageVersion(context.Background(), paths.dataDir, datastorage.StorageVersion{Version: "1.0.0", CreatedAt: timeNowUTC(), UpdatedAt: timeNowUTC()}); err != nil {
		t.Fatalf("WriteStorageVersion() error = %v", err)
	}
	if err := manager.recoverPendingUpdate(context.Background()); err != nil {
		t.Fatalf("recoverPendingUpdate() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.programStoreDir, "operation.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("operation record must be removed after confirmed success: %v", err)
	}
	store, err := release.NewProgramStore(storeRoot, types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox})
	if err != nil {
		t.Fatalf("NewProgramStore() error = %v", err)
	}
	current, err := store.Current()
	if err != nil || current.Version != "0.2.0" {
		t.Fatalf("current = %#v err=%v", current, err)
	}
}

func TestRecoverPendingUpdatePostSwitchUnfinishedMigration(t *testing.T) {
	manager, paths := newRecoverTestManager(t)
	storeRoot := paths.programStoreDir
	seedProgramStoreVersion(t, storeRoot, "0.1.0", "1.0.0")
	seedProgramStoreVersion(t, storeRoot, "0.2.0", "1.2.0")
	activateProgramStoreVersion(t, storeRoot, "0.2.0")
	workDir := filepath.Join(paths.workDir, "update-20260813T100000.000000000Z")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	writeOperationRecord(t, paths, operationRecord(types.ArtifactPhaseSwitch, release.OperationResultRunning, "0.1.0", "0.2.0", workDir))
	// 存在未完成迁移：写 process.json（数据版本仍是旧版）
	workspaceDir := datamigration.WorkspaceDir(paths.dataDir)
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		t.Fatalf("mkdir migration workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "process.json"), []byte(`{"schemaVersion":1,"fromVersion":"1.0.0","targetVersion":"1.2.0","stepIDs":["1.0.0-to-1.2.0"],"currentIndex":0,"stepResults":[],"backup":{"runID":"r","manifest":"m","verified":true},"directive":"continue","startedAt":"2026-08-13T10:00:00Z","updatedAt":"2026-08-13T10:00:00Z"}`), 0o600); err != nil {
		t.Fatalf("write process.json: %v", err)
	}
	if err := datastorage.WriteStorageVersion(context.Background(), paths.dataDir, datastorage.StorageVersion{Version: "1.0.0", CreatedAt: timeNowUTC(), UpdatedAt: timeNowUTC()}); err != nil {
		t.Fatalf("WriteStorageVersion() error = %v", err)
	}
	if err := manager.recoverPendingUpdate(context.Background()); err != nil {
		t.Fatalf("recoverPendingUpdate() error = %v", err)
	}
	record, err := release.ReadOperationRecord(filepath.Join(paths.programStoreDir, "operation.json"))
	if err != nil {
		t.Fatalf("operation record must be kept with failed result: %v", err)
	}
	if record.Result != release.OperationResultFailed {
		t.Fatalf("operation result = %q, want failed", record.Result)
	}
	if !strings.Contains(record.ErrorMessage, "未完成迁移") {
		t.Fatalf("error message = %q", record.ErrorMessage)
	}
}

func TestRecoverPendingUpdatePostSwitchReverted(t *testing.T) {
	manager, paths := newRecoverTestManager(t)
	storeRoot := paths.programStoreDir
	seedProgramStoreVersion(t, storeRoot, "0.1.0", "1.0.0")
	seedProgramStoreVersion(t, storeRoot, "0.2.0", "1.0.0")
	activateProgramStoreVersion(t, storeRoot, "0.1.0")
	workDir := filepath.Join(paths.workDir, "update-20260813T100000.000000000Z")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	writeOperationRecord(t, paths, operationRecord(types.ArtifactPhaseSwitch, release.OperationResultRunning, "0.1.0", "0.2.0", workDir))
	if err := manager.recoverPendingUpdate(context.Background()); err != nil {
		t.Fatalf("recoverPendingUpdate() error = %v", err)
	}
	record, err := release.ReadOperationRecord(filepath.Join(paths.programStoreDir, "operation.json"))
	if err != nil {
		t.Fatalf("operation record must be kept: %v", err)
	}
	if record.Result != release.OperationResultFailed || !strings.Contains(record.ErrorMessage, "更新未生效") {
		t.Fatalf("record = %#v", record)
	}
	if _, err := os.Stat(workDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("work dir must be removed: %v", err)
	}
}

func TestRecoverPendingUpdatePostSwitchUninterpretableVersion(t *testing.T) {
	manager, paths := newRecoverTestManager(t)
	storeRoot := paths.programStoreDir
	seedProgramStoreVersion(t, storeRoot, "0.1.0", "1.0.0")
	seedProgramStoreVersion(t, storeRoot, "0.3.0", "1.0.0")
	activateProgramStoreVersion(t, storeRoot, "0.3.0")
	workDir := filepath.Join(paths.workDir, "update-20260813T100000.000000000Z")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	writeOperationRecord(t, paths, operationRecord(types.ArtifactPhaseProbe, release.OperationResultRunning, "0.1.0", "0.2.0", workDir))
	err := manager.recoverPendingUpdate(context.Background())
	assertErrorCode(t, err, "LOCAL_BOX_INSTALL_FAILED")
}

func TestRecoverPendingUpdateCorruptRecordKeptAfterRename(t *testing.T) {
	manager, paths := newRecoverTestManager(t)
	storeRoot := paths.programStoreDir
	seedProgramStoreVersion(t, storeRoot, "0.1.0", "1.0.0")
	activateProgramStoreVersion(t, storeRoot, "0.1.0")
	if err := os.MkdirAll(paths.programStoreDir, 0o755); err != nil {
		t.Fatalf("mkdir store: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.programStoreDir, "operation.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt record: %v", err)
	}
	if err := manager.recoverPendingUpdate(context.Background()); err != nil {
		t.Fatalf("recoverPendingUpdate() error = %v", err)
	}
	entries, err := os.ReadDir(paths.programStoreDir)
	if err != nil {
		t.Fatalf("read store dir: %v", err)
	}
	foundInvalid := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "operation.json.invalid-") {
			foundInvalid = true
		}
	}
	if !foundInvalid {
		t.Fatalf("corrupt record was not renamed and kept: %v", entries)
	}
}

func TestRecoverPendingUpdateCorruptRecordWithBrokenStore(t *testing.T) {
	manager, paths := newRecoverTestManager(t)
	if err := os.MkdirAll(paths.programStoreDir, 0o755); err != nil {
		t.Fatalf("mkdir store: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.programStoreDir, "operation.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt record: %v", err)
	}
	err := manager.recoverPendingUpdate(context.Background())
	assertErrorCode(t, err, "LOCAL_BOX_INSTALL_FAILED")
	if _, statErr := os.Stat(filepath.Join(paths.programStoreDir, "operation.json")); statErr != nil {
		t.Fatalf("corrupt record must not be deleted: %v", statErr)
	}
}

func TestRecoverPendingUpdatePostSwitchUnsafeData(t *testing.T) {
	manager, paths := newRecoverTestManager(t)
	storeRoot := paths.programStoreDir
	seedProgramStoreVersion(t, storeRoot, "0.1.0", "1.0.0")
	seedProgramStoreVersion(t, storeRoot, "0.2.0", "1.0.0")
	activateProgramStoreVersion(t, storeRoot, "0.2.0")
	workDir := filepath.Join(paths.workDir, "update-20260813T100000.000000000Z")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	writeOperationRecord(t, paths, operationRecord(types.ArtifactPhaseProbe, release.OperationResultRunning, "0.1.0", "0.2.0", workDir))
	// 版本事实不全（无 meta/version.json、无上一版数据目标）→ unsafe
	err := manager.recoverPendingUpdate(context.Background())
	assertErrorCode(t, err, "LOCAL_BOX_DATA_UNSAFE")
	if _, statErr := os.Stat(workDir); statErr != nil {
		t.Fatalf("unsafe scene must be retained: %v", statErr)
	}
}

func TestRecoverPendingUpdatePostSwitchRestorePrevious(t *testing.T) {
	manager, paths := newRecoverTestManager(t)
	storeRoot := paths.programStoreDir
	seedProgramStoreVersion(t, storeRoot, "0.1.0", "1.2.0")
	seedProgramStoreVersion(t, storeRoot, "0.2.0", "1.2.0")
	activateProgramStoreVersion(t, storeRoot, "0.2.0")
	workDir := filepath.Join(paths.workDir, "update-20260813T100000.000000000Z")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	writeOperationRecord(t, paths, operationRecord(types.ArtifactPhaseProbe, release.OperationResultRunning, "0.1.0", "0.2.0", workDir))
	// 数据版本低于上一版数据目标（迁移被回滚）：决策为恢复上一版
	if err := datastorage.WriteStorageVersion(context.Background(), paths.dataDir, datastorage.StorageVersion{Version: "1.0.0", CreatedAt: timeNowUTC(), UpdatedAt: timeNowUTC()}); err != nil {
		t.Fatalf("WriteStorageVersion() error = %v", err)
	}
	if err := manager.recoverPendingUpdate(context.Background()); err != nil {
		t.Fatalf("recoverPendingUpdate() error = %v", err)
	}
	store, err := release.NewProgramStore(storeRoot, types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox})
	if err != nil {
		t.Fatalf("NewProgramStore() error = %v", err)
	}
	current, err := store.Current()
	if err != nil || current.Version != "0.1.0" {
		t.Fatalf("current = %#v err=%v, want previous version", current, err)
	}
	if _, statErr := os.Stat(workDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("work dir must be removed: %v", statErr)
	}
}
