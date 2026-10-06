package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"eucli-box/tools/ai-image/internal/types"
)

func TestDataMigrationModeInitializesDataVersion(t *testing.T) {
	dataDir := t.TempDir()
	input := types.ToolExecutionInput{RequestKind: migrationRequestKind, ToolDataDirectory: dataDir}
	output := runDataMigration(context.Background(), input)
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("data migration status = %v, error = %s", output.Status, output.Error)
	}
	if output.Metadata["migrationState"] != "data-unchanged" {
		t.Fatalf("migrationState = %v", output.Metadata["migrationState"])
	}
	if _, err := os.Stat(filepath.Join(dataDir, "data-version.json")); err != nil {
		t.Fatalf("data version file missing: %v", err)
	}
}

func TestDataMigrationModeIsIdempotent(t *testing.T) {
	dataDir := t.TempDir()
	input := types.ToolExecutionInput{RequestKind: migrationRequestKind, ToolDataDirectory: dataDir}
	runDataMigration(context.Background(), input)
	output := runDataMigration(context.Background(), input)
	if output.Status != types.ToolStatusSuccess || output.Metadata["migrationState"] != "data-unchanged" {
		t.Fatalf("second data migration result = %#v", output)
	}
}

func TestDataMigrationModeRequiresDataDirectory(t *testing.T) {
	output := runDataMigration(context.Background(), types.ToolExecutionInput{RequestKind: migrationRequestKind})
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("status = %v, want failed", output.Status)
	}
}
