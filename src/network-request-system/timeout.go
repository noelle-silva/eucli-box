package networkrequest

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type timeoutMonitor struct {
	timeout      time.Duration
	cancel       context.CancelFunc
	stopOnce     sync.Once
	stopCh       chan struct{}
	doneCh       chan struct{}
	timedOutFlag atomic.Bool
	lastActivity atomic.Int64
}

func startTotalTimeoutMonitor(timeout time.Duration, cancel context.CancelFunc) *timeoutMonitor {
	monitor := newTimeoutMonitor(timeout, cancel)
	go monitor.runTotal()
	return monitor
}

func startIdleTimeoutMonitor(timeout time.Duration, cancel context.CancelFunc) *timeoutMonitor {
	monitor := newTimeoutMonitor(timeout, cancel)
	go monitor.runIdle()
	return monitor
}

func newTimeoutMonitor(timeout time.Duration, cancel context.CancelFunc) *timeoutMonitor {
	monitor := &timeoutMonitor{timeout: timeout, cancel: cancel, stopCh: make(chan struct{}), doneCh: make(chan struct{})}
	monitor.touch()
	return monitor
}

func (m *timeoutMonitor) runTotal() {
	defer close(m.doneCh)
	if m.timeout <= 0 {
		<-m.stopCh
		return
	}
	timer := time.NewTimer(m.timeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		m.timeoutNow()
	case <-m.stopCh:
	}
}

func (m *timeoutMonitor) runIdle() {
	defer close(m.doneCh)
	if m.timeout <= 0 {
		<-m.stopCh
		return
	}
	ticker := time.NewTicker(idleCheckInterval(m.timeout))
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			last := time.Unix(0, m.lastActivity.Load())
			if time.Since(last) >= m.timeout {
				m.timeoutNow()
				return
			}
		case <-m.stopCh:
			return
		}
	}
}

func idleCheckInterval(timeout time.Duration) time.Duration {
	interval := timeout / 4
	if interval < 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	if interval > time.Second {
		return time.Second
	}
	return interval
}

func (m *timeoutMonitor) touch() {
	if m == nil {
		return
	}
	m.lastActivity.Store(time.Now().UnixNano())
}

func (m *timeoutMonitor) timedOut() bool {
	return m != nil && m.timedOutFlag.Load()
}

func (m *timeoutMonitor) timeoutNow() {
	if m.timedOutFlag.CompareAndSwap(false, true) {
		m.cancel()
	}
}

func (m *timeoutMonitor) stop() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		close(m.stopCh)
		<-m.doneCh
	})
}
