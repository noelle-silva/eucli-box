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
		ToolBodyDirectory:    fixture.toolDir,
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
		ToolBodyDirectory:    fixture.toolDir,
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
		ToolBodyDirectory:    fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusFailed || result.Metadata["timedOut"] != true {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteTruncatesCapturedOutput(t *testing.T) {
	fixture := newShellCommandFixture(t)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"command": "ordered-large", "maxOutputChars": 12},
		ToolBodyDirectory:    fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusSuccess || result.Metadata["truncated"] != true {
		t.Fatalf("result = %#v", result)
	}
	// head+tail elision: 20 chars against charLimit 12 keeps first 6 and last 6.
	expected := "012345…8 chars truncated…EFGHIJ"
	if result.Content != expected || result.Metadata["stdout"] != expected || result.Metadata["combinedOutput"] != expected {
		t.Fatalf("elided output mismatch: content = %q, metadata = %#v", result.Content, result.Metadata)
	}
	if result.Metadata["outputBytesTotal"] != int64(20) || result.Metadata["outputLines"] != int64(0) {
		t.Fatalf("output facts = %#v", result.Metadata)
	}
	if result.Metadata["invalidUTF8"] != false || result.Metadata["utf8ReplacementCount"] != 0 {
		t.Fatalf("encoding metadata = %#v", result.Metadata)
	}
}

func TestExecuteDeniesHardlineCommandBeforeProviderSelection(t *testing.T) {
	fixture := newShellCommandFixture(t)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"command": "rm -rf /", "provider": "missing-provider"},
		ToolBodyDirectory:    fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusDenied {
		t.Fatalf("result = %#v", result)
	}
	if result.Metadata["hardlineBlocked"] != true || result.Metadata["denied"] != true || result.Metadata["hardlineRule"] == "" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
	if !strings.Contains(result.Error, "hardline safety rule") || strings.Contains(result.Error, "missing-provider") {
		t.Fatalf("error = %q", result.Error)
	}
}

func TestExecuteDeniesHardlineCommandBeforeWorkdirResolution(t *testing.T) {
	fixture := newShellCommandFixture(t)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"command": "shutdown -h now", "workdir": filepath.Join(fixture.hostDir, "missing")},
		ToolBodyDirectory:    fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusDenied {
		t.Fatalf("result = %#v", result)
	}
	if result.Content != result.Error || result.Metadata["error"] != result.Error || result.Metadata["hardlineBlocked"] != true {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteDoesNotMarkByteTruncationAsInvalidUTF8(t *testing.T) {
	fixture := newShellCommandFixture(t)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"command": "partial-byte-truncate", "maxOutputChars": 1},
		ToolBodyDirectory:    fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusSuccess || result.Metadata["truncated"] != true {
		t.Fatalf("result = %#v", result)
	}
	if result.Metadata["invalidUTF8"] != false || result.Metadata["utf8ReplacementCount"] != 0 {
		t.Fatalf("encoding metadata = %#v", result.Metadata)
	}
	if !strings.Contains(result.Content, "chars truncated") {
		t.Fatalf("content should show elision marker: %q", result.Content)
	}
}

func TestExecuteMarksInvalidUTF8Output(t *testing.T) {
	fixture := newShellCommandFixture(t)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"command": "invalid-utf8"},
		ToolBodyDirectory:    fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", result.Status, result.Error)
	}
	if result.Metadata["invalidUTF8"] != true || result.Metadata["utf8ReplacementCount"] != 1 || result.Metadata["stdoutInvalidUTF8"] != true {
		t.Fatalf("encoding metadata = %#v", result.Metadata)
	}
	if !strings.Contains(result.Content, "Command output contained non-UTF-8 bytes") || !strings.Contains(result.Content, "?ok") {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestExecuteUsesUserConfigDefaults(t *testing.T) {
	fixture := newShellCommandFixture(t)
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"command": "large"},
		UserConfig:           map[string]any{"workdir": ".", "description": "configured run", "timeoutMs": 10000, "maxOutputChars": 8},
		ToolBodyDirectory:    fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusSuccess || result.Metadata["description"] != "configured run" || result.Metadata["maxOutputChars"] != 8 {
		t.Fatalf("result = %#v", result)
	}
	// 30 runes against charLimit 8: elided to first 4 and last 4 plus marker.
	content := result.Content
	if !strings.HasPrefix(content, "界界界界…") || !strings.HasSuffix(content, "…界界界界") {
		t.Fatalf("content = %q", content)
	}
}

func TestExecuteFailsWhenBundledProviderMissing(t *testing.T) {
	fixture := newShellCommandFixture(t)
	if err := os.Remove(fixture.providerExe); err != nil {
		t.Fatalf("Remove(provider) error = %v", err)
	}
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:            map[string]any{"command": "print"},
		ToolBodyDirectory:    fixture.toolDir,
		HostWorkingDirectory: fixture.hostDir,
	})
	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "bundled executable is missing") {
		t.Fatalf("result = %#v", result)
	}
}

func TestProviderEnvAddsUTF8RuntimeHints(t *testing.T) {
	env := providerEnv(ProviderConfig{Encoding: "utf-8"}, []string{"EXISTING=value", "LANG=legacy", "PythonIoEncoding=gbk"})
	for _, expected := range []string{"EXISTING=value", "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "PYTHONIOENCODING=utf-8"} {
		if !containsEnv(env, expected) {
			t.Fatalf("env missing %q: %#v", expected, env)
		}
	}
	for _, unexpected := range []string{"LANG=legacy", "PythonIoEncoding=gbk"} {
		if containsEnv(env, unexpected) {
			t.Fatalf("env contains stale entry %q: %#v", unexpected, env)
		}
	}
}

func containsEnv(env []string, expected string) bool {
	for _, entry := range env {
		if entry == expected {
			return true
		}
	}
	return false
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
		fmt.Println(cwd, "fixture-ok")
	case "fail":
		fmt.Fprintln(os.Stderr, "fixture-error")
		os.Exit(7)
	case "sleep":
		time.Sleep(200 * time.Millisecond)
	case "large":
		fmt.Print(strings.Repeat("界", 30))
	case "ordered-large":
		fmt.Print("0123456789ABCDEFGHIJ")
	case "partial-byte-truncate":
		fmt.Print("界界")
	case "invalid-utf8":
		_, _ = os.Stdout.Write([]byte{0xff, 'o', 'k'})
	default:
		fmt.Println(command)
	}
}
`
