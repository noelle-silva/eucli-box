package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
	if len(tool.Binaries) != 1 || tool.Binaries[0].Path != filepath.ToSlash(binaryRelPath) || filepath.IsAbs(tool.Binaries[0].Path) {
		t.Fatalf("binaries = %#v", tool.Binaries)
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
