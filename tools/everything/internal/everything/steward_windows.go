//go:build windows

package everything

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	stewardServiceCommandTimeout    = 30 * time.Second
	stewardShellExecuteAccessDenied = 5
)

func productionStewardSystem() stewardSystem {
	return stewardSystem{
		protectedBaseDir: platformStewardBaseDir,
		queryService:     platformQueryStewardService,
		deleteService:    platformDeleteStewardService,
		startService:     platformStartStewardService,
		installService:   platformInstallStewardService,
		elevateInstall:   platformElevateStewardInstall,
	}
}

// platformStewardBaseDir returns the system-protected program installation
// area. The location is provided by the system at run time; there is no
// fallback and a missing value is an explicit failure.
func platformStewardBaseDir() (string, error) {
	baseDir := strings.TrimSpace(os.Getenv("ProgramFiles"))
	if baseDir == "" {
		return "", fmt.Errorf("the system did not provide the protected program installation directory")
	}
	if !filepath.IsAbs(baseDir) {
		return "", fmt.Errorf("the protected program installation directory %q is not absolute", baseDir)
	}
	return filepath.Clean(baseDir), nil
}

func platformQueryStewardService(ctx context.Context, serviceName string) (stewardServiceStatus, error) {
	queryOutput, err := runStewardCommand(ctx, "sc.exe", "query", serviceName)
	if err != nil {
		if scExitCode(err) == 1060 {
			return stewardServiceStatus{}, nil
		}
		return stewardServiceStatus{}, err
	}
	configOutput, err := runStewardCommand(ctx, "sc.exe", "qc", serviceName)
	if err != nil {
		return stewardServiceStatus{}, err
	}
	return stewardServiceStatus{
		Exists:     true,
		BinaryPath: scBinaryPath(configOutput),
		Running:    scServiceRunning(queryOutput),
	}, nil
}

func platformDeleteStewardService(ctx context.Context, serviceName string) error {
	if _, err := runStewardCommand(ctx, "sc.exe", "stop", serviceName); err != nil {
		if code := scExitCode(err); code != 1060 && code != 1062 {
			return err
		}
	}
	if _, err := runStewardCommand(ctx, "sc.exe", "delete", serviceName); err != nil {
		if scExitCode(err) == 1060 {
			return nil
		}
		return err
	}
	return nil
}

func platformStartStewardService(ctx context.Context, serviceName string) error {
	if _, err := runStewardCommand(ctx, "sc.exe", "start", serviceName); err != nil {
		if scExitCode(err) == 1056 {
			return nil
		}
		return err
	}
	return nil
}

func platformInstallStewardService(ctx context.Context, enginePath string, instanceName string) error {
	_, err := runStewardCommand(ctx, enginePath, "-instance", instanceName, "-install-service")
	return err
}

// runStewardCommand runs one Windows service command with a hard bound, so a
// stuck service manager can never hang the tool.
func runStewardCommand(ctx context.Context, name string, args ...string) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, stewardServiceCommandTimeout)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, name, args...).CombinedOutput()
	text := strings.TrimSpace(string(output))
	label := strings.Join(append([]string{name}, args...), " ")
	if commandCtx.Err() != nil {
		return text, fmt.Errorf("%s timed out: %w", label, commandCtx.Err())
	}
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("%s failed: %w: %s", label, err, text)
		}
		return text, fmt.Errorf("%s failed: %w", label, err)
	}
	return text, nil
}

func scExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 0
}

func scServiceRunning(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "STATE") && strings.Contains(line, "RUNNING") {
			return true
		}
	}
	return false
}

func scBinaryPath(output string) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "BINARY_PATH_NAME") {
			if _, value, ok := strings.Cut(trimmed, ":"); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

var shellExecuteW = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")

// platformElevateStewardInstall starts the tool binary in its hidden steward
// install mode through the system runas elevation prompt. A refused prompt is
// reported as errStewardElevationDenied.
func platformElevateStewardInstall(configPath string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate the tool executable: %w", err)
	}
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := syscall.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	parameters, err := syscall.UTF16PtrFromString(fmt.Sprintf(`%s "%s"`, stewardInstallFlag, configPath))
	if err != nil {
		return err
	}
	directory, err := syscall.UTF16PtrFromString(filepath.Dir(executable))
	if err != nil {
		return err
	}
	code, _, _ := shellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(parameters)),
		uintptr(unsafe.Pointer(directory)),
		syscall.SW_HIDE,
	)
	if code <= 32 {
		if code == stewardShellExecuteAccessDenied {
			return errStewardElevationDenied
		}
		return fmt.Errorf("start elevated steward install failed with ShellExecute code %d", code)
	}
	return nil
}
