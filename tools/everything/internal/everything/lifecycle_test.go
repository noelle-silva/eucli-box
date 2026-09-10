package everything

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestLoadKeepAliveSettingsDefaultsToDisabled(t *testing.T) {
	settings, err := loadKeepAliveSettings(types.ToolExecutionInput{})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Enabled || settings.Seconds != defaultKeepAliveSeconds {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestLoadKeepAliveSettingsAcceptsUserConfig(t *testing.T) {
	settings, err := loadKeepAliveSettings(types.ToolExecutionInput{
		UserConfig: map[string]any{"keepAliveEnabled": true, "keepAliveSeconds": 42},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Enabled || settings.Seconds != 42 {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestLoadKeepAliveSettingsValidatesSeconds(t *testing.T) {
	if _, err := loadKeepAliveSettings(types.ToolExecutionInput{
		UserConfig: map[string]any{"keepAliveEnabled": true, "keepAliveSeconds": 0},
	}); err == nil {
		t.Fatal("keepAliveSeconds must be positive")
	}
}

func TestRetireBundledRuntimeDisabledStopsInstance(t *testing.T) {
	fixture := newEverythingFixture(t, true)
	runtimeRootDir, err := bundledRuntimeDir(fixture.dataDir, fixtureConfig().Runtime.Directory)
	if err != nil {
		t.Fatal(err)
	}
	instanceDir := bundledInstanceRuntimeDir(runtimeRootDir, "eucli-box-everything-test-scoped")
	if err := os.MkdirAll(instanceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instanceDir, keepAliveStateFileName), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := selectedProvider{ID: "bundled", ESExecutable: fixture.esExe, Bundled: true}
	err = retireBundledRuntime(fixture.dataDir, fixtureConfig(), provider,
		searchRequest{InstanceName: "eucli-box-everything-test-scoped"},
		types.ToolExecutionInput{})
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(instanceDir, keepAliveStateFileName)); !os.IsNotExist(statErr) {
		t.Fatalf("disabled keep-alive must remove state file, stat err = %v", statErr)
	}
}

func TestRetireBundledRuntimeEnabledWritesStateAndSpawnsGuard(t *testing.T) {
	fixture := newEverythingFixture(t, true)
	spawned := make(chan []string, 1)
	previous := spawnKeepAliveGuard
	spawnKeepAliveGuard = func(stateFile string, guardLock string) error {
		spawned <- []string{stateFile, guardLock}
		return nil
	}
	t.Cleanup(func() { spawnKeepAliveGuard = previous })
	provider := selectedProvider{ID: "bundled", ESExecutable: fixture.esExe, Bundled: true}
	err := retireBundledRuntime(fixture.dataDir, fixtureConfig(), provider,
		searchRequest{InstanceName: "eucli-box-everything-test-scoped"},
		types.ToolExecutionInput{UserConfig: map[string]any{"keepAliveEnabled": true, "keepAliveSeconds": 30}})
	if err != nil {
		t.Fatal(err)
	}
	paths := <-spawned
	runtimeRootDir, err := bundledRuntimeDir(fixture.dataDir, fixtureConfig().Runtime.Directory)
	if err != nil {
		t.Fatal(err)
	}
	instanceDir := bundledInstanceRuntimeDir(runtimeRootDir, "eucli-box-everything-test-scoped")
	if paths[0] != filepath.Join(instanceDir, keepAliveStateFileName) {
		t.Fatalf("state file = %s", paths[0])
	}
	state, err := readKeepAliveState(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if state.KeepAliveSeconds != 30 || state.LastUsedAt.IsZero() {
		t.Fatalf("state = %#v", state)
	}
}

func TestRunKeepAliveGuardStopsInstanceAfterIdle(t *testing.T) {
	fixture := newEverythingFixture(t, true)
	runtimeRootDir, err := bundledRuntimeDir(fixture.dataDir, fixtureConfig().Runtime.Directory)
	if err != nil {
		t.Fatal(err)
	}
	instanceDir := bundledInstanceRuntimeDir(runtimeRootDir, "eucli-box-everything-test-scoped")
	if err := os.MkdirAll(instanceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(instanceDir, keepAliveStateFileName)
	guardLock := filepath.Join(instanceDir, keepAliveGuardLockName)
	state := keepAliveState{
		ESExecutable:     fixture.esExe,
		InstanceName:     "eucli-box-everything-test-scoped",
		KeepAliveSeconds: 1,
		ConnectTimeoutMs: 5000,
		LastUsedAt:       time.Now().UTC().Add(-2 * time.Second),
	}
	if err := writeKeepAliveState(stateFile, state); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- runKeepAliveGuard(stateFile, guardLock) }()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("guard did not stop the idle instance in time")
	}
	if _, statErr := os.Stat(stateFile); !os.IsNotExist(statErr) {
		t.Fatalf("guard must remove its state file, stat err = %v", statErr)
	}
	if _, statErr := os.Stat(guardLock); !os.IsNotExist(statErr) {
		t.Fatalf("guard must remove its lock, stat err = %v", statErr)
	}
}
