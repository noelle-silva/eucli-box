package datamigration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"eucli-box/pkg/release"
)

const (
	processSchemaVersion = 1
	directiveContinue    = "continue"
	phaseVerified        = "verified"
)

// processRecord 是迁移过程记录，持久保存在工作区根 process.json。
type processRecord struct {
	SchemaVersion int                 `json:"schemaVersion"`
	FromVersion   string              `json:"fromVersion"`
	TargetVersion string              `json:"targetVersion"`
	StepIDs       []string            `json:"stepIDs"`
	CurrentIndex  int                 `json:"currentIndex"`
	StepResults   []processStepResult `json:"stepResults"`
	Backup        processBackupInfo   `json:"backup"`
	Directive     string              `json:"directive"`
	StartedAt     string              `json:"startedAt"`
	UpdatedAt     string              `json:"updatedAt"`
}

type processStepResult struct {
	StepID            string `json:"stepID"`
	Phase             string `json:"phase"`
	DataVersionWritten string `json:"dataVersionWritten"`
	CheckedAt         string `json:"checkedAt"`
}

type processBackupInfo struct {
	RunID    string `json:"runID"`
	Manifest string `json:"manifest"`
	Verified bool   `json:"verified"`
}

func newProcessRecord(fromVersion string, targetVersion string, stepIDs []string, backup processBackupInfo) processRecord {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return processRecord{
		SchemaVersion: processSchemaVersion,
		FromVersion:   fromVersion,
		TargetVersion: targetVersion,
		StepIDs:       append([]string{}, stepIDs...),
		CurrentIndex:  0,
		StepResults:   []processStepResult{},
		Backup:        backup,
		Directive:     directiveContinue,
		StartedAt:     now,
		UpdatedAt:     now,
	}
}

// appendVerifiedStep 记录一级已核对完成的步骤并推进当前索引。
func (r *processRecord) appendVerifiedStep(step Step, checkedAt string) {
	r.StepResults = append(r.StepResults, processStepResult{
		StepID:             step.ID,
		Phase:              phaseVerified,
		DataVersionWritten: step.ToVersion,
		CheckedAt:          checkedAt,
	})
	r.CurrentIndex = len(r.StepResults)
	r.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
}

func writeProcessRecord(w workspace, record processRecord) error {
	if err := validateProcessRecord(record); err != nil {
		return err
	}
	return writeWorkspaceJSON(w.processFile(), record)
}

func readProcessRecord(w workspace) (processRecord, bool, error) {
	payload, err := os.ReadFile(w.processFile())
	if err != nil {
		if os.IsNotExist(err) {
			return processRecord{}, false, nil
		}
		return processRecord{}, false, migrationStatusUnknown("failed to read process record", err)
	}
	var record processRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return processRecord{}, false, migrationStatusUnknown("failed to decode process record", err)
	}
	if err := validateProcessRecord(record); err != nil {
		return processRecord{}, false, migrationStatusUnknown("process record is invalid", err)
	}
	return record, true, nil
}

func deleteProcessRecord(w workspace) error {
	if err := os.Remove(w.processFile()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove process record: %w", err)
	}
	return nil
}

func validateProcessRecord(record processRecord) error {
	if record.SchemaVersion != processSchemaVersion {
		return fmt.Errorf("process record schema version %d is not supported", record.SchemaVersion)
	}
	if err := release.ValidateVersion(record.FromVersion); err != nil {
		return fmt.Errorf("process record from version is invalid: %w", err)
	}
	if err := release.ValidateVersion(record.TargetVersion); err != nil {
		return fmt.Errorf("process record target version is invalid: %w", err)
	}
	comparison, err := release.CompareVersions(record.FromVersion, record.TargetVersion)
	if err != nil || comparison >= 0 {
		return fmt.Errorf("process record versions must move forward")
	}
	if len(record.StepIDs) == 0 {
		return fmt.Errorf("process record must declare step ids")
	}
	if record.CurrentIndex < 0 || record.CurrentIndex > len(record.StepIDs) {
		return fmt.Errorf("process record current index is out of range")
	}
	if len(record.StepResults) != record.CurrentIndex {
		return fmt.Errorf("process record step results do not match current index")
	}
	for _, result := range record.StepResults {
		if result.Phase != phaseVerified || result.StepID == "" || result.DataVersionWritten == "" || result.CheckedAt == "" {
			return fmt.Errorf("process record contains invalid step result")
		}
	}
	if record.Backup.RunID == "" || record.Backup.Manifest == "" {
		return fmt.Errorf("process record backup info is incomplete")
	}
	if record.Directive != directiveContinue {
		return fmt.Errorf("process record directive is not continue")
	}
	if record.StartedAt == "" || record.UpdatedAt == "" {
		return fmt.Errorf("process record timestamps are missing")
	}
	return nil
}

// writeWorkspaceJSON 以临时文件加原子替换写入工作区 JSON 文件。
func writeWorkspaceJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
