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
	"time"

	"eucli-box/pkg/types"
)

// stewardInstallConfig is the one-time handover between the regular-account
// authorize action and the elevated hidden installer.
type stewardInstallConfig struct {
	SourceEngine    string `json:"sourceEngine"`
	TargetDirectory string `json:"targetDirectory"`
	TargetEngine    string `json:"targetEngine"`
	InstanceName    string `json:"instanceName"`
	ServiceName     string `json:"serviceName"`
	ResultFile      string `json:"resultFile"`
}

// stewardInstallResult is the structured outcome the elevated hidden installer
// writes for the authorize action.
type stewardInstallResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// executeAuthorize implements the one-shot, idempotent authorize action: it
// reports an already authorized steward unchanged, or installs/repairs it
// through exactly one elevated hidden run.
func executeAuthorize(ctx context.Context, input types.ToolExecutionInput, config Config, request searchRequest) types.ToolExecutionOutput {
	metadata := authorizeMetadata(request)
	if runtime.GOOS != "windows" {
		return failure("execute everything authorize", errStewardUnsupported, metadata)
	}
	provider, err := resolveBundledProvider(config, input.ToolBodyDirectory)
	if err != nil {
		return failure("resolve bundled Everything provider", err, metadata)
	}
	sourceEngine := provider.RuntimeExecutable
	lock, err := acquireStewardAuthorizeLock(input.ToolDataDirectory)
	if err != nil {
		return failure("lock Everything authorization", err, metadata)
	}
	defer lock.Release()

	status, err := stewardHealthCheck(ctx, config, sourceEngine)
	if err != nil {
		return failure("check Everything permission steward", err, metadata)
	}
	if status.State == stewardStateHealthy {
		return stewardAuthorizeOutput(status, true, metadata)
	}
	install, configPath, err := newStewardInstallHandover(input.ToolDataDirectory, config, sourceEngine)
	if err != nil {
		return failure("prepare Everything steward install", err, metadata)
	}
	if err := writeStewardInstallConfig(configPath, install); err != nil {
		return failure("write Everything steward install config", err, metadata)
	}
	defer os.Remove(configPath)
	_ = os.Remove(install.ResultFile)
	if err := steward.elevateInstall(configPath); err != nil {
		if errors.Is(err, errStewardElevationDenied) {
			return failure("authorize Everything permission steward", err, metadata)
		}
		return failure("start elevated Everything steward install", err, metadata)
	}
	defer os.Remove(install.ResultFile)
	result, err := waitStewardInstallResult(ctx, install.ResultFile, config)
	if err != nil {
		return failure("wait for Everything steward install", err, metadata)
	}
	if !result.Success {
		return failure("install Everything permission steward", errors.New(result.Error), metadata)
	}
	status, err = stewardHealthCheck(ctx, config, sourceEngine)
	if err != nil {
		return failure("check Everything permission steward after install", err, metadata)
	}
	if status.State != stewardStateHealthy {
		return failure("verify Everything permission steward", fmt.Errorf("the steward is still %s after install", status.State), metadata)
	}
	if err := stopRunningFullDiskInstance(input.ToolDataDirectory, config, provider); err != nil {
		return failure("stop the previous full-disk Everything instance", err, metadata)
	}
	return stewardAuthorizeOutput(status, false, metadata)
}

// stopRunningFullDiskInstance retires the everyday full-disk instance after
// the steward was installed or repaired, so the next full-disk action connects
// to the new steward through a brand-new instance. A running instance is
// stopped together with its keep-alive lease; when it cannot be stopped the
// replacement fails explicitly and the old instance is never reused again.
func stopRunningFullDiskInstance(toolDataDirectory string, config Config, provider selectedProvider) error {
	instanceName := config.Runtime.DefaultInstanceName
	if !bundledRuntimeResponds(context.Background(), provider.ESExecutable, instanceName, config) {
		return nil
	}
	return stopBundledInstance(toolDataDirectory, config, provider.ESExecutable, instanceName)
}

func authorizeMetadata(request searchRequest) map[string]any {
	metadata := map[string]any{"action": string(actionAuthorize)}
	if strings.TrimSpace(request.Description) != "" {
		metadata["description"] = request.Description
	}
	return metadata
}

func stewardAuthorizeOutput(status stewardStatus, alreadyAuthorized bool, metadata map[string]any) types.ToolExecutionOutput {
	headline := "## Everything Permission Steward Authorized\n\nAuthorization completed; full-disk actions are ready.\n"
	if alreadyAuthorized {
		headline = "## Everything Permission Steward Already Authorized\n\nNo installation was needed; full-disk actions are ready.\n"
	}
	content := fmt.Sprintf("%s\nService: `%s`  \nSteward: `%s`\n", headline, status.ServiceName, status.ProtectedEnginePath)
	metadata["stewardState"] = string(status.State)
	metadata["serviceName"] = status.ServiceName
	metadata["protectedEnginePath"] = status.ProtectedEnginePath
	metadata["alreadyAuthorized"] = alreadyAuthorized
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: metadata}
}

// acquireStewardAuthorizeLock takes the exclusive authorization lock inside
// the tool data area. A lock held by a live process reports errStewardAuthorizeBusy
// immediately; a lock left behind by a crashed process is reclaimed.
func acquireStewardAuthorizeLock(toolDataDirectory string) (*runtimeLock, error) {
	directory := strings.TrimSpace(toolDataDirectory)
	if directory == "" {
		return nil, fmt.Errorf("toolDataDirectory is required")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("create Everything tool data directory: %w", err)
	}
	lockPath := filepath.Join(directory, stewardAuthorizeLockFileName)
	acquire := func() (*runtimeLock, error) {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		_, _ = fmt.Fprintf(file, "pid=%d\n", os.Getpid())
		return &runtimeLock{file: file, path: lockPath}, nil
	}
	lock, err := acquire()
	if err == nil {
		return lock, nil
	}
	if !os.IsExist(err) {
		return nil, fmt.Errorf("create Everything authorization lock: %w", err)
	}
	if err := removeStaleRuntimeLock(lockPath); err != nil {
		return nil, err
	}
	lock, err = acquire()
	if err == nil {
		return lock, nil
	}
	if os.IsExist(err) {
		return nil, errStewardAuthorizeBusy
	}
	return nil, fmt.Errorf("create Everything authorization lock: %w", err)
}

func newStewardInstallHandover(toolDataDirectory string, config Config, sourceEngine string) (stewardInstallConfig, string, error) {
	if strings.TrimSpace(toolDataDirectory) == "" {
		return stewardInstallConfig{}, "", fmt.Errorf("toolDataDirectory is required")
	}
	protectedEngine, err := stewardProtectedEnginePath(sourceEngine)
	if err != nil {
		return stewardInstallConfig{}, "", err
	}
	instanceName := config.Runtime.DefaultInstanceName
	install := stewardInstallConfig{
		SourceEngine:    sourceEngine,
		TargetDirectory: filepath.Dir(protectedEngine),
		TargetEngine:    protectedEngine,
		InstanceName:    instanceName,
		ServiceName:     stewardServiceName(instanceName),
		ResultFile:      filepath.Join(toolDataDirectory, stewardInstallResultFileName),
	}
	return install, filepath.Join(toolDataDirectory, stewardInstallConfigFileName), nil
}

func writeStewardInstallConfig(path string, install stewardInstallConfig) error {
	payload, err := json.MarshalIndent(install, "", "  ")
	if err != nil {
		return fmt.Errorf("encode steward install config: %w", err)
	}
	if err := writeFileAtomically(path, payload); err != nil {
		return fmt.Errorf("write steward install config: %w", err)
	}
	return nil
}

// waitStewardInstallResult waits for the hidden installer to publish its
// structured result, honoring the caller deadline and stop instructions. A
// missing result is an explicit failure that names the incomplete run.
func waitStewardInstallResult(ctx context.Context, resultFile string, config Config) (stewardInstallResult, error) {
	var result stewardInstallResult
	probe := func(context.Context) (bool, error) {
		payload, err := os.ReadFile(resultFile)
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("read steward install result: %w", err)
		}
		if err := json.Unmarshal(payload, &result); err != nil {
			return false, fmt.Errorf("decode steward install result: %w", err)
		}
		if !result.Success && strings.TrimSpace(result.Error) == "" {
			return false, fmt.Errorf("steward install result is missing a failure reason")
		}
		return true, nil
	}
	env := waitEnvironment{
		interval: time.Duration(config.Runtime.ProbeIntervalMs) * time.Millisecond,
		sleep:    sleepWithContext,
	}
	if err := waitUntil(ctx, env, probe); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return stewardInstallResult{}, fmt.Errorf("the elevated steward install did not report a result before the caller deadline")
		}
		if errors.Is(err, context.Canceled) {
			return stewardInstallResult{}, fmt.Errorf("the elevated steward install was stopped before it reported a result")
		}
		return stewardInstallResult{}, err
	}
	return result, nil
}
