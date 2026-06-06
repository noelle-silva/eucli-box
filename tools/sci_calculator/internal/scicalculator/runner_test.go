package scicalculator

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

func TestExecuteRunsCalculatorThroughPython(t *testing.T) {
	python := requireScientificPython(t)
	fixture := newSciCalculatorFixture(t)

	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:     map[string]any{"expression": "sqrt(variance([2,4,4,4,5,5,7,9])) + integral('exp(-x**2)', '-inf', 'inf')"},
		UserConfig:    map[string]any{"pythonExecutable": python},
		DefaultConfig: map[string]any{"pythonExecutable": "python", "maxOutputChars": 20000},
		ToolDirectory: fixture.toolDir,
	})

	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Content, "###计算结果：") || !strings.Contains(result.Content, "###，请将结果转告用户") {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestExecuteReportsCalculatorErrors(t *testing.T) {
	python := requireScientificPython(t)
	fixture := newSciCalculatorFixture(t)

	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:     map[string]any{"expression": "1/0"},
		UserConfig:    map[string]any{"pythonExecutable": python},
		DefaultConfig: map[string]any{"pythonExecutable": "python", "maxOutputChars": 20000},
		ToolDirectory: fixture.toolDir,
	})

	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "Division by zero") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteTruncatesCalculatorOutput(t *testing.T) {
	fixture := newSciCalculatorFixture(t)
	fakePython := buildFakePython(t)

	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:     map[string]any{"expression": "large", "maxOutputChars": 12},
		UserConfig:    map[string]any{"pythonExecutable": fakePython},
		DefaultConfig: map[string]any{"pythonExecutable": "python", "maxOutputChars": 20000},
		ToolDirectory: fixture.toolDir,
	})

	if result.Status != types.ToolStatusSuccess || result.Metadata["truncated"] != true {
		t.Fatalf("result = %#v", result)
	}
	if len([]rune(result.Content)) != 12 {
		t.Fatalf("content length = %d, content = %q", len([]rune(result.Content)), result.Content)
	}
}

func TestExecutePrefersBundledPythonRuntime(t *testing.T) {
	fixture := newSciCalculatorFixture(t)
	fakePython := buildFakePython(t)
	configuredPython := buildFakePython(t)
	bundledPython := filepath.Join(fixture.toolDir, "runtime", "python", executableName("python"))
	if err := os.MkdirAll(filepath.Dir(bundledPython), 0o755); err != nil {
		t.Fatalf("MkdirAll(bundled python) error = %v", err)
	}
	if err := os.Rename(fakePython, bundledPython); err != nil {
		t.Fatalf("Rename(fake python) error = %v", err)
	}

	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:     map[string]any{"expression": "large", "maxOutputChars": 12},
		UserConfig:    map[string]any{"pythonExecutable": configuredPython},
		DefaultConfig: map[string]any{"pythonExecutable": "missing-python-command", "maxOutputChars": 20000},
		ToolDirectory: fixture.toolDir,
	})

	if result.Status != types.ToolStatusSuccess || result.Metadata["pythonExecutable"] != bundledPython {
		t.Fatalf("result = %#v, bundledPython = %q", result, bundledPython)
	}
}

func TestExecuteRejectsEscapingBundledPythonPath(t *testing.T) {
	fixture := newSciCalculatorFixtureWithConfig(t, Config{
		PythonEnv:               "SCICALCULATOR_PYTHON",
		BundledPythonExecutable: "../python.exe",
		DefaultPythonExecutable: "python",
		Limits:                  LimitsConfig{MaxOutputChars: 20000, MaxExpressionChars: 10000},
	})

	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:     map[string]any{"expression": "1+1"},
		DefaultConfig: map[string]any{"pythonExecutable": "python", "maxOutputChars": 20000},
		ToolDirectory: fixture.toolDir,
	})

	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "must stay inside tool directory") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteRequiresExpression(t *testing.T) {
	fixture := newSciCalculatorFixture(t)

	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:     map[string]any{},
		DefaultConfig: map[string]any{"pythonExecutable": "python", "maxOutputChars": 20000},
		ToolDirectory: fixture.toolDir,
	})

	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "expression") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteStopsCalculatorWhenContextCancels(t *testing.T) {
	fixture := newSciCalculatorFixture(t)
	fakePython := buildFakePython(t)
	markerPath := filepath.Join(t.TempDir(), "calculator-child-survived.txt")
	t.Setenv("SCICALCULATOR_FAKE_MARKER", markerPath)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	started := time.Now()
	result := Execute(ctx, types.ToolExecutionInput{
		Arguments:     map[string]any{"expression": "sleep"},
		UserConfig:    map[string]any{"pythonExecutable": fakePython},
		DefaultConfig: map[string]any{"pythonExecutable": "python", "maxOutputChars": 20000},
		ToolDirectory: fixture.toolDir,
	})

	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, context.DeadlineExceeded.Error()) {
		t.Fatalf("result = %#v", result)
	}
	if time.Since(started) > 6*time.Second {
		t.Fatalf("calculator waited for the fake process instead of cancelling it")
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("calculator child process was not stopped; marker stat error = %v", err)
	}
}

type sciCalculatorFixture struct {
	toolDir string
}

func newSciCalculatorFixture(t *testing.T) sciCalculatorFixture {
	t.Helper()
	return newSciCalculatorFixtureWithConfig(t, Config{
		PythonEnv:               "SCICALCULATOR_PYTHON",
		BundledPythonExecutable: "runtime/python/python.exe",
		DefaultPythonExecutable: "python",
		Limits:                  LimitsConfig{MaxOutputChars: 20000, MaxExpressionChars: 10000},
	})
}

func newSciCalculatorFixtureWithConfig(t *testing.T, config Config) sciCalculatorFixture {
	t.Helper()
	toolDir := t.TempDir()
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal(config) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "config.json"), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	return sciCalculatorFixture{toolDir: toolDir}
}

func requireScientificPython(t *testing.T) string {
	t.Helper()
	python := strings.TrimSpace(os.Getenv("SCICALCULATOR_PYTHON"))
	if python == "" {
		python = "python"
	}
	cmd := exec.Command(python, "-c", "import sympy, scipy, numpy")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("scientific Python dependencies are unavailable: %v\n%s", err, output)
	}
	return python
}

func buildFakePython(t *testing.T) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), executableName("fake-python"))
	source := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(source, []byte(fakePythonSource), 0o644); err != nil {
		t.Fatalf("WriteFile(fake python source) error = %v", err)
	}
	cmd := exec.Command("go", "build", "-o", target, source)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build fake python failed: %v\n%s", err, output)
	}
	return target
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

const fakePythonSource = `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	expression := ""
	if scanner.Scan() {
		expression = scanner.Text()
	}
	if strings.TrimSpace(expression) == "large" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"status":"success", "result":strings.Repeat("界", 30)})
		return
	}
	if strings.TrimSpace(expression) == "sleep" {
		time.Sleep(time.Second)
		if marker := os.Getenv("SCICALCULATOR_FAKE_MARKER"); marker != "" {
			_ = os.WriteFile(marker, []byte("survived"), 0o644)
		}
		time.Sleep(30 * time.Second)
		return
	}
	fmt.Fprintln(os.Stdout, ` + "`" + `{"status":"success","result":"ok"}` + "`" + `)
}
`
