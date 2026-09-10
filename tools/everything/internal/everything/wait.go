package everything

import (
	"context"
	"time"
)

// waitProbe observes one cycle of a wait target. It reports done when the
// target is reached and failure when an explicit failure fact is observed.
// Reporting neither means the target is still pending, so the wait continues.
type waitProbe func(ctx context.Context) (done bool, failure error)

// waitEnvironment carries the injectable timing behavior of a wait loop.
// Production code waits on the real clock; tests inject deterministic
// behavior so no test depends on a specific wait duration.
type waitEnvironment struct {
	interval time.Duration
	sleep    func(ctx context.Context, duration time.Duration) error
}

// waitUntil drives an internal wait to one of its only three exits:
//
//   - completion: the probe reports the target as done;
//   - failure: the probe reports an explicit failure fact;
//   - deadline: the caller-specified context deadline expires.
//
// Context cancellation always ends the wait. A wait never ends because its
// probes made no observable progress.
func waitUntil(ctx context.Context, env waitEnvironment, probe waitProbe) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		done, failure := probe(ctx)
		if done {
			return nil
		}
		if failure != nil {
			return failure
		}
		if err := env.sleep(ctx, env.interval); err != nil {
			return err
		}
	}
}

// sleepWithContext sleeps for the given duration unless the context ends
// first, so wait loops always respond to cancellation.
func sleepWithContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
