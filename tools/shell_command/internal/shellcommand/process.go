package shellcommand

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

type processResult struct {
	Stdout                       string
	Stderr                       string
	CombinedOutput               string
	ExitCode                     int
	TimedOut                     bool
	DurationMs                   int64
	Truncated                    bool
	StdoutBytes                  int64
	StderrBytes                  int64
	CombinedBytes                int64
	StdoutLines                  int64
	StderrLines                  int64
	CombinedLines                int64
	InvalidUTF8                  bool
	UTF8ReplacementCount         int
	StdoutInvalidUTF8            bool
	StderrInvalidUTF8            bool
	StdoutUTF8ReplacementCount   int
	StderrUTF8ReplacementCount   int
	CombinedInvalidUTF8          bool
	CombinedUTF8ReplacementCount int
	FailureKind                  string
	TerminationError             string
	Error                        string
}

const providerTerminationWait = 5 * time.Second

// outputDrainTimeout bounds how long termination waits for leftover pipe data
// (Codex IO_DRAIN_TIMEOUT_MS). Child processes can hold the pipe write end
// after the provider exits; without this bound the tool would block forever.
const outputDrainTimeout = 2 * time.Second

func runProviderCommand(ctx context.Context, provider selectedProvider, request commandRequest, workdir string, onChunk func(payload []byte)) processResult {
	startedAt := time.Now()
	stdout := newLimitedBuffer(request.MaxOutputChars)
	stderr := newLimitedBuffer(request.MaxOutputChars)
	combined := newLimitedBuffer(request.MaxOutputChars)
	args, err := providerArgs(provider.Config, request.Command)
	if err != nil {
		return processResult{ExitCode: -1, DurationMs: elapsedMs(startedAt), Error: err.Error()}
	}
	var commandTimeout <-chan time.Time
	if request.TimeoutMs > 0 {
		timer := time.NewTimer(time.Duration(request.TimeoutMs) * time.Millisecond)
		defer timer.Stop()
		commandTimeout = timer.C
	}
	cmd := exec.Command(provider.Executable, args...)
	cmd.Dir = workdir
	cmd.Env = providerEnv(provider.Config, os.Environ())
	configureProcess(cmd)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return processResult{ExitCode: -1, DurationMs: elapsedMs(startedAt), Error: fmt.Sprintf("stdout pipe: %v", err)}
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return processResult{ExitCode: -1, DurationMs: elapsedMs(startedAt), Error: fmt.Sprintf("stderr pipe: %v", err)}
	}
	if err := cmd.Start(); err != nil {
		return processResult{ExitCode: -1, DurationMs: elapsedMs(startedAt), Error: fmt.Sprintf("start command: %v", err)}
	}
	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		_, _ = io.Copy(streamCapture{stream: stdout, combined: combined, onChunk: onChunk}, stdoutPipe)
	}()
	go func() {
		defer close(stderrDone)
		_, _ = io.Copy(streamCapture{stream: stderr, combined: combined, onChunk: onChunk}, stderrPipe)
	}()
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	var waitErr error
	timedOut := false
	failureKind := ""
	terminationError := ""
	select {
	case waitErr = <-waitCh:
	case <-commandTimeout:
		if ctx.Err() != nil {
			waitErr, terminationError = terminateAndWait(cmd, waitCh)
			break
		}
		timedOut = true
		failureKind = "command_timeout"
		waitErr, terminationError = terminateAndWait(cmd, waitCh)
	case <-ctx.Done():
		waitErr, terminationError = terminateAndWait(cmd, waitCh)
	}
	_ = waitPipeDrained(stdoutDone, stderrDone, outputDrainTimeout)
	exitCode := 0
	if waitErr != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	if timedOut {
		exitCode = 124
	}
	stdoutText := stdout.Snapshot()
	stderrText := stderr.Snapshot()
	combinedText := combined.Snapshot()
	invalidUTF8 := stdoutText.InvalidUTF8 || stderrText.InvalidUTF8 || combinedText.InvalidUTF8
	utf8ReplacementCount := max(stdoutText.ReplacementCount+stderrText.ReplacementCount, combinedText.ReplacementCount)
	errorMessage := ""
	if timedOut {
		errorMessage = fmt.Sprintf("command timed out after %dms", request.TimeoutMs)
	} else if waitErr != nil && exitCode == -1 {
		errorMessage = waitErr.Error()
	}
	return processResult{
		Stdout:                       stdoutText.Text,
		Stderr:                       stderrText.Text,
		CombinedOutput:               combinedText.Text,
		ExitCode:                     exitCode,
		TimedOut:                     timedOut,
		DurationMs:                   elapsedMs(startedAt),
		Truncated:                    stdoutText.Truncated || stderrText.Truncated || combinedText.Truncated,
		StdoutBytes:                  stdoutText.OriginalBytes,
		StderrBytes:                  stderrText.OriginalBytes,
		CombinedBytes:                combinedText.OriginalBytes,
		StdoutLines:                  stdoutText.TotalLines,
		StderrLines:                  stderrText.TotalLines,
		CombinedLines:                combinedText.TotalLines,
		InvalidUTF8:                  invalidUTF8,
		UTF8ReplacementCount:         utf8ReplacementCount,
		StdoutInvalidUTF8:            stdoutText.InvalidUTF8,
		StderrInvalidUTF8:            stderrText.InvalidUTF8,
		StdoutUTF8ReplacementCount:   stdoutText.ReplacementCount,
		StderrUTF8ReplacementCount:   stderrText.ReplacementCount,
		CombinedInvalidUTF8:          combinedText.InvalidUTF8,
		CombinedUTF8ReplacementCount: combinedText.ReplacementCount,
		FailureKind:                  failureKind,
		TerminationError:             terminationError,
		Error:                        errorMessage,
	}
}

// terminateAndWait kills the provider process tree and waits for its exit.
// Both timeout and external cancellation share this single exit.
func terminateAndWait(cmd *exec.Cmd, waitCh <-chan error) (error, string) {
	terminationError := ""
	if cmd.Process != nil {
		if err := terminateProcessTree(cmd.Process.Pid); err != nil {
			terminationError = err.Error()
		}
	}
	select {
	case waitErr := <-waitCh:
		return waitErr, ""
	case <-time.After(providerTerminationWait):
		if terminationError == "" {
			terminationError = "process did not exit after termination"
		}
		return fmt.Errorf("%s", terminationError), terminationError
	}
}

func elapsedMs(startedAt time.Time) int64 {
	return time.Since(startedAt).Milliseconds()
}

// waitPipeDrained waits until both stream readers hit EOF, or until timeout.
// A natural EOF ends the drain early; an open pipe (a descendant still holds
// the write end) gives up after outputDrainTimeout instead of hanging the tool.
func waitPipeDrained(stdoutDone chan struct{}, stderrDone chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-stdoutDone:
	case <-timer.C:
		return false
	}
	select {
	case <-stderrDone:
		return true
	case <-timer.C:
		return false
	}
}
