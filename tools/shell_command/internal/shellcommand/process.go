package shellcommand

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type processResult struct {
	Stdout         string
	Stderr         string
	CombinedOutput string
	ExitCode       int
	TimedOut       bool
	DurationMs     int64
	Truncated      bool
	Error          string
}

func runProviderCommand(ctx context.Context, provider selectedProvider, request commandRequest, workdir string) processResult {
	startedAt := time.Now()
	stdout := newLimitedBuffer(request.MaxOutputChars)
	stderr := newLimitedBuffer(request.MaxOutputChars)
	combined := newLimitedBuffer(request.MaxOutputChars)
	args, err := providerArgs(provider.Config, request.Command)
	if err != nil {
		return processResult{ExitCode: -1, DurationMs: elapsedMs(startedAt), Error: err.Error()}
	}
	processCtx, cancel := context.WithTimeout(ctx, time.Duration(request.TimeoutMs)*time.Millisecond)
	defer cancel()
	cmd := exec.Command(provider.Executable, args...)
	cmd.Dir = workdir
	cmd.Env = providerEnv(provider.Config, os.Environ())
	cmd.Stdout = streamCapture{stream: stdout, combined: combined}
	cmd.Stderr = streamCapture{stream: stderr, combined: combined}
	configureProcess(cmd)
	if err := cmd.Start(); err != nil {
		return processResult{ExitCode: -1, DurationMs: elapsedMs(startedAt), Error: fmt.Sprintf("start command: %v", err)}
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	var waitErr error
	timedOut := false
	select {
	case waitErr = <-waitCh:
	case <-processCtx.Done():
		timedOut = errors.Is(processCtx.Err(), context.DeadlineExceeded)
		if cmd.Process != nil {
			_ = terminateProcessTree(cmd.Process.Pid)
		}
		select {
		case waitErr = <-waitCh:
		case <-time.After(5 * time.Second):
			waitErr = fmt.Errorf("process did not exit after termination")
		}
	}
	exitCode := 0
	if waitErr != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	stdoutText, stdoutTruncated := stdout.Snapshot()
	stderrText, stderrTruncated := stderr.Snapshot()
	combinedText, combinedTruncated := combined.Snapshot()
	errorMessage := ""
	if timedOut {
		errorMessage = fmt.Sprintf("command timed out after %dms", request.TimeoutMs)
	} else if waitErr != nil && exitCode == -1 {
		errorMessage = waitErr.Error()
	}
	return processResult{
		Stdout:         stdoutText,
		Stderr:         stderrText,
		CombinedOutput: combinedText,
		ExitCode:       exitCode,
		TimedOut:       timedOut,
		DurationMs:     elapsedMs(startedAt),
		Truncated:      stdoutTruncated || stderrTruncated || combinedTruncated,
		Error:          errorMessage,
	}
}

func elapsedMs(startedAt time.Time) int64 {
	return time.Since(startedAt).Milliseconds()
}
