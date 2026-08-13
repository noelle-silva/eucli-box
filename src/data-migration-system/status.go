package datamigration

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"eucli-box/pkg/release"
)

const statusSchemaVersion = 1

// statusRecord 是四态结果交接面，持久保存在工作区根 status.json。
type statusRecord struct {
	SchemaVersion      int      `json:"schemaVersion"`
	Outcome            string   `json:"outcome"`
	FromVersion        string   `json:"fromVersion"`
	TargetVersion      string   `json:"targetVersion"`
	CurrentDataVersion string   `json:"currentDataVersion"`
	StepIDs            []string `json:"stepIDs"`
	Completed          bool     `json:"completed"`
	Detail             string   `json:"detail"`
	UpdatedAt          string   `json:"updatedAt"`
}

func newStatusRecord(outcome Outcome, currentDataVersion string, stepIDs []string, completed bool) (statusRecord, error) {
	if !validState(outcome.State) {
		return statusRecord{}, fmt.Errorf("status outcome %q is outside the fixed vocabulary", outcome.State)
	}
	return statusRecord{
		SchemaVersion:      statusSchemaVersion,
		Outcome:            string(outcome.State),
		FromVersion:        outcome.From,
		TargetVersion:      outcome.To,
		CurrentDataVersion: currentDataVersion,
		StepIDs:            append([]string{}, stepIDs...),
		Completed:          completed,
		Detail:             outcome.Detail,
		UpdatedAt:          time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func writeStatusRecord(w workspace, record statusRecord) error {
	if err := validateStatusRecord(record); err != nil {
		return err
	}
	return writeWorkspaceJSON(w.statusFile(), record)
}

func readStatusRecord(w workspace) (statusRecord, bool, error) {
	payload, err := os.ReadFile(w.statusFile())
	if err != nil {
		if os.IsNotExist(err) {
			return statusRecord{}, false, nil
		}
		return statusRecord{}, false, migrationStatusUnknown("failed to read status record", err)
	}
	var record statusRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return statusRecord{}, false, migrationStatusUnknown("failed to decode status record", err)
	}
	if err := validateStatusRecord(record); err != nil {
		return statusRecord{}, false, migrationStatusUnknown("status record is invalid", err)
	}
	return record, true, nil
}

func validateStatusRecord(record statusRecord) error {
	if record.SchemaVersion != statusSchemaVersion {
		return fmt.Errorf("status record schema version %d is not supported", record.SchemaVersion)
	}
	if !validState(State(record.Outcome)) {
		return fmt.Errorf("status record outcome %q is outside the fixed vocabulary", record.Outcome)
	}
	if err := release.ValidateVersion(record.CurrentDataVersion); err != nil {
		return fmt.Errorf("status record current data version is invalid: %w", err)
	}
	if record.FromVersion != "" {
		if err := release.ValidateVersion(record.FromVersion); err != nil {
			return fmt.Errorf("status record from version is invalid: %w", err)
		}
	}
	if record.TargetVersion != "" {
		if err := release.ValidateVersion(record.TargetVersion); err != nil {
			return fmt.Errorf("status record target version is invalid: %w", err)
		}
	}
	if record.UpdatedAt == "" {
		return fmt.Errorf("status record updated at is missing")
	}
	return nil
}

func validState(state State) bool {
	switch state {
	case StateDataUnchanged, StateMigrated, StateRecovered, StateRecoveryFailed:
		return true
	}
	return false
}
