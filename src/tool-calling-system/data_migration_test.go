package toolcalling

import (
	"testing"

	"eucli-box/pkg/types"
)

// migrationRecoveredToolSource 是迁移模式回报「已恢复」的替身工具：
// 数据未迁移，新版本不应被激活。
const migrationRecoveredToolSource = `
package main

import (
	"encoding/json"
	"os"
)

type input struct {
	RequestKind string ` + "`json:\"requestKind\"`" + `
}

func main() {
	var in input
	_ = json.NewDecoder(os.Stdin).Decode(&in)
	if in.RequestKind == "migration" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"status":   "success",
			"content":  "recovered",
			"metadata": map[string]any{"migrationState": "recovered", "from": "1.0.0", "to": "1.0.0"},
		})
		return
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "success", "content": "ok"})
}
`

// migrationFailedToolSource 是迁移模式回报失败的替身工具：新版本不应被激活。
const migrationFailedToolSource = `
package main

import (
	"encoding/json"
	"os"
)

type input struct {
	RequestKind string ` + "`json:\"requestKind\"`" + `
}

func main() {
	var in input
	_ = json.NewDecoder(os.Stdin).Decode(&in)
	if in.RequestKind == "migration" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"status":  "failed",
			"content": "migration exploded",
			"error":   "migration exploded",
		})
		return
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "success", "content": "ok"})
}
`

func TestInstallRejectsToolWhoseDataMigrationDidNotComplete(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidateFromSource("migration-gate", "0.1.0", migrationRecoveredToolSource, false)
	state := fixture.installToolAndWait(t, "migration-gate")
	if state.Status != types.ArtifactStatusFailed {
		t.Fatalf("install state = %#v", state)
	}
	if state.Error.Code != types.ArtifactErrorDataMigrationFailed || state.Error.Phase != types.ArtifactPhaseMigration {
		t.Fatalf("install error = %#v", state.Error)
	}
	if state.CurrentVersion != "" || state.Installed {
		t.Fatalf("tool must not be activated after failed data migration: %#v", state)
	}
}

func TestInstallRejectsToolWithFailedDataMigration(t *testing.T) {
	fixture := newToolOperationFixture(t)
	fixture.makeToolCandidateFromSource("migration-gate", "0.1.0", migrationFailedToolSource, false)
	state := fixture.installToolAndWait(t, "migration-gate")
	if state.Status != types.ArtifactStatusFailed {
		t.Fatalf("install state = %#v", state)
	}
	if state.Error.Code != types.ArtifactErrorDataMigrationFailed || state.Error.Phase != types.ArtifactPhaseMigration {
		t.Fatalf("install error = %#v", state.Error)
	}
	if state.CurrentVersion != "" || state.Installed {
		t.Fatalf("tool must not be activated after failed data migration: %#v", state)
	}
}
