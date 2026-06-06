package everything

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type runtimeLock struct {
	file *os.File
	path string
}

const scopedInstanceSuffix = "-scoped"

func acquireBundledRuntimeLock(ctx context.Context, toolDirectory string, config Config, request searchRequest) (*runtimeLock, error) {
	runtimeDir, err := bundledRuntimeDir(toolDirectory, config.Runtime.Directory)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return nil, fmt.Errorf("create Everything runtime directory: %w", err)
	}
	lockPath := filepath.Join(runtimeDir, "everything.lock")
	deadline := time.Now().Add(time.Duration(request.TimeoutMs) * time.Millisecond)
	for {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(file, "pid=%d\n", os.Getpid())
			return &runtimeLock{file: file, path: lockPath}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("create Everything runtime lock: %w", err)
		}
		if err := removeStaleRuntimeLock(lockPath, config); err != nil {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("Everything runtime is busy")
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if sleep := minDuration(time.Duration(config.Runtime.ProbeIntervalMs)*time.Millisecond, time.Until(deadline)); sleep > 0 {
			time.Sleep(sleep)
		}
	}
}

func removeStaleRuntimeLock(lockPath string, config Config) error {
	info, err := os.Stat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat Everything runtime lock: %w", err)
	}
	maxAge := time.Duration(config.Limits.MaxTimeoutMs+config.Runtime.ReadyTimeoutMs) * time.Millisecond
	if maxAge <= 0 || time.Since(info.ModTime()) <= maxAge {
		return nil
	}
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale Everything runtime lock: %w", err)
	}
	return nil
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

func ensureBundledRuntime(ctx context.Context, toolDirectory string, config Config, provider selectedProvider, request searchRequest) (searchRequest, error) {
	if strings.TrimSpace(request.InstanceName) == "" {
		request.InstanceName = defaultBundledInstanceName(config, request)
	}
	runtimeRootDir, err := bundledRuntimeDir(toolDirectory, config.Runtime.Directory)
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
	if requiresBundledWindowsService(request) {
		if err := ensureBundledWindowsService(ctx, runtimeExecutable, request.InstanceName, config); err != nil {
			return searchRequest{}, err
		}
	}
	if !runtimeConfigMatches(runtimeConfig, desiredConfig) || !bundledRuntimeResponds(ctx, provider.ESExecutable, request.InstanceName, config) {
		_ = stopBundledEverything(ctx, provider.ESExecutable, request.InstanceName, config)
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

func bundledRuntimeDir(toolDirectory string, runtimeDirectory string) (string, error) {
	toolDirectory = strings.TrimSpace(toolDirectory)
	if toolDirectory == "" {
		return "", fmt.Errorf("toolDirectory is required")
	}
	path := strings.TrimSpace(runtimeDirectory)
	if path == "" {
		return "", fmt.Errorf("runtime.directory is required")
	}
	cleaned := filepath.Clean(filepath.FromSlash(path))
	if filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" {
		return "", fmt.Errorf("runtime.directory must be relative")
	}
	resolved := filepath.Clean(filepath.Join(toolDirectory, cleaned))
	if !pathWithin(toolDirectory, resolved) {
		return "", fmt.Errorf("runtime.directory escapes tool directory")
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

func requiresBundledWindowsService(request searchRequest) bool {
	return request.ScopeMode == scopeModeAllLocalDrives
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

func stopBundledEverything(ctx context.Context, executable string, instanceName string, config Config) error {
	timeout := time.Duration(config.Limits.DefaultConnectTimeoutMs) * time.Millisecond
	_, err := runCommandOutput(ctx, timeout, executable, everythingExitArgs(instanceName, config.Limits.DefaultConnectTimeoutMs)...)
	return err
}

func persistBundledDatabaseIfMissing(ctx context.Context, executable string, databasePath string, request searchRequest) error {
	if _, err := os.Stat(databasePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat Everything database: %w", err)
	}
	timeout := time.Duration(request.TimeoutMs) * time.Millisecond
	if _, err := runCommandOutput(ctx, timeout, executable, everythingSaveDatabaseArgs(request.InstanceName, request.ConnectTimeoutMs)...); err != nil {
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

type bundledWindowsServiceOps interface {
	Exists(ctx context.Context, name string, config Config) (bool, error)
	Install(ctx context.Context, executable string, instanceName string, config Config) error
	Uninstall(ctx context.Context, executable string, instanceName string, config Config) error
	UsesExecutable(ctx context.Context, name string, expectedExecutable string, config Config) (bool, error)
	EnsureRunning(ctx context.Context, name string, config Config) error
}

type scBundledWindowsServiceOps struct{}

var bundledServiceOps bundledWindowsServiceOps = scBundledWindowsServiceOps{}

func (scBundledWindowsServiceOps) Exists(ctx context.Context, name string, config Config) (bool, error) {
	return windowsServiceExists(ctx, name, config)
}

func (scBundledWindowsServiceOps) Install(ctx context.Context, executable string, instanceName string, config Config) error {
	if _, err := runCommandOutput(ctx, runtimeCommandTimeout(config), executable, "-instance", instanceName, "-install-service"); err != nil {
		return fmt.Errorf("install bundled Everything service: %w", err)
	}
	return nil
}

func (scBundledWindowsServiceOps) Uninstall(ctx context.Context, executable string, instanceName string, config Config) error {
	if _, err := runCommandOutput(ctx, runtimeCommandTimeout(config), executable, "-instance", instanceName, "-uninstall-service"); err != nil {
		return fmt.Errorf("uninstall bundled Everything service: %w", err)
	}
	return nil
}

func (scBundledWindowsServiceOps) UsesExecutable(ctx context.Context, name string, expectedExecutable string, config Config) (bool, error) {
	return windowsServiceUsesExecutable(ctx, name, expectedExecutable, config)
}

func (scBundledWindowsServiceOps) EnsureRunning(ctx context.Context, name string, config Config) error {
	return ensureWindowsServiceRunning(ctx, name, config)
}

func ensureBundledWindowsService(ctx context.Context, executable string, instanceName string, config Config) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("bundled full-disk Everything search is only supported on Windows")
	}
	serviceName := bundledServiceName(instanceName)
	exists, err := bundledServiceOps.Exists(ctx, serviceName, config)
	if err != nil {
		return err
	}
	if !exists {
		if err := bundledServiceOps.Install(ctx, executable, instanceName, config); err != nil {
			return err
		}
	} else {
		matches, err := bundledServiceOps.UsesExecutable(ctx, serviceName, executable, config)
		if err != nil {
			return err
		}
		if !matches {
			if err := bundledServiceOps.Uninstall(ctx, executable, instanceName, config); err != nil {
				return err
			}
			if err := bundledServiceOps.Install(ctx, executable, instanceName, config); err != nil {
				return err
			}
		}
	}
	matches, err := bundledServiceOps.UsesExecutable(ctx, serviceName, executable, config)
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("bundled Everything service uses a different runtime path")
	}
	return bundledServiceOps.EnsureRunning(ctx, serviceName, config)
}

func bundledServiceName(instanceName string) string {
	return "Everything (" + strings.TrimSpace(instanceName) + ")"
}

func windowsServiceExists(ctx context.Context, name string, config Config) (bool, error) {
	output, err := runCommandOutput(ctx, serviceQueryTimeout(config), "sc.exe", "query", name)
	if err == nil {
		return true, nil
	}
	if isWindowsServiceNotFound(output) || isWindowsServiceNotFound(err.Error()) {
		return false, nil
	}
	return false, fmt.Errorf("query bundled Everything service: %w", err)
}

func windowsServiceUsesExecutable(ctx context.Context, name string, expectedExecutable string, config Config) (bool, error) {
	output, err := runCommandOutput(ctx, serviceQueryTimeout(config), "sc.exe", "qc", name)
	if err != nil {
		return false, fmt.Errorf("query bundled Everything service config: %w", err)
	}
	actual := windowsServiceBinaryPath(output)
	if actual == "" {
		return false, fmt.Errorf("bundled Everything service binary path is missing: %s", name)
	}
	return serviceBinaryPathUsesExecutable(actual, expectedExecutable), nil
}

func ensureWindowsServiceRunning(ctx context.Context, name string, config Config) error {
	running, err := windowsServiceRunning(ctx, name, config)
	if err != nil {
		return err
	}
	if running {
		return nil
	}
	if _, err := runCommandOutput(ctx, runtimeCommandTimeout(config), "sc.exe", "start", name); err != nil {
		return fmt.Errorf("start bundled Everything service: %w", err)
	}
	deadline := time.Now().Add(runtimeCommandTimeout(config))
	for {
		running, err := windowsServiceRunning(ctx, name, config)
		if err != nil {
			return err
		}
		if running {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("bundled Everything service did not reach RUNNING state: %s", name)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		time.Sleep(time.Duration(config.Runtime.ProbeIntervalMs) * time.Millisecond)
	}
}

func windowsServiceRunning(ctx context.Context, name string, config Config) (bool, error) {
	output, err := runCommandOutput(ctx, serviceQueryTimeout(config), "sc.exe", "query", name)
	if err != nil {
		return false, fmt.Errorf("query bundled Everything service state: %w", err)
	}
	return strings.Contains(strings.ToUpper(output), "RUNNING"), nil
}

func windowsServiceBinaryPath(scOutput string) string {
	for _, line := range strings.Split(scOutput, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "BINARY_PATH_NAME") {
			continue
		}
		_, value, found := strings.Cut(trimmed, ":")
		if !found {
			return ""
		}
		return strings.TrimSpace(value)
	}
	return ""
}

func serviceBinaryPathUsesExecutable(binaryPath string, expectedExecutable string) bool {
	actual := serviceBinaryExecutablePath(binaryPath)
	if actual == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(actual), filepath.Clean(expectedExecutable))
}

func serviceBinaryExecutablePath(binaryPath string) string {
	value := strings.TrimSpace(binaryPath)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "\"") {
		end := strings.Index(value[1:], "\"")
		if end < 0 {
			return strings.Trim(value, "\"")
		}
		return value[1 : end+1]
	}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func isWindowsServiceNotFound(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "1060") || strings.Contains(lower, "does not exist")
}

func serviceQueryTimeout(config Config) time.Duration {
	return time.Duration(config.Limits.DefaultConnectTimeoutMs) * time.Millisecond
}

func runtimeCommandTimeout(config Config) time.Duration {
	timeout := time.Duration(config.Limits.DefaultTimeoutMs) * time.Millisecond
	return minDuration(timeout, 20*time.Second)
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

func bundledRuntimeResponds(ctx context.Context, executable string, instanceName string, config Config) bool {
	return waitEverythingReady(ctx, executable, instanceName, config, false) == nil
}

func waitIndexedFoldersReady(ctx context.Context, executable string, request searchRequest, config Config) error {
	folders := readyProbePaths(request)
	if len(folders) == 0 {
		return nil
	}
	deadline := time.Now().Add(time.Duration(request.TimeoutMs) * time.Millisecond)
	probeInterval := time.Duration(config.Runtime.ProbeIntervalMs) * time.Millisecond
	var lastErr error
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		probeTimeout := minDuration(time.Duration(request.ConnectTimeoutMs)*time.Millisecond, remaining)
		allReady := true
		for _, folder := range folders {
			output, err := runCommandOutput(ctx, probeTimeout, executable, everythingCountArgs(request.InstanceName, int(probeTimeout/time.Millisecond), folder)...)
			if err != nil {
				lastErr = err
				allReady = false
				continue
			}
			ready, err := folderIndexReady(folder, output)
			if err != nil {
				lastErr = err
				allReady = false
				continue
			}
			if !ready {
				lastErr = fmt.Errorf("Everything folder index has no visible entries yet: %s", folder)
				allReady = false
			}
		}
		if allReady {
			return nil
		}
		if sleep := minDuration(probeInterval, time.Until(deadline)); sleep > 0 {
			time.Sleep(sleep)
		}
	}
	if lastErr != nil {
		return fmt.Errorf("Everything folder index is not ready: %w", lastErr)
	}
	return fmt.Errorf("Everything folder index is not ready")
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
