package everything

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// RunStewardInstall is the hidden, administrator-context entry point that
// performs the steward installation and reports the structured outcome through
// the one-time result file. The authorize action launches it through the
// system elevation mechanism.
func RunStewardInstall(configPath string) error {
	install, err := readStewardInstallConfig(configPath)
	if err != nil {
		return err
	}
	result := performStewardInstall(context.Background(), install)
	if err := writeStewardInstallResult(install.ResultFile, result); err != nil {
		return err
	}
	if !result.Success {
		return errors.New(result.Error)
	}
	return nil
}

// performStewardInstall replaces any existing service registration, publishes
// the engine copy into the protected area, installs the service with the same
// instance name, starts it and waits until it runs.
func performStewardInstall(ctx context.Context, install stewardInstallConfig) stewardInstallResult {
	status, err := steward.queryService(ctx, install.ServiceName)
	if err != nil {
		return stewardInstallFailure(fmt.Errorf("query service %q: %w", install.ServiceName, err))
	}
	if status.Exists {
		if err := steward.deleteService(ctx, install.ServiceName); err != nil {
			return stewardInstallFailure(fmt.Errorf("delete service %q: %w", install.ServiceName, err))
		}
		if err := waitStewardServiceState(ctx, install.ServiceName, false); err != nil {
			return stewardInstallFailure(err)
		}
	}
	if err := os.MkdirAll(install.TargetDirectory, 0o755); err != nil {
		return stewardInstallFailure(fmt.Errorf("create protected steward directory: %w", err))
	}
	if err := syncRuntimeBinary(install.SourceEngine, install.TargetEngine); err != nil {
		return stewardInstallFailure(fmt.Errorf("publish steward engine: %w", err))
	}
	if err := steward.installService(ctx, install.TargetEngine, install.InstanceName); err != nil {
		return stewardInstallFailure(fmt.Errorf("install service %q: %w", install.ServiceName, err))
	}
	if err := steward.startService(ctx, install.ServiceName); err != nil {
		return stewardInstallFailure(fmt.Errorf("start service %q: %w", install.ServiceName, err))
	}
	if err := waitStewardServiceState(ctx, install.ServiceName, true); err != nil {
		return stewardInstallFailure(err)
	}
	return stewardInstallResult{Success: true}
}

// waitStewardServiceState waits until the service matches the wanted running
// state; a deleted service counts as stopped. Probe failures and the timeout
// end the wait explicitly instead of being retried forever.
func waitStewardServiceState(ctx context.Context, serviceName string, wantRunning bool) error {
	waitCtx, cancel := context.WithTimeout(ctx, stewardServiceStateTimeout)
	defer cancel()
	probe := func(probeCtx context.Context) (bool, error) {
		status, err := steward.queryService(probeCtx, serviceName)
		if err != nil {
			return false, fmt.Errorf("query service %q: %w", serviceName, err)
		}
		if wantRunning {
			return status.Exists && status.Running, nil
		}
		return !status.Exists || !status.Running, nil
	}
	env := waitEnvironment{interval: stewardServiceProbeInterval, sleep: sleepWithContext}
	if err := waitUntil(waitCtx, env, probe); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("service %q did not become %s within %s", serviceName, stewardServiceStateWord(wantRunning), stewardServiceStateTimeout)
		}
		return err
	}
	return nil
}

func stewardServiceStateWord(running bool) string {
	if running {
		return "running"
	}
	return "stopped"
}

func readStewardInstallConfig(path string) (stewardInstallConfig, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return stewardInstallConfig{}, fmt.Errorf("read steward install config: %w", err)
	}
	var install stewardInstallConfig
	if err := json.Unmarshal(payload, &install); err != nil {
		return stewardInstallConfig{}, fmt.Errorf("decode steward install config: %w", err)
	}
	if err := validateStewardInstallConfig(install); err != nil {
		return stewardInstallConfig{}, err
	}
	return install, nil
}

func validateStewardInstallConfig(install stewardInstallConfig) error {
	required := []struct {
		name  string
		value string
	}{
		{"sourceEngine", install.SourceEngine},
		{"targetDirectory", install.TargetDirectory},
		{"targetEngine", install.TargetEngine},
		{"instanceName", install.InstanceName},
		{"serviceName", install.ServiceName},
		{"resultFile", install.ResultFile},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("steward install config %s is required", field.name)
		}
	}
	return nil
}

func writeStewardInstallResult(path string, result stewardInstallResult) error {
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode steward install result: %w", err)
	}
	if err := writeFileAtomically(path, payload); err != nil {
		return fmt.Errorf("write steward install result: %w", err)
	}
	return nil
}

func stewardInstallFailure(err error) stewardInstallResult {
	return stewardInstallResult{Success: false, Error: err.Error()}
}
