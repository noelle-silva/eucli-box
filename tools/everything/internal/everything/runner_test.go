package everything

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestExecuteUsesBundledEverythingProvider(t *testing.T) {
	fixture := newEverythingFixture(t, true)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"query": "notes", "scopePath": ".", "maxResults": 3, "description": "fixture search"},
		DefaultConfig:        map[string]any{"maxOutputChars": 20000},
		ToolBodyDirectory:    fixture.toolDir,
		ToolDataDirectory:    fixture.dataDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Content, "notes.md") || result.Metadata["provider"] != "bundled" || result.Metadata["resultsCount"] != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.Metadata["scopePath"] != fixture.hostDir || result.Metadata["executableSource"] != "bundled" || result.Metadata["runtimeSource"] != "bundled" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteAllowsExplicitExternalCLIOverride(t *testing.T) {
	fixture := newEverythingFixture(t, true)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"query": "notes", "scopePath": ".", "maxResults": 3, "description": "fixture search"},
		UserConfig:           map[string]any{"esPath": fixture.esExe},
		DefaultConfig:        map[string]any{"maxOutputChars": 20000},
		ToolBodyDirectory:    fixture.toolDir,
		ToolDataDirectory:    fixture.dataDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Content, "notes.md") || result.Metadata["query"] != "notes" || result.Metadata["resultsCount"] != 1 || result.Metadata["description"] != "fixture search" {
		t.Fatalf("result = %#v", result)
	}
	if result.Metadata["scopePath"] != fixture.hostDir || result.Metadata["provider"] != "external" || result.Metadata["executableSource"] != "userConfig.esPath" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteDefaultsToBundledFullDiskRuntime(t *testing.T) {
	fixture := newEverythingFixture(t, true)
	serviceOps := &fakeBundledServiceOps{}
	useFakeBundledServiceOps(t, serviceOps)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"query": "notes", "maxResults": 3, "description": "default full disk"},
		DefaultConfig:        map[string]any{"maxOutputChars": 20000},
		ToolBodyDirectory:    fixture.toolDir,
		ToolDataDirectory:    fixture.dataDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if result.Metadata["scopeMode"] != scopeModeAllLocalDrives || result.Metadata["scopePath"] != "" || result.Metadata["runtimeSource"] != "bundled" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
	if !serviceOps.installed || !serviceOps.executableChecked || !serviceOps.runningEnsured {
		t.Fatalf("service ops were not exercised: %#v", serviceOps)
	}
	if paths, ok := result.Metadata["scopePaths"].([]string); !ok || len(paths) == 0 {
		t.Fatalf("scopePaths = %#v", result.Metadata["scopePaths"])
	}
	if !strings.Contains(result.Content, "Scope: all local drives") || !strings.Contains(result.Content, "notes.md") {
		t.Fatalf("content = %s", result.Content)
	}
}

func TestResolveSearchScopeDefaultsToLocalDriveRoots(t *testing.T) {
	scope, err := resolveSearchScope("", "")
	if err != nil {
		t.Fatal(err)
	}
	if scope.Mode != scopeModeAllLocalDrives || scope.SearchPath != "" || len(scope.DisplayPaths) == 0 || len(scope.IndexPaths) != 0 {
		t.Fatalf("scope = %#v", scope)
	}
}

func TestWindowsServiceBinaryPathParsing(t *testing.T) {
	path := windowsServiceBinaryPath(`SERVICE_NAME: Everything (eucli-box-everything)
        BINARY_PATH_NAME   : "E:\eucli-project\eucli-box\tools\everything\providers\everything\Everything.exe" -svc -instance "eucli-box-everything"`)
	if path == "" || !serviceBinaryPathUsesExecutable(path, `E:\eucli-project\eucli-box\tools\everything\providers\everything\Everything.exe`) {
		t.Fatalf("path = %q", path)
	}
}

func TestExecuteFailsWhenBundledProviderMissing(t *testing.T) {
	fixture := newEverythingFixture(t, false)
	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"query": "notes"}, ToolBodyDirectory: fixture.toolDir, ToolDataDirectory: fixture.dataDir})
	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "provider \"bundled\" es executable does not exist") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteRejectsInvalidLimit(t *testing.T) {
	fixture := newEverythingFixture(t, true)
	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"query": "notes", "maxResults": 999}, UserConfig: map[string]any{"esPath": fixture.esExe}, ToolBodyDirectory: fixture.toolDir, ToolDataDirectory: fixture.dataDir})
	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "between 1 and 500") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteRejectsUnsupportedActionArgument(t *testing.T) {
	fixture := newEverythingFixture(t, true)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"action": "search", "query": "notes"},
		ToolBodyDirectory:    fixture.toolDir,
		ToolDataDirectory:    fixture.dataDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "not supported") {
		t.Fatalf("result = %#v", result)
	}
}

func TestParseSearchCSVRemovesUTF8BOM(t *testing.T) {
	results, err := parseSearchCSV("\ufeff\"file.txt\",\"C:\\Temp\",12,2026/06/01 10:00\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "file.txt" || results[0].Path != "C:\\Temp" {
		t.Fatalf("results = %+v", results)
	}
}

func TestEverythingSearchArgsUseOptionalInstanceAndScope(t *testing.T) {
	args := everythingSearchArgs("custom", "todo.md", 10, `C:\Temp\results.csv`, `D:\Projects`, 5000)
	joined := strings.Join(args, "\n")
	if !containsAll(joined, "-instance", "custom", "-path-column", "-path", `D:\Projects`, "todo.md") {
		t.Fatalf("args = %+v", args)
	}
	args = everythingSearchArgs("", "todo.md", 10, `C:\Temp\results.csv`, "", 5000)
	for _, arg := range args {
		if arg == "-instance" || arg == "-path" {
			t.Fatalf("empty instance and scope must not add optional filters: %+v", args)
		}
	}
}

func TestRuntimeConfigIncludesIndexedScopePath(t *testing.T) {
	runtimeDir := `E:\eucli-project\eucli-box\data\tools\everything\runtime`
	target := filepath.Join(t.TempDir(), "Everything.ini")
	if err := writeRuntimeConfig(target, runtimeConfigContent(runtimeDir, []string{`C:\`, `E:\eucli-project\eucli-box`})); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if !containsAll(text, `folders="C:\\","E:\\eucli-project\\eucli-box"`, "folder_monitor_changes=1,1", "folder_update_types=0,0") {
		t.Fatalf("runtime config = %s", text)
	}
}

func TestRuntimeConfigOmitsFolderListForFullDiskSearch(t *testing.T) {
	runtimeDir := `E:\eucli-project\eucli-box\data\tools\everything\runtime`
	target := filepath.Join(t.TempDir(), "Everything.ini")
	if err := writeRuntimeConfig(target, runtimeConfigContent(runtimeDir, nil)); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if strings.Contains(text, "folders=") || strings.Contains(text, "folder_monitor_changes=") || strings.Contains(text, "folder_update_types=") {
		t.Fatalf("full disk config must use Everything volume indexing, got %s", text)
	}
}

func TestFormatContentShowsAllLocalDrivesScope(t *testing.T) {
	content, _ := formatContent(searchResponse{Query: "notes", Limit: 3, ScopeMode: scopeModeAllLocalDrives, ScopePaths: []string{`C:\`, `E:\`}}, searchRequest{MaxOutputChars: 2000})
	if !containsAll(content, "Scope: all local drives", `C:\`, `E:\`) {
		t.Fatalf("content = %s", content)
	}
}

func TestEnsureBundledWindowsServiceDoesNotReinstallMatchingService(t *testing.T) {
	serviceOps := &fakeBundledServiceOps{exists: true}
	useFakeBundledServiceOps(t, serviceOps)
	if err := ensureBundledWindowsService(context.Background(), `E:\Tools\Everything.exe`, "eucli-box-everything-test", fixtureConfig()); err != nil {
		t.Fatal(err)
	}
	if serviceOps.installed || serviceOps.uninstalled || !serviceOps.runningEnsured {
		t.Fatalf("service ops = %#v", serviceOps)
	}
}

func TestPersistBundledDatabaseIfMissingSavesMissingDatabase(t *testing.T) {
	fixture := newEverythingFixture(t, true)
	databasePath := filepath.Join(t.TempDir(), "Everything.db")
	t.Setenv("FAKE_EVERYTHING_DB_PATH", databasePath)
	request := searchRequest{InstanceName: "custom", TimeoutMs: 30000, ConnectTimeoutMs: 5000}
	if err := persistBundledDatabaseIfMissing(context.Background(), fixture.esExe, databasePath, request); err != nil {
		t.Fatal(err)
	}
	assertFile(t, databasePath)
}

func TestPersistBundledDatabaseIfMissingKeepsExistingDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "Everything.db")
	if err := os.WriteFile(databasePath, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := searchRequest{InstanceName: "custom", TimeoutMs: 30000, ConnectTimeoutMs: 5000}
	if err := persistBundledDatabaseIfMissing(context.Background(), filepath.Join(t.TempDir(), "missing-es"), databasePath, request); err != nil {
		t.Fatal(err)
	}
}

func TestFolderIndexReadyRequiresVisibleEntriesForNonEmptyScope(t *testing.T) {
	nonEmpty := t.TempDir()
	if err := os.WriteFile(filepath.Join(nonEmpty, "file.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	ready, err := folderIndexReady(nonEmpty, "0\n")
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("non-empty folder with zero indexed entries must wait")
	}
	ready, err = folderIndexReady(nonEmpty, "1\n")
	if err != nil || !ready {
		t.Fatalf("ready = %v, err = %v", ready, err)
	}
	empty := t.TempDir()
	ready, err = folderIndexReady(empty, "0\n")
	if err != nil || !ready {
		t.Fatalf("empty folder ready = %v, err = %v", ready, err)
	}
}

func TestBundledRuntimeLockSerializesRuntimePreparation(t *testing.T) {
	fixture := newEverythingFixture(t, true)
	request := searchRequest{TimeoutMs: 40}
	config := fixtureConfig()
	first, err := acquireBundledRuntimeLock(context.Background(), fixture.dataDir, config, request)
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now()
	second, err := acquireBundledRuntimeLock(context.Background(), fixture.dataDir, config, request)
	if err == nil {
		second.Release()
		t.Fatal("second lock must wait until timeout while first lock is held")
	}
	if time.Since(startedAt) < 30*time.Millisecond {
		t.Fatalf("lock returned too quickly: %s", time.Since(startedAt))
	}
	first.Release()
	third, err := acquireBundledRuntimeLock(context.Background(), fixture.dataDir, config, searchRequest{TimeoutMs: 1000})
	if err != nil {
		t.Fatal(err)
	}
	third.Release()
}

type everythingFixture struct {
	toolDir string
	dataDir string
	hostDir string
	esExe   string
}

type fakeBundledServiceOps struct {
	exists            bool
	installed         bool
	uninstalled       bool
	executableChecked bool
	runningEnsured    bool
}

func useFakeBundledServiceOps(t *testing.T, ops bundledWindowsServiceOps) {
	t.Helper()
	previous := bundledServiceOps
	bundledServiceOps = ops
	t.Cleanup(func() {
		bundledServiceOps = previous
	})
}

func (ops *fakeBundledServiceOps) Exists(ctx context.Context, name string, config Config) (bool, error) {
	return ops.exists, nil
}

func (ops *fakeBundledServiceOps) Install(ctx context.Context, executable string, instanceName string, config Config) error {
	ops.installed = true
	ops.exists = true
	return nil
}

func (ops *fakeBundledServiceOps) Uninstall(ctx context.Context, executable string, instanceName string, config Config) error {
	ops.uninstalled = true
	ops.exists = false
	return nil
}

func (ops *fakeBundledServiceOps) UsesExecutable(ctx context.Context, name string, expectedExecutable string, config Config) (bool, error) {
	ops.executableChecked = true
	return ops.exists, nil
}

func (ops *fakeBundledServiceOps) EnsureRunning(ctx context.Context, name string, config Config) error {
	ops.runningEnsured = true
	return nil
}

func newEverythingFixture(t *testing.T, includeBundledProvider bool) everythingFixture {
	t.Helper()
	root := t.TempDir()
	toolDir := filepath.Join(root, "tool")
	dataDir := filepath.Join(root, "data")
	hostDir := filepath.Join(root, "host")
	esExe := filepath.Join(root, executableName("fake-es"))
	bundledDir := filepath.Join(toolDir, "providers", "everything")
	bundledES := filepath.Join(bundledDir, executableName("es"))
	bundledRuntime := filepath.Join(bundledDir, executableName("Everything"))
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(tool) error = %v", err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(data) error = %v", err)
	}
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(host) error = %v", err)
	}
	t.Setenv("FAKE_EVERYTHING_TOOL_DIR", dataDir)
	buildFakeES(t, esExe)
	if includeBundledProvider {
		if err := os.MkdirAll(bundledDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(bundled) error = %v", err)
		}
		buildFakeES(t, bundledES)
		buildFakeRuntime(t, bundledRuntime)
	}
	config := fixtureConfig()
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal(config) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "config.json"), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	return everythingFixture{toolDir: toolDir, dataDir: dataDir, hostDir: hostDir, esExe: esExe}
}

func fixtureConfig() Config {
	return Config{
		DefaultProvider: "bundled",
		ESPathEnv:       "MISSING_EVERYTHING_ES_PATH_FOR_TEST",
		Providers: []ProviderConfig{{
			ID:                 "bundled",
			Mode:               "bundled",
			Enabled:            true,
			Executables:        []types.ToolBinary{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Path: filepath.ToSlash(filepath.Join("providers", "everything", executableName("es")))}},
			RuntimeExecutables: []types.ToolBinary{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Path: filepath.ToSlash(filepath.Join("providers", "everything", executableName("Everything")))}},
		}},
		Runtime: RuntimeConfig{Directory: "runtime", DefaultInstanceName: "eucli-box-everything-test", ReadyTimeoutMs: 30000, ProbeIntervalMs: 250},
		Limits:  LimitsConfig{DefaultTimeoutMs: 30000, MaxTimeoutMs: 120000, DefaultConnectTimeoutMs: 5000, DefaultMaxResults: 80, MaxResults: 500, MaxOutputChars: 30000},
	}
}

func buildFakeES(t *testing.T, target string) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(source, []byte(fakeESSource), 0o644); err != nil {
		t.Fatalf("WriteFile(fake es source) error = %v", err)
	}
	cmd := exec.Command("go", "build", "-o", target, source)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build fake es failed: %v\n%s", err, output)
	}
}

func buildFakeRuntime(t *testing.T, target string) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(source, []byte(fakeRuntimeSource), 0o644); err != nil {
		t.Fatalf("WriteFile(fake runtime source) error = %v", err)
	}
	cmd := exec.Command("go", "build", "-o", target, source)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build fake runtime failed: %v\n%s", err, output)
	}
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func containsAll(text string, values ...string) bool {
	for _, value := range values {
		if !strings.Contains(text, value) {
			return false
		}
	}
	return true
}

func assertFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s error = %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("%s is a directory", path)
	}
}

const fakeESSource = `package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	csvPath := ""
	instanceName := ""
	scopePath := "C:\\Temp"
	query := ""
	for index, arg := range os.Args {
		if arg == "-instance" && index+1 < len(os.Args) {
			instanceName = os.Args[index+1]
		}
	}
	for _, arg := range os.Args {
		if arg == "-get-everything-version" {
			fmt.Println("test-version")
			return
		}
		if arg == "-save-db" {
			dbPath := os.Getenv("FAKE_EVERYTHING_DB_PATH")
			if dbPath == "" && os.Getenv("FAKE_EVERYTHING_TOOL_DIR") != "" && instanceName != "" {
				dbPath = filepath.Join(os.Getenv("FAKE_EVERYTHING_TOOL_DIR"), "runtime", instanceName, "Everything.db")
			}
			if dbPath != "" {
				_ = os.MkdirAll(filepath.Dir(dbPath), 0755)
				_ = os.WriteFile(dbPath, []byte("db"), 0644)
			}
			return
		}
		if arg == "-exit" {
			return
		}
		if arg == "-get-result-count" {
			fmt.Println("1")
			return
		}
	}
	for index, arg := range os.Args {
		if arg == "-export-csv" && index+1 < len(os.Args) {
			csvPath = os.Args[index+1]
		}
		if arg == "-path" && index+1 < len(os.Args) {
			scopePath = os.Args[index+1]
		}
	}
	if len(os.Args) > 1 {
		query = os.Args[len(os.Args)-1]
	}
	if query == "fail" {
		fmt.Fprintln(os.Stderr, "fake failure")
		os.Exit(2)
	}
	if csvPath == "" {
		fmt.Fprintln(os.Stderr, "missing csv path")
		os.Exit(3)
	}
	file, err := os.Create(csvPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(4)
	}
	defer file.Close()
	_, _ = file.WriteString("\ufeff")
	writer := csv.NewWriter(file)
	_ = writer.Write([]string{"notes.md", scopePath, "12", "2026/06/01 10:00"})
	writer.Flush()
}
`

const fakeRuntimeSource = `package main

func main() {}
`
