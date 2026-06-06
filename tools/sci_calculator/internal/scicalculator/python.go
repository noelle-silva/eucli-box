package scicalculator

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

//go:embed calculator.py
var calculatorScript string

type calculatorRunResult struct {
	Status     types.ToolStatus
	Content    string
	Error      string
	Stderr     string
	ExitCode   int
	DurationMs int64
}

type calculatorJSONOutput struct {
	Status string `json:"status"`
	Result string `json:"result"`
	Error  string `json:"error"`
}

func runPythonCalculator(ctx context.Context, toolDir string, request calculationRequest) (calculatorRunResult, error) {
	tempDir, err := os.MkdirTemp("", "eucli-scicalculator-")
	if err != nil {
		return calculatorRunResult{}, fmt.Errorf("create temporary calculator directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	scriptPath := filepath.Join(tempDir, "calculator.py")
	if err := os.WriteFile(scriptPath, []byte(calculatorScript), 0o600); err != nil {
		return calculatorRunResult{}, fmt.Errorf("write calculator script: %w", err)
	}

	started := time.Now()
	cmd := exec.CommandContext(ctx, request.PythonExecutable, scriptPath)
	cmd.Dir = toolDir
	cmd.Stdin = strings.NewReader(request.Expression + "\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	durationMs := time.Since(started).Milliseconds()
	result := calculatorRunResult{Stderr: stderr.String(), ExitCode: exitCode(runErr), DurationMs: durationMs}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, ctxErr
	}

	rawOutput := bytes.TrimSpace(stdout.Bytes())
	if len(rawOutput) == 0 {
		message := "calculator produced no output"
		if runErr != nil {
			message = processErrorMessage(runErr, result.Stderr)
		}
		result.Status = types.ToolStatusFailed
		result.Content = message
		result.Error = message
		return result, nil
	}

	var output calculatorJSONOutput
	if err := json.Unmarshal(rawOutput, &output); err != nil {
		if runErr != nil {
			message := processErrorMessage(runErr, result.Stderr)
			result.Status = types.ToolStatusFailed
			result.Content = message
			result.Error = message
			return result, nil
		}
		return result, invalidCalculatorOutput("not valid json")
	}

	switch output.Status {
	case "success":
		if runErr != nil {
			result.Status = types.ToolStatusFailed
			result.Content = processErrorMessage(runErr, result.Stderr)
			result.Error = result.Content
			return result, nil
		}
		if strings.TrimSpace(output.Result) == "" {
			return result, invalidCalculatorOutput("success result is empty")
		}
		result.Status = types.ToolStatusSuccess
		result.Content = output.Result
		return result, nil
	case "error":
		message := strings.TrimSpace(output.Error)
		if message == "" {
			message = strings.TrimSpace(output.Result)
		}
		if message == "" {
			message = "calculator returned an empty error"
		}
		result.Status = types.ToolStatusFailed
		result.Content = message
		result.Error = message
		return result, nil
	default:
		return result, invalidCalculatorOutput("status must be success or error")
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func processErrorMessage(err error, stderr string) string {
	message := err.Error()
	if strings.TrimSpace(stderr) != "" {
		message = message + ": " + strings.TrimSpace(stderr)
	}
	return message
}
