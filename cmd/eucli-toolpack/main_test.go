package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func TestRunBuildsShellCommandIntoAbsoluteDataDir(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "runtime-data")
	gitBashRoot := filepath.Join(t.TempDir(), "git-bash-root")
	writeFixtureFile(t, filepath.Join(gitBashRoot, "bin", "bash.exe"))
	if err := run(context.Background(), []string{"-tool", "shell_command", "-data-dir", dataDir, "-asset-root", "git-bash-root=" + gitBashRoot}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	targetDir := filepath.Join(dataDir, "tools", "shell_command")
	assertFile(t, filepath.Join(targetDir, "config.json"))
	assertFile(t, filepath.Join(targetDir, "providers", "git-bash", "bin", "bash.exe"))
	config := readRuntimeConfigFile(t, filepath.Join(targetDir, "config.json"))
	providers, err := runtimeProviders(config)
	if err != nil {
		t.Fatalf("runtimeProviders() error = %v", err)
	}
	if len(providers) != 3 || providerID(providers[0]) != "git-bash" || !providerEnabled(providers[0]) || providerEnabled(providers[1]) || providerEnabled(providers[2]) {
		t.Fatalf("config providers = %#v", providers)
	}
	binaryRelPath := filepath.Join("binary", runtime.GOOS+"-"+runtime.GOARCH, executableName("shell_command"))
	assertFile(t, filepath.Join(targetDir, binaryRelPath))
	tool := readToolDefinitionFile(t, filepath.Join(targetDir, "data.json"))
	if tool.ID != "shell_command" || tool.Directory != "." {
		t.Fatalf("tool definition = %#v", tool)
	}
	providerSchema := tool.InputSchema["properties"].(map[string]any)["provider"].(map[string]any)
	providerEnum := providerSchema["enum"].([]any)
	if len(providerEnum) != 1 || providerEnum[0] != "git-bash" {
		t.Fatalf("provider enum = %#v", providerEnum)
	}
	configProviderSchema := tool.UserConfigSchema["properties"].(map[string]any)["provider"].(map[string]any)
	configProviderEnum := configProviderSchema["enum"].([]any)
	if len(configProviderEnum) != 1 || configProviderEnum[0] != "git-bash" {
		t.Fatalf("config provider enum = %#v", configProviderEnum)
	}
	if len(tool.Binaries) != 1 || tool.Binaries[0].Path != filepath.ToSlash(binaryRelPath) || filepath.IsAbs(tool.Binaries[0].Path) {
		t.Fatalf("binaries = %#v", tool.Binaries)
	}
}

func TestRunBuildsSciCalculatorWithBundledPythonRuntime(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "runtime-data")
	pythonRoot := filepath.Join(t.TempDir(), "python-runtime")
	writeFixtureFile(t, filepath.Join(pythonRoot, "python.exe"))
	if err := run(context.Background(), []string{"-tool", "sci_calculator", "-data-dir", dataDir, "-asset-root", "sci-calculator-python-runtime=" + pythonRoot}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	targetDir := filepath.Join(dataDir, "tools", "sci_calculator")
	assertFile(t, filepath.Join(targetDir, "config.json"))
	assertFile(t, filepath.Join(targetDir, "runtime", "python", "python.exe"))
	binaryRelPath := filepath.Join("binary", runtime.GOOS+"-"+runtime.GOARCH, executableName("sci_calculator"))
	assertFile(t, filepath.Join(targetDir, binaryRelPath))
	tool := readToolDefinitionFile(t, filepath.Join(targetDir, "data.json"))
	if tool.ID != "sci_calculator" || tool.Name != "SciCalculator" || tool.Directory != "." {
		t.Fatalf("tool definition = %#v", tool)
	}
	if len(tool.Binaries) != 1 || tool.Binaries[0].Path != filepath.ToSlash(binaryRelPath) || filepath.IsAbs(tool.Binaries[0].Path) {
		t.Fatalf("binaries = %#v", tool.Binaries)
	}
}

func TestRunBuildsSciCalculatorWithoutBundledPythonRuntime(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "runtime-data")
	if err := run(context.Background(), []string{"-tool", "sci_calculator", "-data-dir", dataDir}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	targetDir := filepath.Join(dataDir, "tools", "sci_calculator")
	assertFile(t, filepath.Join(targetDir, "config.json"))
	if _, err := os.Stat(filepath.Join(targetDir, "runtime", "python", "python.exe")); !os.IsNotExist(err) {
		t.Fatalf("bundled python stat error = %v, want not exist", err)
	}
}

func TestRunRequiresExplicitSciCalculatorPythonRuntimeAsset(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "runtime-data")
	if err := run(context.Background(), []string{"-tool", "sci_calculator", "-data-dir", dataDir, "-require-asset-root", "sci-calculator-python-runtime"}); err == nil {
		t.Fatalf("run() error = nil, want required asset root error")
	}
}

func TestRunRejectsSciCalculatorPythonRuntimeDirectoryExecutable(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "runtime-data")
	pythonRoot := filepath.Join(t.TempDir(), "python-runtime")
	if err := os.MkdirAll(filepath.Join(pythonRoot, "python.exe"), 0o755); err != nil {
		t.Fatalf("MkdirAll(python.exe directory) error = %v", err)
	}
	if err := run(context.Background(), []string{"-tool", "sci_calculator", "-data-dir", dataDir, "-asset-root", "sci-calculator-python-runtime=" + pythonRoot}); err == nil {
		t.Fatalf("run() error = nil, want required file directory error")
	}
}

func TestCopyDeclaredAssetRootsAcceptsRequiredFiles(t *testing.T) {
	assetRoot := filepath.Join(t.TempDir(), "everything-root")
	writeFixtureFile(t, filepath.Join(assetRoot, "Everything.exe"))
	writeFixtureFile(t, filepath.Join(assetRoot, "es.exe"))
	targetDir := filepath.Join(t.TempDir(), "tool")

	err := copyDeclaredAssetRoots(targetDir, toolpackSpec{AssetRoots: []assetRootSpec{{
		Name:          "everything-root",
		Target:        "providers/everything",
		RequiredFiles: []string{"Everything.exe", "es.exe"},
	}}}, assetRootFlags{"everything-root": assetRoot}, nil)
	if err != nil {
		t.Fatalf("copyDeclaredAssetRoots() error = %v", err)
	}
	assertFile(t, filepath.Join(targetDir, "providers", "everything", "Everything.exe"))
	assertFile(t, filepath.Join(targetDir, "providers", "everything", "es.exe"))
}

func TestCopyDeclaredAssetRootsRejectsMissingRequiredFile(t *testing.T) {
	assetRoot := filepath.Join(t.TempDir(), "everything-root")
	writeFixtureFile(t, filepath.Join(assetRoot, "Everything.exe"))
	targetDir := filepath.Join(t.TempDir(), "tool")

	err := copyDeclaredAssetRoots(targetDir, toolpackSpec{AssetRoots: []assetRootSpec{{
		Name:          "everything-root",
		Target:        "providers/everything",
		RequiredFiles: []string{"Everything.exe", "es.exe"},
	}}}, assetRootFlags{"everything-root": assetRoot}, nil)
	if err == nil || !strings.Contains(err.Error(), "es.exe") {
		t.Fatalf("copyDeclaredAssetRoots() error = %v, want missing es.exe error", err)
	}
}

func TestCopyDeclaredAssetRootsAcceptsLegacyRequiredFile(t *testing.T) {
	assetRoot := filepath.Join(t.TempDir(), "python-runtime")
	writeFixtureFile(t, filepath.Join(assetRoot, "python.exe"))
	targetDir := filepath.Join(t.TempDir(), "tool")

	err := copyDeclaredAssetRoots(targetDir, toolpackSpec{AssetRoots: []assetRootSpec{{
		Name:         "python-runtime",
		Target:       "runtime/python",
		RequiredFile: "python.exe",
	}}}, assetRootFlags{"python-runtime": assetRoot}, nil)
	if err != nil {
		t.Fatalf("copyDeclaredAssetRoots() error = %v", err)
	}
	assertFile(t, filepath.Join(targetDir, "runtime", "python", "python.exe"))
}

func TestCopyDeclaredAssetRootsAcceptsExistingPackagedTarget(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "tool")
	writeFixtureFile(t, filepath.Join(targetDir, "providers", "everything", "Everything.exe"))
	writeFixtureFile(t, filepath.Join(targetDir, "providers", "everything", "es.exe"))

	err := copyDeclaredAssetRoots(targetDir, toolpackSpec{AssetRoots: []assetRootSpec{{
		Name:          "everything-root",
		Target:        "providers/everything",
		RequiredFiles: []string{"Everything.exe", "es.exe"},
	}}}, nil, nil)
	if err != nil {
		t.Fatalf("copyDeclaredAssetRoots() error = %v", err)
	}
}

func TestCopyDeclaredAssetRootsRejectsIncompletePackagedTarget(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "tool")
	writeFixtureFile(t, filepath.Join(targetDir, "providers", "everything", "Everything.exe"))

	err := copyDeclaredAssetRoots(targetDir, toolpackSpec{AssetRoots: []assetRootSpec{{
		Name:          "everything-root",
		Target:        "providers/everything",
		RequiredFiles: []string{"Everything.exe", "es.exe"},
	}}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "es.exe") {
		t.Fatalf("copyDeclaredAssetRoots() error = %v, want missing packaged es.exe error", err)
	}
}

func TestCopyDeclaredAssetRootsRejectsMissingRequiredPackagedTarget(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "tool")

	err := copyDeclaredAssetRoots(targetDir, toolpackSpec{AssetRoots: []assetRootSpec{{
		Name:              "everything-root",
		Target:            "providers/everything",
		RequiredFiles:     []string{"Everything.exe", "es.exe"},
		RequiredInPackage: true,
	}}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "was not produced") {
		t.Fatalf("copyDeclaredAssetRoots() error = %v, want missing required packaged target error", err)
	}
}

func TestCopyDeclaredAssetRootsAcceptsMissingOptionalPackagedTarget(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "tool")

	err := copyDeclaredAssetRoots(targetDir, toolpackSpec{AssetRoots: []assetRootSpec{{
		Name:         "python-runtime",
		Target:       "runtime/python",
		RequiredFile: "python.exe",
	}}}, nil, nil)
	if err != nil {
		t.Fatalf("copyDeclaredAssetRoots() error = %v", err)
	}
}

func TestCopyDeclaredAssetRootsRejectsEscapingRequiredFile(t *testing.T) {
	assetRoot := filepath.Join(t.TempDir(), "asset-root")
	writeFixtureFile(t, filepath.Join(assetRoot, "inside.exe"))
	targetDir := filepath.Join(t.TempDir(), "tool")

	err := copyDeclaredAssetRoots(targetDir, toolpackSpec{AssetRoots: []assetRootSpec{{
		Name:          "asset-root",
		Target:        "runtime",
		RequiredFiles: []string{"../outside.exe"},
	}}}, assetRootFlags{"asset-root": assetRoot}, nil)
	if err == nil || !strings.Contains(err.Error(), "paths must be relative") {
		t.Fatalf("copyDeclaredAssetRoots() error = %v, want relative path error", err)
	}
}

func writeFixtureFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func readToolDefinitionFile(t *testing.T, path string) types.ToolDefinition {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var tool types.ToolDefinition
	if err := json.Unmarshal(payload, &tool); err != nil {
		t.Fatalf("Unmarshal(tool) error = %v", err)
	}
	return tool
}

func readRuntimeConfigFile(t *testing.T, path string) map[string]any {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var config map[string]any
	if err := json.Unmarshal(payload, &config); err != nil {
		t.Fatalf("Unmarshal(config) error = %v", err)
	}
	return config
}

func assertFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("%s is a directory", path)
	}
}
