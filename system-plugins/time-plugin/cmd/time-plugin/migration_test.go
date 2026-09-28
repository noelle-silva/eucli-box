package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDataMigrationModeInitializesDataVersion(t *testing.T) {
	dataDir := t.TempDir()
	var output bytes.Buffer
	code := runDataMigration([]string{dataMigrationFlag, dataDir}, &output)
	if code != 0 {
		t.Fatalf("exit code = %d, output = %s", code, output.String())
	}
	var result migrationResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("result decode failed: %v", err)
	}
	if result.Status != "success" || result.MigrationState != "data-unchanged" {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "data-version.json")); err != nil {
		t.Fatalf("data version file missing: %v", err)
	}
}

func TestDataMigrationModeIsIdempotent(t *testing.T) {
	dataDir := t.TempDir()
	var output bytes.Buffer
	if code := runDataMigration([]string{dataMigrationFlag, dataDir}, &output); code != 0 {
		t.Fatalf("first exit code = %d", code)
	}
	output.Reset()
	code := runDataMigration([]string{dataMigrationFlag, dataDir}, &output)
	if code != 0 {
		t.Fatalf("second exit code = %d, output = %s", code, output.String())
	}
	var result migrationResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("result decode failed: %v", err)
	}
	if result.MigrationState != "data-unchanged" {
		t.Fatalf("migrationState = %q", result.MigrationState)
	}
}

func TestDataMigrationModeRequiresDataDirectory(t *testing.T) {
	var output bytes.Buffer
	if code := runDataMigration([]string{dataMigrationFlag}, &output); code == 0 {
		t.Fatal("data migration without data directory must fail")
	}
}

func TestIsDataMigrationMode(t *testing.T) {
	if !isDataMigrationMode([]string{dataMigrationFlag, "x"}) {
		t.Fatal("data migration flag was not recognized")
	}
	if isDataMigrationMode(nil) || isDataMigrationMode([]string{"--other"}) {
		t.Fatal("non-migration args were recognized as data migration mode")
	}
}
