package everything

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The permission steward is the system service that lets the everyday
// full-disk Everything instance read raw volume data. It is installed once by
// an elevated authorization into the protected program installation area,
// where the steward engine copy lives. Scoped folder instances never use it.
const (
	stewardProtectedDirectoryName = "eucli-box-everything"
	stewardServiceNameFormat      = "Everything (%s)"
	stewardInstallFlag            = "--steward-install"
	stewardInstallConfigFileName  = "steward-install.json"
	stewardInstallResultFileName  = "steward-result.json"
	stewardAuthorizeLockFileName  = "steward-authorize.lock"
	stewardServiceProbeInterval   = 250 * time.Millisecond
	stewardServiceStateTimeout    = 30 * time.Second
)

var (
	// errStewardElevationDenied reports that the user refused the system
	// elevation prompt, so no installation was started.
	errStewardElevationDenied = errors.New("authorization was denied by the user")
	// errStewardAuthorizeBusy reports that another authorization already holds
	// the exclusive authorization lock.
	errStewardAuthorizeBusy = errors.New("authorization is already in progress")
	// errStewardUnsupported reports use of the steward on a platform without it.
	errStewardUnsupported = errors.New("the permission steward is supported only on Windows")
)

// stewardState is the observed health of the permission steward.
type stewardState string

const (
	stewardStateNotInstalled    stewardState = "notInstalled"
	stewardStatePathMismatch    stewardState = "pathMismatch"
	stewardStateVersionMismatch stewardState = "versionMismatch"
	stewardStateStopped         stewardState = "stopped"
	stewardStateHealthy         stewardState = "healthy"
)

// stewardServiceStatus is one service query result. BinaryPath is the raw
// registered binary path, which may be quoted and may carry engine arguments.
type stewardServiceStatus struct {
	Exists     bool
	BinaryPath string
	Running    bool
}

// stewardSystem carries every external side effect of the steward mechanism.
// Tests replace it with deterministic substitutes and never touch the real
// system service database or the real elevation mechanism.
type stewardSystem struct {
	protectedBaseDir func() (string, error)
	queryService     func(ctx context.Context, serviceName string) (stewardServiceStatus, error)
	deleteService    func(ctx context.Context, serviceName string) error
	startService     func(ctx context.Context, serviceName string) error
	installService   func(ctx context.Context, enginePath string, instanceName string) error
	elevateInstall   func(configPath string) error
}

var steward = productionStewardSystem()

// stewardStatus is the evaluated steward health plus the facts that explain it.
type stewardStatus struct {
	State               stewardState
	ServiceName         string
	ProtectedEnginePath string
}

// stewardHealthCheck is the injectable health gate every full-disk action
// consults before it prepares or starts the everyday instance.
var stewardHealthCheck = inspectStewardHealth

func stewardServiceName(instanceName string) string {
	return fmt.Sprintf(stewardServiceNameFormat, instanceName)
}

// stewardProtectedEnginePath derives the protected engine copy location from
// the system-provided protected base directory and the engine file name of the
// tool package copy.
func stewardProtectedEnginePath(sourceEngine string) (string, error) {
	baseDir, err := steward.protectedBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, stewardProtectedDirectoryName, filepath.Base(sourceEngine)), nil
}

// inspectStewardHealth derives the steward health from the system service
// registration and the engine copies. It never requires administrator rights.
func inspectStewardHealth(ctx context.Context, config Config, sourceEngine string) (stewardStatus, error) {
	protectedEngine, err := stewardProtectedEnginePath(sourceEngine)
	if err != nil {
		return stewardStatus{}, err
	}
	serviceName := stewardServiceName(config.Runtime.DefaultInstanceName)
	service, err := steward.queryService(ctx, serviceName)
	if err != nil {
		return stewardStatus{}, err
	}
	return stewardStatus{
		State:               evaluateStewardState(service, protectedEngine, sourceEngine),
		ServiceName:         serviceName,
		ProtectedEnginePath: protectedEngine,
	}, nil
}

// evaluateStewardState orders the health facts: an absent service first, then
// a wrong engine registration, then an engine copy that does not match the
// tool package, and finally a stopped service. Only all facts together are
// healthy.
func evaluateStewardState(service stewardServiceStatus, protectedEngine string, sourceEngine string) stewardState {
	if !service.Exists {
		return stewardStateNotInstalled
	}
	if !sameExecutablePath(serviceExecutablePath(service.BinaryPath), protectedEngine) {
		return stewardStatePathMismatch
	}
	if !filesHaveSameContent(protectedEngine, sourceEngine) {
		return stewardStateVersionMismatch
	}
	if !service.Running {
		return stewardStateStopped
	}
	return stewardStateHealthy
}

// requireHealthySteward returns the observed steward status when it is healthy
// and an explicit authorization guidance otherwise. The observed facts are
// recorded on metadata before the guidance is returned.
func requireHealthySteward(ctx context.Context, config Config, sourceEngine string, metadata map[string]any) (stewardStatus, error) {
	status, err := stewardHealthCheck(ctx, config, sourceEngine)
	if err != nil {
		return stewardStatus{}, err
	}
	if status.State != stewardStateHealthy {
		metadata["stewardState"] = string(status.State)
		metadata["serviceName"] = status.ServiceName
		metadata["stewardEngine"] = status.ProtectedEnginePath
		return status, stewardGuidance(status)
	}
	return status, nil
}

func stewardGuidance(status stewardStatus) error {
	return fmt.Errorf("full-disk Everything actions require a healthy permission steward (state: %s); call the %q action first and have the user confirm the system elevation prompt at the computer", status.State, actionAuthorize)
}

// serviceExecutablePath extracts the executable from a registered service
// binary path, which may be quoted and may carry engine arguments.
func serviceExecutablePath(commandLine string) string {
	trimmed := strings.TrimSpace(commandLine)
	if strings.HasPrefix(trimmed, `"`) {
		if end := strings.Index(trimmed[1:], `"`); end >= 0 {
			return strings.TrimSpace(trimmed[1 : end+1])
		}
		return strings.TrimSpace(strings.Trim(trimmed, `"`))
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func sameExecutablePath(left string, right string) bool {
	if left == "" || right == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func filesHaveSameContent(left string, right string) bool {
	leftHash, err := fileSHA256(left)
	if err != nil {
		return false
	}
	rightHash, err := fileSHA256(right)
	if err != nil {
		return false
	}
	return leftHash == rightHash
}

// writeFileAtomically publishes a file through a temporary sibling so readers
// never observe a partially written payload.
func writeFileAtomically(path string, payload []byte) error {
	temp := path + ".tmp"
	if err := os.WriteFile(temp, payload, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}
