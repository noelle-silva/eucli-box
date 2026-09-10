package everything

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

const (
	keepAliveStateFileName = "keepalive.json"
	keepAliveGuardLockName = "keepalive.guard.lock"
	keepAliveProbeInterval = time.Second
	// keepAliveStaleMargin is the extra lifetime granted to a guard lock
	// beyond its keep-alive budget before a newcomer may reclaim it.
	keepAliveStaleMargin = 60 * time.Second
)

type keepAliveSettings struct {
	Enabled bool
	Seconds int
}

type keepAliveState struct {
	ESExecutable     string    `json:"esExecutable"`
	InstanceName     string    `json:"instanceName"`
	KeepAliveSeconds int       `json:"keepAliveSeconds"`
	ConnectTimeoutMs int       `json:"connectTimeoutMs"`
	LastUsedAt       time.Time `json:"lastUsedAt"`
}

// retireBundledRuntime closes or keeps the bundled Everything runtime after
// one action that used it, following the configured keep-alive behavior.
//
// The retirement is an independent, short and bounded teardown: it never
// depends on the execution context, which may already be expired by a
// deadline or an active stop. It cannot extend the task deadline, cannot
// change the action outcome and never reruns the action.
func retireBundledRuntime(toolDataDirectory string, config Config, provider selectedProvider, request searchRequest, input types.ToolExecutionInput) error {
	if !provider.Bundled {
		return nil
	}
	if strings.TrimSpace(request.InstanceName) == "" {
		return nil
	}
	settings, err := loadKeepAliveSettings(input)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return stopBundledInstance(toolDataDirectory, config, provider.ESExecutable, request.InstanceName)
	}
	stateFile, guardLock, err := bundledInstanceLeasePaths(toolDataDirectory, config, request.InstanceName)
	if err != nil {
		return err
	}
	state := keepAliveState{
		ESExecutable:     provider.ESExecutable,
		InstanceName:     request.InstanceName,
		KeepAliveSeconds: settings.Seconds,
		ConnectTimeoutMs: config.Limits.DefaultConnectTimeoutMs,
		LastUsedAt:       time.Now().UTC(),
	}
	if err := writeKeepAliveState(stateFile, state); err != nil {
		return err
	}
	return spawnKeepAliveGuard(stateFile, guardLock)
}

// stopInstanceBounded stops one bundled instance inside a fresh, short-lived
// bound of its own, so an expired execution context can never prevent the
// teardown from stopping the instance it owns.
func stopInstanceBounded(esExecutable string, instanceName string, connectTimeoutMs int) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(connectTimeoutMs)*time.Millisecond)
	defer cancel()
	return stopInstance(ctx, esExecutable, instanceName, connectTimeoutMs)
}

// bundledInstanceLeasePaths resolves the keep-alive lease artifacts of one
// bundled instance: the state file that grants its reuse window and the guard
// lock that arbitrates the guards watching it.
func bundledInstanceLeasePaths(toolDataDirectory string, config Config, instanceName string) (string, string, error) {
	runtimeRootDir, err := bundledRuntimeDir(toolDataDirectory, config.Runtime.Directory)
	if err != nil {
		return "", "", err
	}
	instanceDir := bundledInstanceRuntimeDir(runtimeRootDir, instanceName)
	return filepath.Join(instanceDir, keepAliveStateFileName), filepath.Join(instanceDir, keepAliveGuardLockName), nil
}

// stopBundledInstance stops one bundled instance and clears its keep-alive
// lease, so no guard keeps watching an instance that no longer exists. The
// lease is cleared before the stop, so an instance that cannot be stopped is
// never reused again: the caller observes the stop failure and must not fall
// back to the old instance.
func stopBundledInstance(toolDataDirectory string, config Config, esExecutable string, instanceName string) error {
	stateFile, guardLock, err := bundledInstanceLeasePaths(toolDataDirectory, config, instanceName)
	if err != nil {
		return err
	}
	_ = os.Remove(stateFile)
	_ = os.Remove(guardLock)
	return stopInstanceBounded(esExecutable, instanceName, config.Limits.DefaultConnectTimeoutMs)
}

// keepAliveLeaseActive reports whether a keep-alive state still grants its
// instance a reuse window. An instance without an active lease is the leftover
// of an interrupted call and must never be reused.
func keepAliveLeaseActive(stateFile string, esExecutable string, instanceName string) bool {
	state, err := readKeepAliveState(stateFile)
	if err != nil {
		return false
	}
	if state.KeepAliveSeconds <= 0 {
		return false
	}
	if !sameExecutablePath(state.ESExecutable, esExecutable) || state.InstanceName != instanceName {
		return false
	}
	return !keepAliveExpired(state, time.Now())
}

// keepAliveExpired reports whether a keep-alive lease has outlived its budget.
func keepAliveExpired(state keepAliveState, now time.Time) bool {
	return !now.Before(state.LastUsedAt.Add(time.Duration(state.KeepAliveSeconds) * time.Second))
}

func loadKeepAliveSettings(input types.ToolExecutionInput) (keepAliveSettings, error) {
	enabled, err := mergedBool(input, "keepAliveEnabled", false)
	if err != nil {
		return keepAliveSettings{}, err
	}
	seconds, err := mergedInt(input, "keepAliveSeconds", defaultKeepAliveSeconds)
	if err != nil {
		return keepAliveSettings{}, err
	}
	if seconds <= 0 {
		return keepAliveSettings{}, fmt.Errorf("keepAliveSeconds must be greater than zero")
	}
	return keepAliveSettings{Enabled: enabled, Seconds: seconds}, nil
}

func writeKeepAliveState(path string, state keepAliveState) error {
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode keep-alive state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create keep-alive state directory: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write keep-alive state: %w", err)
	}
	return nil
}

func readKeepAliveState(path string) (keepAliveState, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return keepAliveState{}, err
	}
	var state keepAliveState
	if err := json.Unmarshal(payload, &state); err != nil {
		return keepAliveState{}, fmt.Errorf("decode keep-alive state: %w", err)
	}
	return state, nil
}

// spawnKeepAliveGuard starts a detached guard process for one bundled
// instance. The guard arbitrates via a lock file and coexists safely with
// any other guard already watching the same instance.
var spawnKeepAliveGuard = func(stateFile string, guardLock string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate guard executable: %w", err)
	}
	cmd := exec.Command(executable, "--keepalive-guard", stateFile, guardLock)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start keep-alive guard: %w", err)
	}
	return cmd.Process.Release()
}

// runKeepAliveGuard watches the state file of one bundled instance and stops
// the instance once it has been idle beyond the keep-alive budget.
// RunKeepAliveGuard is the public entry point exercised by the guard process
// started by the host. It never returns an error that is worth propagating:
// a dead guard must not make search calls fail.
func RunKeepAliveGuard(stateFile string, guardLock string) error {
	return runKeepAliveGuard(stateFile, guardLock)
}

func runKeepAliveGuard(stateFile string, guardLock string) error {
	state, err := readKeepAliveState(stateFile)
	if err != nil {
		return nil
	}
	lock, err := acquireGuardLock(guardLock, state.KeepAliveSeconds)
	if err != nil {
		return nil
	}
	defer lock.release()
	for {
		state, err := readKeepAliveState(stateFile)
		if err != nil {
			return nil
		}
		if keepAliveExpired(state, time.Now()) {
			_ = stopInstance(context.Background(), state.ESExecutable, state.InstanceName, state.ConnectTimeoutMs)
			_ = os.Remove(stateFile)
			return nil
		}
		lock.touch()
		time.Sleep(keepAliveProbeInterval)
	}
}

type guardLock struct {
	file *os.File
	path string
}

func acquireGuardLock(path string, keepAliveSeconds int) (*guardLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
	if err == nil {
		lock := &guardLock{file: file, path: path}
		lock.touch()
		return lock, nil
	}
	if !os.IsExist(err) {
		return nil, fmt.Errorf("create guard lock: %w", err)
	}
	stale := time.Duration(keepAliveSeconds)*time.Second + keepAliveStaleMargin
	if info, statErr := os.Stat(path); statErr == nil {
		if time.Since(info.ModTime()) <= stale {
			return nil, os.ErrExist
		}
		_ = os.Remove(path)
	}
	file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	lock := &guardLock{file: file, path: path}
	lock.touch()
	return lock, nil
}

func (l *guardLock) touch() {
	if l == nil || l.file == nil {
		return
	}
	now := time.Now()
	_ = os.Chtimes(l.path, now, now)
}

func (l *guardLock) release() {
	if l == nil {
		return
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	if l.path != "" {
		_ = os.Remove(l.path)
	}
}
