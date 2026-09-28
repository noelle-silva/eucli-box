package systemplugin

import (
	"context"
	"path/filepath"
	"testing"

	"eucli-box/pkg/types"
)

// TestPluginInstallRunsDataMigrationGate 验证插件安装的切换前会以专门迁移模式
// 唤起新版本插件迁移自己的数据；数据未变化或迁移成功才允许激活。
func TestPluginInstallRunsDataMigrationGate(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	track := filepath.Join(fixture.dataRoot, "migration-track.jsonl")
	fixture.makePluginCandidate("gate-plugin", "0.1.0", onDemandHosting(), false, map[string]any{
		"values":    map[string]string{"value": "gate"},
		"trackFile": track,
	})
	state := fixture.installPluginAndWait(t, "gate-plugin")
	if state.Status != types.ArtifactStatusActive {
		t.Fatalf("install state = %#v", state)
	}
	if len(trackEntriesOfKind(t, track, "data-migration")) < 1 {
		t.Fatalf("data migration was not invoked: %#v", readTrackEntries(t, track))
	}
}

// TestPluginInstallRejectsRecoveredDataMigration 验证迁移未完成（数据已恢复）时新版本不激活。
func TestPluginInstallRejectsRecoveredDataMigration(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("recovered-plugin", "0.1.0", onDemandHosting(), false, map[string]any{
		"migrationState": "recovered",
	})
	state := fixture.installPluginAndWait(t, "recovered-plugin")
	if state.Status != types.ArtifactStatusFailed {
		t.Fatalf("install state = %#v", state)
	}
	if state.Error.Code != types.ArtifactErrorDataMigrationFailed || state.Error.Phase != types.ArtifactPhaseMigration {
		t.Fatalf("install error = %#v", state.Error)
	}
	if state.Installed || state.CurrentVersion != "" {
		t.Fatalf("plugin must not be activated after failed data migration: %#v", state)
	}
}

// TestPluginInstallRejectsFailedDataMigration 验证迁移失败时新版本不激活。
func TestPluginInstallRejectsFailedDataMigration(t *testing.T) {
	fixture := newPluginOperationFixture(t)
	fixture.makePluginCandidate("failed-plugin", "0.1.0", onDemandHosting(), false, map[string]any{
		"migrationFailure": true,
	})
	state := fixture.installPluginAndWait(t, "failed-plugin")
	if state.Status != types.ArtifactStatusFailed {
		t.Fatalf("install state = %#v", state)
	}
	if state.Error.Code != types.ArtifactErrorDataMigrationFailed || state.Error.Phase != types.ArtifactPhaseMigration {
		t.Fatalf("install error = %#v", state.Error)
	}
	if state.Installed || state.CurrentVersion != "" {
		t.Fatalf("plugin must not be activated after failed data migration: %#v", state)
	}
}

// TestResidentPluginStartRunsDataMigrationGate 验证常驻插件装载前也会过数据迁移关卡。
func TestResidentPluginStartRunsDataMigrationGate(t *testing.T) {
	root := t.TempDir()
	track := filepath.Join(root, "track.jsonl")
	writeTestPlugin(t, root, testPluginSpec{
		manifest: testManifest("resident-gate-plugin", "ResidentGate", residentHosting(types.SystemPluginStartBoot)),
		behavior: map[string]any{"trackFile": track, "values": map[string]string{"value": "ok"}},
	})
	system := newTestSystem(t, root)
	if err := system.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(trackEntriesOfKind(t, track, "data-migration")) != 1 {
		t.Fatalf("data migration was not invoked: %#v", readTrackEntries(t, track))
	}
}
