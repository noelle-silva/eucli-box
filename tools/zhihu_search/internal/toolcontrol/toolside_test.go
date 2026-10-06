package toolcontrol

import (
	"math"
	"testing"
	"time"
)

func TestExecutionContextWithoutTimeoutSetsNoDeadline(t *testing.T) {
	ctx, cancel := ExecutionContext(0)
	defer cancel()
	if _, ok := ctx.Deadline(); ok {
		t.Fatal("zero timeout must not set a deadline")
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("zero timeout context must start alive, got: %v", err)
	}
	cancel()
	if err := ctx.Err(); err == nil {
		t.Fatal("cancelling the zero timeout context must cancel it")
	}
}

func TestExecutionContextNegativeTimeoutSetsNoDeadlineWithoutPanic(t *testing.T) {
	for _, timeoutMs := range []int64{-1, -math.MaxInt64, math.MinInt64} {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("ExecutionContext(%d) panicked: %v", timeoutMs, recovered)
				}
			}()
			ctx, cancel := ExecutionContext(timeoutMs)
			defer cancel()
			if _, ok := ctx.Deadline(); ok {
				t.Fatalf("ExecutionContext(%d) must not set a deadline", timeoutMs)
			}
			if err := ctx.Err(); err != nil {
				t.Fatalf("ExecutionContext(%d) context must start alive, got: %v", timeoutMs, err)
			}
		}()
	}
}

func TestExecutionContextNormalTimeoutSetsDeadlineByValue(t *testing.T) {
	ctx, cancel := ExecutionContext(1500)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("positive timeout must set a deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > 2*time.Second {
		t.Fatalf("deadline must sit about 1.5s away, remaining = %s", remaining)
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("fresh deadline context must start alive, got: %v", err)
	}
}

func TestExecutionContextExtremeTimeoutDoesNotExpireImmediately(t *testing.T) {
	for _, timeoutMs := range []int64{math.MaxInt64, math.MaxInt32} {
		ctx, cancel := ExecutionContext(timeoutMs)
		defer cancel()
		if err := ctx.Err(); err != nil {
			t.Fatalf("ExecutionContext(%d) must not expire immediately, got: %v", timeoutMs, err)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatalf("ExecutionContext(%d) must keep a deadline", timeoutMs)
		}
		if time.Until(deadline) <= 0 {
			t.Fatalf("ExecutionContext(%d) deadline must sit in the future", timeoutMs)
		}
		cancel()
		if err := ctx.Err(); err == nil {
			t.Fatalf("cancelling ExecutionContext(%d) must cancel it", timeoutMs)
		}
	}
}
