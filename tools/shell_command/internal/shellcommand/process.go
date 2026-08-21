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

func runProviderCommand(ctx context.Context, provider selectedProvider, request commandRequest, workdir string, onChunk func(payload []byte)) processResult {
	startedAt := time.Now()
	stdout := newLimitedBuffer(request.MaxOutputChars)
	stderr := newLimitedBuffer(request.MaxOutputChars)
	combined := newLimitedBuffer(request.MaxOutputChars)
	args, err := providerArgs(provider.Config, request.Command)
	if err != nil {
		return processResult{ExitCode: -1, DurationMs: elapsedMs(startedAt), Error: err.Error()}
	}
	commandTimeout := time.NewTimer(time.Duration(request.TimeoutMs) * time.Millisecond)
	defer commandTimeout.Stop()
	cmd := exec.Command(provider.Executable, args...)
	cmd.Dir = workdir
	cmd.Env = providerEnv(provider.Config, os.Environ())
	cmd.Stdout = streamCapture{stream: stdout, combined: combined, onChunk: onChunk}
	cmd.Stderr = streamCapture{stream: stderr, combined: combined, onChunk: onChunk}
	configureProcess(cmd)
	if err := cmd.Start(); err != nil {
		return processResult{ExitCode: -1, DurationMs: elapsedMs(startedAt), Error: fmt.Sprintf("start command: %v", err)}
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	var waitErr error
	timedOut := false
	failureKind := ""
	terminationError := ""
	select {
	case waitErr = <-waitCh:
	case <-commandTimeout.C:
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
