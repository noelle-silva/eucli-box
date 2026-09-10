package everything

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestInspectStewardHealthStates(t *testing.T) {
	sourceEngine := filepath.Join(t.TempDir(), "Everything.exe")
	if err := os.WriteFile(sourceEngine, []byte("engine"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseDir := t.TempDir()
	protectedEngine := filepath.Join(baseDir, stewardProtectedDirectoryName, "Everything.exe")
	if err := os.MkdirAll(filepath.Dir(protectedEngine), 0o755); err != nil {
		t.Fatal(err)
	}
	config := fixtureConfig()
	serviceName := stewardServiceName(config.Runtime.DefaultInstanceName)
	registered := fmt.Sprintf(`"%s" -instance "%s"`, protectedEngine, config.Runtime.DefaultInstanceName)

	cases := []struct {
		name      string
		service   stewardServiceStatus
		protected string
		want      stewardState
	}{
		{name: "not installed", service: stewardServiceStatus{}, want: stewardStateNotInstalled},
		{name: "path mismatch", service: stewardServiceStatus{Exists: true, Running: true, BinaryPath: `"C:\Elsewhere\Everything.exe"`}, want: stewardStatePathMismatch},
		{name: "version mismatch", service: stewardServiceStatus{Exists: true, Running: true, BinaryPath: registered}, protected: "outdated-engine", want: stewardStateVersionMismatch},
		{name: "stopped", service: stewardServiceStatus{Exists: true, Running: false, BinaryPath: registered}, want: stewardStateStopped},
		{name: "healthy", service: stewardServiceStatus{Exists: true, Running: true, BinaryPath: registered}, want: stewardStateHealthy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := tc.protected
			if content == "" {
				content = "engine"
			}
			if err := os.WriteFile(protectedEngine, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			stubStewardSystem(t, stewardSystem{
				protectedBaseDir: func() (string, error) { return baseDir, nil },
				queryService:     func(context.Context, string) (stewardServiceStatus, error) { return tc.service, nil },
			})
			status, err := inspectStewardHealth(context.Background(), config, sourceEngine)
			if err != nil {
				t.Fatal(err)
			}
			if status.State != tc.want {
				t.Fatalf("state = %q, want %q", status.State, tc.want)
			}
			if status.ServiceName != serviceName || status.ProtectedEnginePath != protectedEngine {
				t.Fatalf("status = %#v", status)
			}
		})
	}
}

func TestServiceExecutablePath(t *testing.T) {
	cases := []struct {
		commandLine string
		want        string
	}{
		{commandLine: `"C:\Program Files\eucli-box\Everything.exe" -instance "eucli-box-everything"`, want: `C:\Program Files\eucli-box\Everything.exe`},
		{commandLine: `C:\Everything.exe -instance "eucli-box-everything"`, want: `C:\Everything.exe`},
		{commandLine: `"C:\Everything.exe"`, want: `C:\Everything.exe`},
		{commandLine: "", want: ""},
	}
	for _, tc := range cases {
		if got := serviceExecutablePath(tc.commandLine); got != tc.want {
			t.Fatalf("serviceExecutablePath(%q) = %q, want %q", tc.commandLine, got, tc.want)
		}
	}
}

func TestAcquireStewardAuthorizeLockRejectsConcurrentAuthorization(t *testing.T) {
	directory := t.TempDir()
	lock, err := acquireStewardAuthorizeLock(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireStewardAuthorizeLock(directory); !errors.Is(err, errStewardAuthorizeBusy) {
		t.Fatalf("concurrent authorization err = %v", err)
	}
	lock.Release()
	second, err := acquireStewardAuthorizeLock(directory)
	if err != nil {
		t.Fatal(err)
	}
	second.Release()
}

func TestExecuteAuthorizeReturnsAlreadyAuthorized(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the permission steward is Windows-only")
	}
	fixture := newEverythingFixture(t, true)
	stubStewardHealth(t, stewardStateHealthy)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:         map[string]any{"action": "authorize"},
		ToolBodyDirectory: fixture.toolDir,
		ToolDataDirectory: fixture.dataDir,
	})
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Content, "Already Authorized") {
		t.Fatalf("content = %s", result.Content)
	}
	if result.Metadata["alreadyAuthorized"] != true || result.Metadata["serviceName"] != stewardServiceName(fixtureConfig().Runtime.DefaultInstanceName) {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteAuthorizeInstallsStewardWhenUnhealthy(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the permission steward is Windows-only")
	}
	fixture := newEverythingFixture(t, true)
	sourceEngine := filepath.Join(fixture.toolDir, "providers", "everything", executableName("Everything"))
	baseDir := t.TempDir()
	protectedEngine := filepath.Join(baseDir, stewardProtectedDirectoryName, filepath.Base(sourceEngine))
	instanceName := fixtureConfig().Runtime.DefaultInstanceName
	serviceName := stewardServiceName(instanceName)
	installed := false
	var handover stewardInstallConfig
	stubStewardSystem(t, stewardSystem{
		protectedBaseDir: func() (string, error) { return baseDir, nil },
		queryService: func(context.Context, string) (stewardServiceStatus, error) {
			if !installed {
				return stewardServiceStatus{}, nil
			}
			return stewardServiceStatus{Exists: true, Running: true, BinaryPath: fmt.Sprintf(`%q -instance %q`, protectedEngine, instanceName)}, nil
		},
		elevateInstall: func(configPath string) error {
			payload, err := os.ReadFile(configPath)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(payload, &handover); err != nil {
				return err
			}
			content, err := os.ReadFile(handover.SourceEngine)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(handover.TargetDirectory, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(handover.TargetEngine, content, 0o755); err != nil {
				return err
			}
			encoded, err := json.Marshal(stewardInstallResult{Success: true})
			if err != nil {
				return err
			}
			if err := os.WriteFile(handover.ResultFile, encoded, 0o644); err != nil {
				return err
			}
			installed = true
			return nil
		},
	})
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:         map[string]any{"action": "authorize"},
		ToolBodyDirectory: fixture.toolDir,
		ToolDataDirectory: fixture.dataDir,
	})
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if result.Metadata["stewardState"] != string(stewardStateHealthy) || result.Metadata["alreadyAuthorized"] != false {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
	if handover.SourceEngine != sourceEngine || handover.TargetEngine != protectedEngine || handover.TargetDirectory != filepath.Dir(protectedEngine) {
		t.Fatalf("handover = %#v", handover)
	}
	if handover.InstanceName != instanceName || handover.ServiceName != serviceName {
		t.Fatalf("handover = %#v", handover)
	}
	if _, err := os.Stat(filepath.Join(fixture.dataDir, stewardInstallConfigFileName)); !os.IsNotExist(err) {
		t.Fatalf("install config must be removed after use, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.dataDir, stewardInstallResultFileName)); !os.IsNotExist(err) {
		t.Fatalf("install result must be removed after use, stat err = %v", err)
	}
}

func TestExecuteAuthorizeReportsDeniedElevation(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the permission steward is Windows-only")
	}
	fixture := newEverythingFixture(t, true)
	stubStewardHealth(t, stewardStateNotInstalled)
	stubStewardSystem(t, stewardSystem{
		protectedBaseDir: func() (string, error) { return t.TempDir(), nil },
		elevateInstall:   func(string) error { return errStewardElevationDenied },
	})
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:         map[string]any{"action": "authorize"},
		ToolBodyDirectory: fixture.toolDir,
		ToolDataDirectory: fixture.dataDir,
	})
	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "denied") {
		t.Fatalf("result = %#v", result)
	}
	if result.Metadata["action"] != string(actionAuthorize) {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
	if _, err := os.Stat(filepath.Join(fixture.dataDir, stewardInstallConfigFileName)); !os.IsNotExist(err) {
		t.Fatalf("install config must be removed after a denied elevation, stat err = %v", err)
	}
}

func TestExecuteAuthorizeReportsUnfinishedInstall(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the permission steward is Windows-only")
	}
	fixture := newEverythingFixture(t, true)
	stubStewardHealth(t, stewardStateNotInstalled)
	stubStewardSystem(t, stewardSystem{
		protectedBaseDir: func() (string, error) { return t.TempDir(), nil },
		elevateInstall:   func(string) error { return nil },
	})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	result := Execute(ctx, types.ToolExecutionInput{
		Arguments:         map[string]any{"action": "authorize"},
		ToolBodyDirectory: fixture.toolDir,
		ToolDataDirectory: fixture.dataDir,
	})
	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "did not report a result") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunStewardInstallPublishesEngineAndReportsResult(t *testing.T) {
	sourceEngine := filepath.Join(t.TempDir(), "Everything.exe")
	if err := os.WriteFile(sourceEngine, []byte("engine-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseDir := t.TempDir()
	protectedEngine := filepath.Join(baseDir, stewardProtectedDirectoryName, "Everything.exe")
	resultFile := filepath.Join(t.TempDir(), stewardInstallResultFileName)
	configPath := filepath.Join(t.TempDir(), stewardInstallConfigFileName)
	instanceName := "eucli-box-everything-test"
	install := stewardInstallConfig{
		SourceEngine:    sourceEngine,
		TargetDirectory: filepath.Dir(protectedEngine),
		TargetEngine:    protectedEngine,
		InstanceName:    instanceName,
		ServiceName:     stewardServiceName(instanceName),
		ResultFile:      resultFile,
	}
	payload, err := json.Marshal(install)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	steps := []string{}
	serviceExists := false
	stubStewardSystem(t, stewardSystem{
		queryService: func(context.Context, string) (stewardServiceStatus, error) {
			if !serviceExists {
				return stewardServiceStatus{}, nil
			}
			return stewardServiceStatus{Exists: true, Running: true}, nil
		},
		deleteService: func(context.Context, string) error {
			steps = append(steps, "delete")
			serviceExists = false
			return nil
		},
		installService: func(_ context.Context, enginePath string, gotInstance string) error {
			steps = append(steps, "install")
			if enginePath != protectedEngine || gotInstance != instanceName {
				t.Errorf("installService(%q, %q)", enginePath, gotInstance)
			}
			serviceExists = true
			return nil
		},
		startService: func(context.Context, string) error {
			steps = append(steps, "start")
			return nil
		},
	})
	if err := RunStewardInstall(configPath); err != nil {
		t.Fatal(err)
	}
	if strings.Join(steps, ",") != "install,start" {
		t.Fatalf("steps = %v", steps)
	}
	content, err := os.ReadFile(protectedEngine)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "engine-bytes" {
		t.Fatalf("protected engine = %q", content)
	}
	result := readTestInstallResult(t, resultFile)
	if !result.Success {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunStewardInstallReplacesExistingService(t *testing.T) {
	sourceEngine := filepath.Join(t.TempDir(), "Everything.exe")
	if err := os.WriteFile(sourceEngine, []byte("engine-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseDir := t.TempDir()
	protectedEngine := filepath.Join(baseDir, stewardProtectedDirectoryName, "Everything.exe")
	resultFile := filepath.Join(t.TempDir(), stewardInstallResultFileName)
	configPath := filepath.Join(t.TempDir(), stewardInstallConfigFileName)
	install := stewardInstallConfig{
		SourceEngine:    sourceEngine,
		TargetDirectory: filepath.Dir(protectedEngine),
		TargetEngine:    protectedEngine,
		InstanceName:    "eucli-box-everything-test",
		ServiceName:     stewardServiceName("eucli-box-everything-test"),
		ResultFile:      resultFile,
	}
	payload, err := json.Marshal(install)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	steps := []string{}
	serviceExists := true
	stubStewardSystem(t, stewardSystem{
		queryService: func(context.Context, string) (stewardServiceStatus, error) {
			if !serviceExists {
				return stewardServiceStatus{}, nil
			}
			return stewardServiceStatus{Exists: true, Running: true}, nil
		},
		deleteService: func(context.Context, string) error {
			steps = append(steps, "delete")
			serviceExists = false
			return nil
		},
		installService: func(context.Context, string, string) error {
			steps = append(steps, "install")
			serviceExists = true
			return nil
		},
		startService: func(context.Context, string) error {
			steps = append(steps, "start")
			return nil
		},
	})
	if err := RunStewardInstall(configPath); err != nil {
		t.Fatal(err)
	}
	if strings.Join(steps, ",") != "delete,install,start" {
		t.Fatalf("steps = %v", steps)
	}
}

func TestRunStewardInstallReportsFailureResult(t *testing.T) {
	sourceEngine := filepath.Join(t.TempDir(), "Everything.exe")
	if err := os.WriteFile(sourceEngine, []byte("engine-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseDir := t.TempDir()
	protectedEngine := filepath.Join(baseDir, stewardProtectedDirectoryName, "Everything.exe")
	resultFile := filepath.Join(t.TempDir(), stewardInstallResultFileName)
	configPath := filepath.Join(t.TempDir(), stewardInstallConfigFileName)
	install := stewardInstallConfig{
		SourceEngine:    sourceEngine,
		TargetDirectory: filepath.Dir(protectedEngine),
		TargetEngine:    protectedEngine,
		InstanceName:    "eucli-box-everything-test",
		ServiceName:     stewardServiceName("eucli-box-everything-test"),
		ResultFile:      resultFile,
	}
	payload, err := json.Marshal(install)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	stubStewardSystem(t, stewardSystem{
		queryService: func(context.Context, string) (stewardServiceStatus, error) {
			return stewardServiceStatus{}, nil
		},
		installService: func(context.Context, string, string) error {
			return errors.New("engine install failed")
		},
	})
	err = RunStewardInstall(configPath)
	if err == nil || !strings.Contains(err.Error(), "engine install failed") {
		t.Fatalf("err = %v", err)
	}
	result := readTestInstallResult(t, resultFile)
	if result.Success || !strings.Contains(result.Error, "engine install failed") {
		t.Fatalf("result = %#v", result)
	}
}

func readTestInstallResult(t *testing.T, path string) stewardInstallResult {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result stewardInstallResult
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func stubStewardHealth(t *testing.T, state stewardState) {
	t.Helper()
	stubStewardHealthStatus(t, stewardStatus{
		State:               state,
		ServiceName:         stewardServiceName(fixtureConfig().Runtime.DefaultInstanceName),
		ProtectedEnginePath: filepath.Join("protected", "Everything.exe"),
	})
}

func stubStewardHealthStatus(t *testing.T, status stewardStatus) {
	t.Helper()
	previous := stewardHealthCheck
	stewardHealthCheck = func(context.Context, Config, string) (stewardStatus, error) {
		return status, nil
	}
	t.Cleanup(func() { stewardHealthCheck = previous })
}

func stubStewardSystem(t *testing.T, system stewardSystem) {
	t.Helper()
	previous := steward
	steward = system
	t.Cleanup(func() { steward = previous })
}
