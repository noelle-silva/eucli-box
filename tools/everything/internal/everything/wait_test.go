package everything

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"testing"
	"time"
)

func noWaitSleep(context.Context, time.Duration) error {
	return nil
}

func TestWaitUntilKeepsWaitingUntilTargetReady(t *testing.T) {
	attempts := 0
	probe := func(context.Context) (bool, error) {
		attempts++
		return attempts >= 3, nil
	}
	if err := waitUntil(context.Background(), waitEnvironment{sleep: noWaitSleep}, probe); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestWaitUntilEndsOnExplicitFailureFact(t *testing.T) {
	failure := errors.New("Everything IPC window not found")
	attempts := 0
	probe := func(context.Context) (bool, error) {
		attempts++
		if attempts == 2 {
			return false, failure
		}
		return false, nil
	}
	err := waitUntil(context.Background(), waitEnvironment{sleep: noWaitSleep}, probe)
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v", err)
	}
}

func TestWaitUntilKeepsWaitingWithoutProgress(t *testing.T) {
	const cycles = 50
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := 0
	probe := func(context.Context) (bool, error) {
		attempts++
		return false, nil
	}
	sleep := func(ctx context.Context, _ time.Duration) error {
		if attempts >= cycles {
			cancel()
		}
		return ctx.Err()
	}
	err := waitUntil(ctx, waitEnvironment{sleep: sleep}, probe)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if attempts < cycles {
		t.Fatalf("attempts = %d, want at least %d", attempts, cycles)
	}
}

func TestWaitUntilEndsAtCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	probe := func(context.Context) (bool, error) {
		t.Fatal("probe must not run after the caller deadline expired")
		return false, nil
	}
	err := waitUntil(ctx, waitEnvironment{sleep: noWaitSleep}, probe)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestSleepWithContextEndsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepWithContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

type fakeExitCodeError struct {
	code int
}

func (e fakeExitCodeError) Error() string { return fmt.Sprintf("exit status %d", e.code) }
func (e fakeExitCodeError) ExitCode() int { return e.code }

func TestHardProbeFailureClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		hard bool
	}{
		{name: "no error", err: nil, hard: false},
		{name: "probe timeout", err: errors.New("Everything search timed out after 5s"), hard: false},
		{name: "busy instance", err: fakeExitCodeError{code: 7}, hard: false},
		{name: "empty result", err: fakeExitCodeError{code: 9}, hard: false},
		{name: "wrapped busy instance", err: fmt.Errorf("probe failed: %w", fakeExitCodeError{code: 7}), hard: false},
		{name: "lost instance", err: fakeExitCodeError{code: 8}, hard: true},
		{name: "broken command line", err: fakeExitCodeError{code: 6}, hard: true},
		{name: "missing executable", err: &exec.Error{Name: "es", Err: exec.ErrNotFound}, hard: true},
		{name: "permission denied", err: &fs.PathError{Op: "fork/exec", Path: "es", Err: fs.ErrPermission}, hard: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hardProbeFailure(tc.err); got != tc.hard {
				t.Fatalf("hardProbeFailure(%v) = %v, want %v", tc.err, got, tc.hard)
			}
		})
	}
}
