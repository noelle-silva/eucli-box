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
	targetDir := filepath.Join(dataDir, "tool-bodies", "shell_command")
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
	tool := readToolDefinitionFile(t, filepath.Join(targetDir, "definition.json"))
	if tool.ID != "shell_command" || tool.BodyDirectory != "." || tool.DefaultInvocationMode != types.ToolInvocationModeSync {
		t.Fatalf("tool definition = %#v", tool)
	}
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot() error = %v", err)
	}
	sourceTool, err := readToolDefinition(toolSource{ID: "shell_command", Dir: filepath.Join(repoRoot, "tools", "shell_command")})
	if err != nil {
		t.Fatalf("readToolDefinition() error = %v", err)
	}
	if tool.Version != sourceTool.Version || tool.EucliBoxCompatibility != sourceTool.EucliBoxCompatibility {
		t.Fatalf("tool release metadata = %#v", tool)
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
	targetDir := filepath.Join(dataDir, "tool-bodies", "sci_calculator")
	assertFile(t, filepath.Join(targetDir, "config.json"))
	assertFile(t, filepath.Join(targetDir, "runtime", "python", "python.exe"))
	binaryRelPath := filepath.Join("binary", runtime.GOOS+"-"+runtime.GOARCH, executableName("sci_calculator"))
	assertFile(t, filepath.Join(targetDir, binaryRelPath))
	tool := readToolDefinitionFile(t, filepath.Join(targetDir, "definition.json"))
	if tool.ID != "sci_calculator" || tool.Name != "SciCalculator" || tool.BodyDirectory != "." || tool.DefaultInvocationMode != types.ToolInvocationModeSync {
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
	targetDir := filepath.Join(dataDir, "tool-bodies", "sci_calculator")
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

func TestRunRebuildsToolBodyWithoutChangingToolData(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "runtime-data")
	settings := filepath.Join(dataDir, "tool-data", "sci_calculator", "settings.json")
	writeFixtureFile(t, settings)

	if err := run(context.Background(), []string{"-tool", "sci_calculator", "-data-dir", dataDir}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	payload, err := os.ReadFile(settings)
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}
	if string(payload) != "fixture" {
		t.Fatalf("settings changed to %q", payload)
	}
}

func TestMigrateToolLayoutSeparatesAndVerifiesLegacyData(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	sourceDir := filepath.Join(root, "source", "demo")
	writeMigrationSource(t, sourceDir)
	legacyDir := filepath.Join(dataDir, "tools", "demo")
	writeFixtureFile(t, filepath.Join(legacyDir, "binary", "windows-amd64", "demo.exe"))
	writeFixtureFile(t, filepath.Join(legacyDir, "runtime", "cache.db"))
	if err := os.WriteFile(filepath.Join(legacyDir, "config.json"), []byte(`{"mode":"default"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(config.json) error = %v", err)
	}
	writeLegacyToolDefinition(t, legacyDir)

	if err := migrateToolLayout(context.Background(), dataDir, []toolSource{{ID: "demo", Dir: sourceDir}}); err != nil {
		t.Fatalf("migrateToolLayout() error = %v", err)
	}
	assertFile(t, filepath.Join(dataDir, "tool-bodies", "demo", "binary", "windows-amd64", "demo.exe"))
	assertFile(t, filepath.Join(dataDir, "tool-bodies", "demo", "config.json"))
	assertFile(t, filepath.Join(dataDir, "tool-data", "demo", "runtime", "cache.db"))
	assertFile(t, filepath.Join(dataDir, "tools", "demo", "runtime", "cache.db"))
	if _, err := os.Stat(filepath.Join(dataDir, "tool-bodies", "demo", "runtime")); !os.IsNotExist(err) {
		t.Fatalf("body runtime stat error = %v, want not exist", err)
	}
	definition := readToolDefinitionFile(t, filepath.Join(dataDir, "tool-bodies", "demo", "definition.json"))
	if definition.Version != "0.1.0" || definition.BodyDirectory != "." || len(definition.UserConfig) != 0 || definition.PromptDescriptionOverride != "" {
		t.Fatalf("definition = %#v", definition)
	}
	settings := readToolSettingsFile(t, filepath.Join(dataDir, "tool-data", "demo", "settings.json"))
	if settings.UserConfig["token"] != "secret" || settings.PromptDescriptionOverride != "custom prompt" {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestMigrateToolLayoutRejectsUnknownLegacyPathWithoutActivation(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	sourceDir := filepath.Join(root, "source", "demo")
	writeMigrationSource(t, sourceDir)
	legacyDir := filepath.Join(dataDir, "tools", "demo")
	writeFixtureFile(t, filepath.Join(legacyDir, "binary", "windows-amd64", "demo.exe"))
	writeFixtureFile(t, filepath.Join(legacyDir, "unknown", "state.bin"))
	if err := os.WriteFile(filepath.Join(legacyDir, "config.json"), []byte(`{"mode":"default"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(config.json) error = %v", err)
	}
	writeLegacyToolDefinition(t, legacyDir)

	err := migrateToolLayout(context.Background(), dataDir, []toolSource{{ID: "demo", Dir: sourceDir}})
	if err == nil || !strings.Contains(err.Error(), "unrecognized legacy path") {
		t.Fatalf("migrateToolLayout() error = %v, want unrecognized path", err)
	}
	for _, path := range []string{filepath.Join(dataDir, "tool-bodies"), filepath.Join(dataDir, "tool-data")} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("migration target %s stat error = %v, want not exist", path, statErr)
		}
	}
	assertFile(t, filepath.Join(legacyDir, "unknown", "state.bin"))
}

func TestReadToolDefinitionRequiresDefaultInvocationMode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tool.json"), []byte(`{
  "id": "demo_tool",
  "name": "demo_tool",
  "description": "Demo tool",
  "type": "local"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(tool.json) error = %v", err)
	}

	_, err := readToolDefinition(toolSource{ID: "demo_tool", Dir: dir})
	if err == nil || !strings.Contains(err.Error(), "defaultInvocationMode") {
		t.Fatalf("readToolDefinition() error = %v, want defaultInvocationMode error", err)
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

func readToolSettingsFile(t *testing.T, path string) types.ToolUserSettings {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var settings types.ToolUserSettings
	if err := json.Unmarshal(payload, &settings); err != nil {
		t.Fatalf("Unmarshal(settings) error = %v", err)
	}
	return settings
}

func writeMigrationSource(t *testing.T, sourceDir string) {
	t.Helper()
	definition := types.ToolDefinition{
		ID:                    "demo",
		Name:                  "demo",
		Description:           "Demo tool",
		Version:               "0.1.0",
		EucliBoxCompatibility: types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		DefaultInvocationMode: types.ToolInvocationModeSync,
		Type:                  "local",
		BodyDirectory:         ".",
	}
	if err := writeJSON(filepath.Join(sourceDir, "tool.json"), definition); err != nil {
		t.Fatalf("writeJSON(tool.json) error = %v", err)
	}
	toolpack := toolpackSpec{RuntimeConfig: runtimeConfigSpec{Source: "config.json"}, DataPaths: []string{"runtime"}}
	if err := writeJSON(filepath.Join(sourceDir, "toolpack.json"), toolpack); err != nil {
		t.Fatalf("writeJSON(toolpack.json) error = %v", err)
	}
}

func writeLegacyToolDefinition(t *testing.T, legacyDir string) {
	t.Helper()
	legacy := map[string]any{
		"id":                        "demo",
		"name":                      "demo",
		"description":               "Demo tool",
		"defaultInvocationMode":     "sync",
		"type":                      "local",
		"directory":                 ".",
		"userConfig":                map[string]any{"token": "secret"},
		"promptDescriptionOverride": "custom prompt",
		"binaries":                  []map[string]any{{"goos": "windows", "goarch": "amd64", "path": "binary/windows-amd64/demo.exe"}},
		"createdAt":                 "2026-06-18T06:10:31Z",
		"updatedAt":                 "2026-06-19T06:10:31Z",
	}
	if err := writeJSON(filepath.Join(legacyDir, "data.json"), legacy); err != nil {
		t.Fatalf("writeJSON(data.json) error = %v", err)
	}
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
