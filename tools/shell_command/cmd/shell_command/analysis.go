package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// analyzerTimeout caps a single analyzer invocation. A healthy analyzer is
// sub-second; a hung invocation is a broken deployment that should fail fast
// instead of stalling command execution.
const analyzerTimeout = 15 * time.Second

// analyzerRuntimeVersion is the analyzer release version; it mirrors
// tools/shell_command/analyzer/Cargo.toml (kept in sync). It names the
// development runtime slot under .dev-workspace/.dev-tools-runtime,
// per the development asset layout norm.
const analyzerRuntimeVersion = "0.1.0"

// analyzerRuntimeDir is relative to the repository root.
const analyzerRuntimeDir = ".dev-workspace/.dev-tools-runtime/shell-command-analyzer"

// commandAnalyzer drives the Rust unified command analyzer shipped with the
// shell_command tool. The analyzer binary lives at
// <tool body directory>/analyzer/<executable> and reasons about command
// classification, semantics, read-only flag verification and impact paths.
type commandAnalyzer struct {
	exe string
}

// newCommandAnalyzer resolves the analyzer executable:
//  1. inside the tool body (<body>/analyzer/) — the packaged layout used by
//     released tool bundles (toolpack asset root), and by verification
//     fixtures;
//  2. in the development runtime slot under .dev-workspace/.dev-tools-runtime
//     — built by tools/shell_command/analyzer/build.cmd for development runs
//     against the source tree body.
//
// Missing analyzer in both places is a deployment defect: the analyzer is part
// of the tool, so the tool fails fast instead of running without analysis.
func newCommandAnalyzer(toolBodyDirectory string) (*commandAnalyzer, error) {
	if toolBodyDirectory == "" {
		return nil, fmt.Errorf("tool body directory is required")
	}
	name := "command-analyzer"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bodyExe := filepath.Join(toolBodyDirectory, "analyzer", name)
	if info, err := os.Stat(bodyExe); err == nil && !info.IsDir() {
		return &commandAnalyzer{exe: bodyExe}, nil
	}
	if runtimeExe := developmentRuntimeAnalyzer(toolBodyDirectory, name); runtimeExe != "" {
		return &commandAnalyzer{exe: runtimeExe}, nil
	}
	return nil, fmt.Errorf(
		"command analyzer is not available: expected %s or the development runtime slot %s/%s; build it with tools/shell_command/analyzer/build.cmd",
		bodyExe, filepath.ToSlash(analyzerRuntimeDir), analyzerRuntimeVersion,
	)
}

// developmentRuntimeAnalyzer locates the development runtime slot by walking
// up from the tool body to the repository root (go.mod marker).
func developmentRuntimeAnalyzer(toolBodyDirectory string, name string) string {
	current := filepath.Clean(toolBodyDirectory)
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			candidate := filepath.Join(
				current,
				filepath.FromSlash(analyzerRuntimeDir),
				analyzerRuntimeVersion,
				name,
			)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
			return ""
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

// analyzerShell maps a shell provider id onto the analyzer's shell grammar.
func analyzerShell(providerID string) string {
	switch strings.TrimSpace(providerID) {
	case "git-bash":
		return "bash"
	case "powershell":
		return "powershell"
	default:
		return "unknown"
	}
}

// analyze runs the analyzer over the requested command. It returns the parsed
// analysis report as a generic value suitable for tool result metadata.
func (a *commandAnalyzer) analyze(ctx context.Context, command string, providerID string, workdir string) (map[string]any, error) {
	request := map[string]any{
		"version": 1,
		"command": command,
		"shell":   analyzerShell(providerID),
		"workdir": workdir,
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("serialize analyzer request: %w", err)
	}

	analyzeCtx, cancel := context.WithTimeout(ctx, analyzerTimeout)
	defer cancel()

	cmd := exec.CommandContext(analyzeCtx, a.exe)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("command analyzer failed: %w: %s", err, stderr.String())
	}

	var report map[string]any
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.UseNumber()
	if err := decoder.Decode(&report); err != nil {
		return nil, fmt.Errorf("decode analyzer report: %w", err)
	}
	return report, nil
}
