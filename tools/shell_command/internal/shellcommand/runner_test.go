package shellcommand

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func TestExecuteRunsBundledProviderCommand(t *testing.T) {
	fixture := newShellCommandFixture(t)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"command": "print", "workdir": ".", "description": "fixture run"},
		ToolDirectory:        fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", result.Status, result.Error)
	}
	if !strings.Contains(result.Content, "fixture-ok") {
		t.Fatalf("content = %q", result.Content)
	}
	if result.Metadata["provider"] != "git-bash" || result.Metadata["workdir"] != fixture.hostDir || result.Metadata["description"] != "fixture run" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteReturnsFailureWithProcessMetadata(t *testing.T) {
	fixture := newShellCommandFixture(t)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"command": "fail", "timeoutMs": 10000},
		ToolDirectory:        fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusFailed || result.Error != "command exited with code 7" {
		t.Fatalf("result = %#v", result)
	}
	if result.Metadata["exitCode"] != 7 || !strings.Contains(result.Metadata["stderr"].(string), "fixture-error") {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteTimesOutCommand(t *testing.T) {
	fixture := newShellCommandFixture(t)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"command": "sleep", "timeoutMs": 10},
		ToolDirectory:        fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusFailed || result.Metadata["timedOut"] != true {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteTruncatesCapturedOutput(t *testing.T) {
	fixture := newShellCommandFixture(t)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"command": "large", "maxOutputChars": 12},
		ToolDirectory:        fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusSuccess || result.Metadata["truncated"] != true {
		t.Fatalf("result = %#v", result)
	}
	if len([]rune(result.Content)) != 12 {
		t.Fatalf("content length = %d, content = %q", len([]rune(result.Content)), result.Content)
	}
}

func TestExecuteUsesUserConfigDefaults(t *testing.T) {
	fixture := newShellCommandFixture(t)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"command": "large"},
		UserConfig:           map[string]any{"workdir": ".", "description": "configured run", "timeoutMs": 10000, "maxOutputChars": 8},
		ToolDirectory:        fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusSuccess || result.Metadata["description"] != "configured run" || result.Metadata["maxOutputChars"] != 8 {
		t.Fatalf("result = %#v", result)
	}
	if len([]rune(result.Content)) != 8 {
		t.Fatalf("content length = %d, content = %q", len([]rune(result.Content)), result.Content)
	}
}

func TestExecuteFailsWhenBundledProviderMissing(t *testing.T) {
	fixture := newShellCommandFixture(t)
	if err := os.Remove(fixture.providerExe); err != nil {
		t.Fatalf("Remove(provider) error = %v", err)
	}
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"command": "print"},
		ToolDirectory:        fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "bundled executable is missing") {
		t.Fatalf("result = %#v", result)
	}
}

type shellCommandFixture struct {
	toolDir     string
	hostDir     string
	providerExe string
}

func newShellCommandFixture(t *testing.T) shellCommandFixture {
	t.Helper()
	root := t.TempDir()
	toolDir := filepath.Join(root, "tool")
	hostDir := filepath.Join(root, "host")
	providerRel := filepath.Join("providers", "git-bash", executableName("fake-provider"))
	providerExe := filepath.Join(toolDir, providerRel)
	if err := os.MkdirAll(filepath.Dir(providerExe), 0o755); err != nil {
		t.Fatalf("MkdirAll(provider) error = %v", err)
	}
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(host) error = %v", err)
	}
	buildFakeProvider(t, providerExe)
	config := Config{
		DefaultProvider:             "git-bash",
		AllowModelProviderSelection: true,
		Providers:                   []ProviderConfig{{ID: "git-bash", Kind: "git-bash", Mode: "bundled", Enabled: true, Encoding: "utf-8", Executables: []types.ToolBinary{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Path: filepath.ToSlash(providerRel)}}}},
		Limits:                      LimitsConfig{DefaultTimeoutMs: 10000, MaxTimeoutMs: 20000, MaxOutputChars: 40},
	}
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal(config) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "config.json"), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	return shellCommandFixture{toolDir: toolDir, hostDir: hostDir, providerExe: providerExe}
}

func buildFakeProvider(t *testing.T, target string) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(source, []byte(fakeProviderSource), 0o644); err != nil {
		t.Fatalf("WriteFile(fake provider source) error = %v", err)
	}
	cmd := exec.Command("go", "build", "-o", target, source)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build fake provider failed: %v\n%s", err, output)
	}
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

const fakeProviderSource = `package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	command := ""
	if len(os.Args) > 1 {
		command = os.Args[len(os.Args)-1]
	}
	switch command {
	case "print":
		cwd, _ := os.Getwd()
		fmt.Println("fixture-ok", cwd)
	case "fail":
		fmt.Fprintln(os.Stderr, "fixture-error")
		os.Exit(7)
	case "sleep":
		time.Sleep(200 * time.Millisecond)
	case "large":
		fmt.Print(strings.Repeat("界", 30))
	default:
		fmt.Println(command)
	}
}
`
