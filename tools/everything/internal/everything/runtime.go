package everything

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const defaultKeepAliveSeconds = 300

type runtimeLock struct {
	file *os.File
	path string
}

const scopedInstanceSuffix = "-scoped"

// acquireBundledRuntimeLock waits until the runtime lock is free and takes it.
// The wait ends only when the lock is acquired, an explicit hard file error
// appears, or the caller-specified deadline expires (reported as busy).
func acquireBundledRuntimeLock(ctx context.Context, toolDataDirectory string, config Config) (*runtimeLock, error) {
	runtimeDir, err := bundledRuntimeDir(toolDataDirectory, config.Runtime.Directory)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return nil, fmt.Errorf("create Everything runtime directory: %w", err)
	}
	lockPath := filepath.Join(runtimeDir, "everything.lock")
	var acquired *runtimeLock
	probe := func(context.Context) (bool, error) {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(file, "pid=%d\n", os.Getpid())
			acquired = &runtimeLock{file: file, path: lockPath}
			return true, nil
		}
		if !os.IsExist(err) {
			return false, fmt.Errorf("create Everything runtime lock: %w", err)
		}
		if err := removeStaleRuntimeLock(lockPath); err != nil {
			return false, err
		}
		return false, nil
	}
	env := waitEnvironment{
		interval: time.Duration(config.Runtime.ProbeIntervalMs) * time.Millisecond,
		sleep:    sleepWithContext,
	}
	if err := waitUntil(ctx, env, probe); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("Everything runtime is busy")
		}
		return nil, err
	}
	return acquired, nil
}

// removeStaleRuntimeLock removes a runtime lock left behind by a crashed
// owner. A lock whose recorded owner process is still alive is never treated
// as stale.
func removeStaleRuntimeLock(lockPath string) error {
	info, err := os.Stat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat Everything runtime lock: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("Everything runtime lock path is a directory")
	}
	pid, ok := lockOwnerPID(lockPath)
	if !ok || processAlive(pid) {
		return nil
	}
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale Everything runtime lock: %w", err)
	}
	return nil
}

// lockOwnerPID reads the owner pid recorded in a runtime lock file.
func lockOwnerPID(lockPath string) (int, bool) {
	payload, err := os.ReadFile(lockPath)
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(payload)), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(key) == "pid" {
			pid, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || pid <= 0 {
				return 0, false
			}
			return pid, true
		}
	}
	return 0, false
}

// processAlive reports whether a process with the given pid exists. When the
// check itself fails the process is conservatively treated as alive.
func processAlive(pid int) bool {
	if runtime.GOOS == "windows" {
		output, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid)).CombinedOutput()
		if err != nil {
			return true
		}
		for _, line := range strings.Split(string(output), "\r\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[1] == strconv.Itoa(pid) {
				return true
			}
		}
		return false
	}
	err := exec.Command("kill", "-0", strconv.Itoa(pid)).Run()
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false
	}
	return true
}

func (l *runtimeLock) Release() {
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

func ensureBundledRuntime(ctx context.Context, toolDataDirectory string, config Config, provider selectedProvider, request searchRequest) (searchRequest, error) {
	if strings.TrimSpace(request.InstanceName) == "" {
		request.InstanceName = defaultBundledInstanceName(config, request)
	}
	runtimeRootDir, err := bundledRuntimeDir(toolDataDirectory, config.Runtime.Directory)
	if err != nil {
		return searchRequest{}, err
	}
	runtimeExecutable, err := syncBundledRuntimeExecutable(provider.RuntimeExecutable, runtimeRootDir)
	if err != nil {
		return searchRequest{}, err
	}
	runtimeDir := bundledInstanceRuntimeDir(runtimeRootDir, request.InstanceName)
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return searchRequest{}, fmt.Errorf("create Everything runtime directory: %w", err)
	}
	runtimeConfig := filepath.Join(runtimeDir, "Everything.ini")
	databasePath := filepath.Join(runtimeDir, "Everything.db")
	desiredConfig := runtimeConfigContent(runtimeDir, indexedFolders(request))
	stateFile := filepath.Join(runtimeDir, keepAliveStateFileName)
	instanceRunning := bundledRuntimeResponds(ctx, provider.ESExecutable, request.InstanceName, config)
	if !runtimeConfigMatches(runtimeConfig, desiredConfig) || !keepAliveLeaseActive(stateFile, provider.ESExecutable, request.InstanceName) || !instanceRunning {
		if instanceRunning {
			if err := stopBundledInstance(toolDataDirectory, config, provider.ESExecutable, request.InstanceName); err != nil {
				return searchRequest{}, fmt.Errorf("replace the running Everything instance: %w", err)
			}
		}
		if err := writeRuntimeConfig(runtimeConfig, desiredConfig); err != nil {
			return searchRequest{}, err
		}
		if err := startBundledEverything(runtimeExecutable, request.InstanceName, runtimeConfig, databasePath); err != nil {
			return searchRequest{}, err
		}
		if err := waitEverythingReady(ctx, provider.ESExecutable, request.InstanceName, config, true); err != nil {
			return searchRequest{}, err
		}
	}
	if err := waitIndexedFoldersReady(ctx, provider.ESExecutable, request, config); err != nil {
		return searchRequest{}, err
	}
	if err := persistBundledDatabaseIfMissing(ctx, provider.ESExecutable, databasePath, request); err != nil {
		return searchRequest{}, err
	}
	return request, nil
}

func defaultBundledInstanceName(config Config, request searchRequest) string {
	name := strings.TrimSpace(config.Runtime.DefaultInstanceName)
	if request.ScopeMode == scopeModeDirectory {
		return name + scopedInstanceSuffix
	}
	return name
}

func bundledRuntimeDir(toolDataDirectory string, runtimeDirectory string) (string, error) {
	toolDataDirectory = strings.TrimSpace(toolDataDirectory)
	if toolDataDirectory == "" {
		return "", fmt.Errorf("toolDataDirectory is required")
	}
	path := strings.TrimSpace(runtimeDirectory)
	if path == "" {
		return "", fmt.Errorf("runtime.directory is required")
	}
	cleaned := filepath.Clean(filepath.FromSlash(path))
	if filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" {
		return "", fmt.Errorf("runtime.directory must be relative")
	}
	resolved := filepath.Clean(filepath.Join(toolDataDirectory, cleaned))
	if !pathWithin(toolDataDirectory, resolved) {
		return "", fmt.Errorf("runtime.directory escapes tool data directory")
	}
	return resolved, nil
}

func syncBundledRuntimeExecutable(source string, runtimeRootDir string) (string, error) {
	target := filepath.Join(runtimeRootDir, "bin", filepath.Base(source))
	if err := syncRuntimeBinary(source, target); err != nil {
		return "", fmt.Errorf("sync bundled Everything runtime: %w", err)
	}
	return target, nil
}

func syncRuntimeBinary(source string, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	sourceHash, err := fileSHA256(source)
	if err != nil {
		return err
	}
	if targetHash, err := fileSHA256(target); err == nil && sourceHash == targetHash {
		return nil
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func fileSHA256(path string) ([32]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return [32]byte{}, err
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result, nil
}

func bundledInstanceRuntimeDir(runtimeRootDir string, instanceName string) string {
	return filepath.Join(runtimeRootDir, instanceRuntimeName(instanceName))
}

func instanceRuntimeName(instanceName string) string {
	name := strings.TrimSpace(instanceName)
	if name == "" {
		return "default"
	}
	var builder strings.Builder
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			builder.WriteRune(char)
			continue
		}
		builder.WriteRune('_')
	}
	value := strings.Trim(builder.String(), "._-")
	if value == "" {
		return "default"
	}
	return value
}

func runtimeConfigContent(runtimeDir string, folders []string) string {
	lines := []string{
		"[Everything]",
		"app_data=0",
		"run_as_admin=0",
		"run_in_background=1",
		"show_tray_icon=0",
		"check_for_updates_on_startup=0",
		"allow_multiple_windows=0",
		"allow_http_server=0",
		"db_location=" + iniValue(runtimeDir),
	}
	if len(folders) > 0 {
		lines = append(lines,
			"folders="+iniList(folders),
			"folder_monitor_changes="+repeatList("1", len(folders)),
			"folder_update_types="+repeatList("0", len(folders)),
		)
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

func writeRuntimeConfig(target string, content string) error {
	return os.WriteFile(target, []byte(content), 0o644)
}

func runtimeConfigMatches(path string, expected string) bool {
	payload, err := os.ReadFile(path)
	return err == nil && string(payload) == expected
}

func indexedFolders(request searchRequest) []string {
	return request.ScopeIndexPaths
}

func readyProbePaths(request searchRequest) []string {
	if request.ScopeMode == scopeModeAllLocalDrives {
		return request.ScopePaths
	}
	return indexedFolders(request)
}

func startBundledEverything(executable string, instanceName string, configPath string, databasePath string) error {
	cmd := exec.Command(executable,
		"-instance", instanceName,
		"-config", configPath,
		"-db", databasePath,
		"-startup",
	)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start bundled Everything failed: %w", err)
	}
	return cmd.Process.Release()
}

// stopInstance asks the bundled Everything instance to exit gracefully
// through the CLI, waiting at most connectTimeoutMs for the exit to start.
func stopInstance(ctx context.Context, esExecutable string, instanceName string, connectTimeoutMs int) error {
	timeout := time.Duration(connectTimeoutMs) * time.Millisecond
	_, err := runCommandOutput(ctx, timeout, esExecutable, everythingExitArgs(instanceName, connectTimeoutMs)...)
	return err
}

func persistBundledDatabaseIfMissing(ctx context.Context, executable string, databasePath string, request searchRequest) error {
	if _, err := os.Stat(databasePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat Everything database: %w", err)
	}
	if _, err := runCommandOutput(ctx, 0, executable, everythingSaveDatabaseArgs(request.InstanceName, request.ConnectTimeoutMs)...); err != nil {
		return fmt.Errorf("save Everything database: %w", err)
	}
	if _, err := os.Stat(databasePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("Everything database was not saved: %s", databasePath)
		}
		return fmt.Errorf("stat saved Everything database: %w", err)
	}
	return nil
}

func bundledRuntimeResponds(ctx context.Context, executable string, instanceName string, config Config) bool {
	return waitEverythingReady(ctx, executable, instanceName, config, false) == nil
}

func waitEverythingReady(ctx context.Context, executable string, instanceName string, config Config, requireWait bool) error {
	deadline := time.Now().Add(time.Duration(config.Runtime.ReadyTimeoutMs) * time.Millisecond)
	probeInterval := time.Duration(config.Runtime.ProbeIntervalMs) * time.Millisecond
	var lastErr error
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		probeTimeout := minDuration(time.Duration(config.Limits.DefaultConnectTimeoutMs)*time.Millisecond, remaining)
		_, err := runCommandOutput(ctx, probeTimeout, executable, everythingVersionArgs(instanceName, int(probeTimeout/time.Millisecond))...)
		if err == nil {
			return nil
		}
		lastErr = err
		if !requireWait {
			break
		}
		if sleep := minDuration(probeInterval, time.Until(deadline)); sleep > 0 {
			time.Sleep(sleep)
		}
	}
	if lastErr != nil {
		return fmt.Errorf("bundled Everything runtime is not ready: %w", lastErr)
	}
	return fmt.Errorf("bundled Everything runtime is not ready")
}

// folderIndexWatcher observes whether every probed folder has a ready index.
// It keeps the latest transient probe failure so a caller-deadline failure can
// report why the index never became ready.
type folderIndexWatcher struct {
	executable  string
	request     searchRequest
	folders     []string
	lastFailure error
}

// observe implements waitProbe for the index-readiness wait. A folder is ready
// when it has visible entries or is actually empty; a hard probe failure ends
// the wait, while a transient probe failure only keeps it alive.
func (w *folderIndexWatcher) observe(ctx context.Context) (bool, error) {
	probeTimeout := time.Duration(w.request.ConnectTimeoutMs) * time.Millisecond
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false, nil
		}
		if remaining < probeTimeout {
			probeTimeout = remaining
		}
	}
	allReady := true
	for _, folder := range w.folders {
		output, err := runCommandOutput(ctx, probeTimeout, w.executable, everythingCountArgs(w.request.InstanceName, int(probeTimeout/time.Millisecond), folder)...)
		if err != nil {
			if hardProbeFailure(err) {
				return false, fmt.Errorf("Everything folder index probe failed for %s: %w", folder, err)
			}
			w.lastFailure = err
			allReady = false
			continue
		}
		ready, err := folderIndexReady(folder, output)
		if err != nil {
			w.lastFailure = err
			allReady = false
			continue
		}
		if !ready {
			allReady = false
		}
	}
	return allReady, nil
}

// waitIndexedFoldersReady waits until every probed folder reports indexed
// entries (or is actually empty). The wait ends only when the index is ready,
// an explicit hard probe failure appears, or the caller-specified deadline
// expires. It never gives up because visible counts stopped growing.
func waitIndexedFoldersReady(ctx context.Context, executable string, request searchRequest, config Config) error {
	folders := readyProbePaths(request)
	if len(folders) == 0 {
		return nil
	}
	watcher := &folderIndexWatcher{executable: executable, request: request, folders: folders}
	env := waitEnvironment{
		interval: time.Duration(config.Runtime.ProbeIntervalMs) * time.Millisecond,
		sleep:    sleepWithContext,
	}
	if err := waitUntil(ctx, env, watcher.observe); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			if watcher.lastFailure != nil {
				return fmt.Errorf("Everything folder index is not ready before the caller deadline: %w", watcher.lastFailure)
			}
			return fmt.Errorf("Everything folder index is not ready before the caller deadline")
		}
		return err
	}
	return nil
}

// exitCodeCarrier is implemented by errors that carry a process exit code.
type exitCodeCarrier interface {
	ExitCode() int
}

// hardProbeFailure reports whether an index probe failure is an explicit
// failure fact that waiting cannot recover from: a missing or unusable
// executable, a lost Everything instance, or a probe process that exited with
// an error other than a transient busy condition. Everything CLI exit code 7
// (unable to send IPC message) and 9 (no results found) describe a busy
// instance or an empty result set rather than a broken probe. Timeouts and
// unclassifiable failures also keep the wait alive.
func hardProbeFailure(err error) bool {
	if err == nil {
		return false
	}
	var carrier exitCodeCarrier
	if errors.As(err, &carrier) {
		return !transientProbeExitCode(carrier.ExitCode())
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return true
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return true
	}
	return false
}

// transientProbeExitCode reports whether an Everything CLI exit code describes
// a condition the index wait can recover from by waiting.
func transientProbeExitCode(code int) bool {
	return code == 7 || code == 9
}

func folderIndexReady(scopePath string, countOutput string) (bool, error) {
	count, err := parseResultCount(countOutput)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	return directoryIsEmpty(scopePath), nil
}

func parseResultCount(text string) (int, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, fmt.Errorf("Everything result count output is empty")
	}
	count, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("Everything result count output is invalid: %s", trimmed)
	}
	return count, nil
}

func directoryIsEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) == 0
}

func everythingVersionArgs(instanceName string, timeoutMs int) []string {
	args := []string{}
	if strings.TrimSpace(instanceName) != "" {
		args = append(args, "-instance", strings.TrimSpace(instanceName))
	}
	return append(args, "-timeout", strconv.Itoa(timeoutMs), "-get-everything-version")
}

func everythingExitArgs(instanceName string, timeoutMs int) []string {
	args := []string{}
	if strings.TrimSpace(instanceName) != "" {
		args = append(args, "-instance", strings.TrimSpace(instanceName))
	}
	return append(args, "-timeout", strconv.Itoa(timeoutMs), "-exit")
}

func everythingSaveDatabaseArgs(instanceName string, timeoutMs int) []string {
	args := []string{}
	if strings.TrimSpace(instanceName) != "" {
		args = append(args, "-instance", strings.TrimSpace(instanceName))
	}
	return append(args, "-timeout", strconv.Itoa(timeoutMs), "-save-db")
}

func everythingCountArgs(instanceName string, timeoutMs int, scopePath string) []string {
	args := []string{}
	if strings.TrimSpace(instanceName) != "" {
		args = append(args, "-instance", strings.TrimSpace(instanceName))
	}
	return append(args, "-timeout", strconv.Itoa(timeoutMs), "-get-result-count", scopePath)
}

func minDuration(left time.Duration, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}

func iniValue(value string) string {
	return strings.ReplaceAll(value, "\\", "\\\\")
}

func iniList(values []string) string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, iniQuotedValue(value))
	}
	return strings.Join(items, ",")
}

func iniQuotedValue(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return "\"" + escaped + "\""
}

func repeatList(value string, count int) string {
	items := make([]string, 0, count)
	for index := 0; index < count; index++ {
		items = append(items, value)
	}
	return strings.Join(items, ",")
}
