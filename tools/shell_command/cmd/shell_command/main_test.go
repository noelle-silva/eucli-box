package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func TestExecutableRunsProviderWithUTF8EnvironmentAndOutput(t *testing.T) {
	fixture := newExecutableFixture(t)

	envResult := fixture.run(t, map[string]any{"command": "env"})
	if envResult.Status != types.ToolStatusSuccess {
		t.Fatalf("env status = %s, error = %s", envResult.Status, envResult.Error)
	}
	for _, expected := range []string{"LANG=C.UTF-8", "LC_ALL=C.UTF-8", "PYTHONIOENCODING=utf-8"} {
		if !strings.Contains(envResult.Content, expected) {
			t.Fatalf("env content missing %q: %q", expected, envResult.Content)
		}
	}
	for _, unexpected := range []string{"LANG=legacy", "LC_ALL=legacy", "PYTHONIOENCODING=gbk"} {
		if strings.Contains(envResult.Content, unexpected) {
			t.Fatalf("env content contains stale value %q: %q", unexpected, envResult.Content)
		}
	}
	if envResult.Metadata["invalidUTF8"] != false {
		t.Fatalf("env encoding metadata = %#v", envResult.Metadata)
	}
	t.Logf("provider environment output:\n%s", envResult.Content)

	utf8Result := fixture.run(t, map[string]any{"command": "print-utf8"})
	if utf8Result.Status != types.ToolStatusSuccess {
		t.Fatalf("utf8 status = %s, error = %s", utf8Result.Status, utf8Result.Error)
	}
	if !strings.Contains(utf8Result.Content, "中文-ok") || utf8Result.Metadata["invalidUTF8"] != false {
		t.Fatalf("utf8 result = %#v", utf8Result)
	}
	t.Logf("utf8 command output: %s", utf8Result.Content)
}

func TestExecutableMarksInvalidProviderOutput(t *testing.T) {
	fixture := newExecutableFixture(t)
	result := fixture.run(t, map[string]any{"command": "invalid-utf8"})
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", result.Status, result.Error)
	}
	if result.Metadata["invalidUTF8"] != true || result.Metadata["stdoutInvalidUTF8"] != true {
		t.Fatalf("encoding metadata = %#v", result.Metadata)
	}
	if count, ok := result.Metadata["utf8ReplacementCount"].(float64); !ok || count != 1 {
		t.Fatalf("replacement count metadata = %#v", result.Metadata)
	}
	if !strings.Contains(result.Content, "Command output contained non-UTF-8 bytes") || !strings.Contains(result.Content, "?ok") {
		t.Fatalf("content = %q", result.Content)
	}
	t.Logf("invalid utf8 command output: %s", result.Content)
}

type executableFixture struct {
	executable string
	toolDir    string
	hostDir    string
}

func newExecutableFixture(t *testing.T) executableFixture {
	t.Helper()
	root := t.TempDir()
	executable := filepath.Join(root, executableName("shell_command"))
	buildShellCommandExecutable(t, executable)

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
	buildFakeProviderExecutable(t, providerExe)
	writeToolConfig(t, toolDir, providerRel)
	return executableFixture{executable: executable, toolDir: toolDir, hostDir: hostDir}
}

func (f executableFixture) run(t *testing.T, arguments map[string]any) types.ToolExecutionOutput {
	t.Helper()
	input := types.ToolExecutionInput{ActionID: "action-test", ToolName: "shell_command", Arguments: arguments, ToolDirectory: f.toolDir, HostWorkingDirectory: f.hostDir}
	payload, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal(input) error = %v", err)
	}
	cmd := exec.Command(f.executable)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = append(os.Environ(), "LANG=legacy", "LC_ALL=legacy", "PYTHONIOENCODING=gbk")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shell_command executable failed: %v\n%s", err, output)
	}
	var result types.ToolExecutionOutput
	if err := json.Unmarshal(bytes.TrimSpace(output), &result); err != nil {
		t.Fatalf("Unmarshal(tool output) error = %v\n%s", err, output)
	}
	return result
}

func buildShellCommandExecutable(t *testing.T, target string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", target, ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build shell_command failed: %v\n%s", err, output)
	}
}

func buildFakeProviderExecutable(t *testing.T, target string) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(source, []byte(fakeProviderExecutableSource), 0o644); err != nil {
		t.Fatalf("WriteFile(fake provider source) error = %v", err)
	}
	cmd := exec.Command("go", "build", "-o", target, source)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build fake provider failed: %v\n%s", err, output)
	}
}

func writeToolConfig(t *testing.T, toolDir string, providerRel string) {
	t.Helper()
	config := map[string]any{
		"defaultProvider":             "git-bash",
		"allowModelProviderSelection": true,
		"providers": []map[string]any{{
			"id":       "git-bash",
			"kind":     "git-bash",
			"mode":     "bundled",
			"enabled":  true,
			"priority": 10,
			"encoding": "utf-8",
			"executables": []types.ToolBinary{{
				GOOS:   runtime.GOOS,
				GOARCH: runtime.GOARCH,
				Path:   filepath.ToSlash(providerRel),
			}},
		}},
		"limits": map[string]any{
			"defaultTimeoutMs": 10000,
			"maxTimeoutMs":     20000,
			"maxOutputChars":   200,
		},
	}
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal(config) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "config.json"), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

const fakeProviderExecutableSource = `package main

import (
	"fmt"
	"os"
)

func main() {
	command := ""
	if len(os.Args) > 1 {
		command = os.Args[len(os.Args)-1]
	}
	switch command {
	case "env":
		fmt.Printf("LANG=%s\nLC_ALL=%s\nPYTHONIOENCODING=%s", os.Getenv("LANG"), os.Getenv("LC_ALL"), os.Getenv("PYTHONIOENCODING"))
	case "print-utf8":
		fmt.Print("中文-ok")
	case "invalid-utf8":
		_, _ = os.Stdout.Write([]byte{0xff, 'o', 'k'})
	default:
		fmt.Print(command)
	}
}
`
