package toolcalling

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

const toolTerminationWait = 5 * time.Second

type toolProcess struct {
	cmd     *exec.Cmd
	stdout  bytes.Buffer
	stderr  bytes.Buffer
	mu      sync.Mutex
	done    chan struct{}
	exitErr error
}

type toolProcessOutcome struct {
	Stdout       []byte
	Stderr       []byte
	ExitError    error
	FailureKind  string
	FailureError error
}

type toolProcessEvent struct {
	Kind        string
	FailureKind string
	Err         error
	ObservedAt  time.Time
}

const (
	toolEventParentCancelled = "parent_cancelled"
	toolEventWatchdogFailed  = "watchdog_failed"
	toolEventProcessExited   = "process_exited"
	toolEventLegacyTimeout   = "legacy_timeout"
)

func startToolProcess(executable string, workdir string, input []byte, control *toolcontrol.Server) (*toolProcess, error) {
	cmd := exec.Command(executable)
	cmd.Dir = workdir
	cmd.Stdin = bytes.NewReader(input)
	process := &toolProcess{cmd: cmd, done: make(chan struct{})}
	cmd.Stdout = &process.stdout
	cmd.Stderr = &process.stderr
	configureToolProcess(cmd)
	if control != nil {
		cmd.Env = replaceEnvironment(os.Environ(), map[string]string{
			"EUCLI_TOOL_CONTROL_ADDR":     control.Address(),
			"EUCLI_TOOL_CONTROL_TOKEN":    control.Token(),
			"EUCLI_TOOL_CONTROL_VERSION":  "1",
			"EUCLI_TOOL_CONTROL_REQUIRED": "1",
		})
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return process, nil
}

func (p *toolProcess) wait() error {
	if p == nil || p.cmd == nil {
		return errors.New("tool process is not initialized")
	}
	err := p.cmd.Wait()
	p.mu.Lock()
	p.exitErr = err
	close(p.done)
	p.mu.Unlock()
	return err
}

func (p *toolProcess) waitForExit(timeout time.Duration) (error, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-p.done:
		p.mu.Lock()
		err := p.exitErr
		p.mu.Unlock()
		return err, true
	case <-timer.C:
		return nil, false
	}
}

func (p *toolProcess) waitDone() <-chan struct{} {
	if p == nil || p.done == nil {
		return nil
	}
	return p.done
}

func (p *toolProcess) terminateTree() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return terminateToolProcessTree(p.cmd.Process.Pid)
}

func (s *system) executeToolProcess(ctx context.Context, executable string, workdir string, input []byte, capabilities types.ToolControlCapabilities) toolProcessOutcome {
	var control *toolcontrol.Server
	if capabilities.Heartbeat {
		var err error
		control, err = toolcontrol.NewServer(toolcontrol.Config{Timeout: s.config.ToolWatchdogTimeout, PingInterval: s.config.ToolWatchdogPingInterval})
		if err != nil {
			return toolProcessOutcome{FailureError: err}
		}
	}
	process, err := startToolProcess(executable, workdir, input, control)
	if err != nil {
		if control != nil {
			_ = control.Close()
		}
		return toolProcessOutcome{FailureError: fmt.Errorf("start tool process: %w", err)}
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- process.wait() }()

	handshakeCompleted := control == nil
	if control != nil {
		handshakeCtx, handshakeCancel := context.WithTimeout(ctx, s.config.ToolWatchdogTimeout)
		handshakeCh := make(chan error, 1)
		go func() { handshakeCh <- control.AcceptAndHandshake(handshakeCtx) }()
		event, handshakeOK := waitForHandshake(ctx, waitCh, handshakeCh)
		handshakeCancel()
		if !handshakeOK {
			return s.finishToolProcess(control, process, event, event.Kind != toolEventProcessExited, false)
		}
		handshakeCompleted = true
	}

	eventCtx, stopEvents := context.WithCancel(context.Background())
	defer stopEvents()
	events := make(chan toolProcessEvent, 4)
	sendEvent := func(event toolProcessEvent) {
		select {
		case events <- event:
		case <-eventCtx.Done():
		}
	}
	go func() {
		waitErr := <-waitCh
		sendEvent(toolProcessEvent{Kind: toolEventProcessExited, Err: waitErr, ObservedAt: time.Now()})
	}()
	go func() {
		select {
		case <-ctx.Done():
			sendEvent(toolProcessEvent{Kind: toolEventParentCancelled, FailureKind: "user_cancelled", Err: ctx.Err(), ObservedAt: time.Now()})
		case <-eventCtx.Done():
		}
	}()
	if control != nil {
		go func() {
			failure, ok := <-control.Watch(ctx)
			if ok {
				sendEvent(toolProcessEvent{Kind: toolEventWatchdogFailed, FailureKind: string(failure), ObservedAt: time.Now()})
			}
			select {
			case <-process.waitDone():
			case <-time.After(100 * time.Millisecond):
				sendEvent(toolProcessEvent{Kind: toolEventWatchdogFailed, FailureKind: "tool_protocol_failed", ObservedAt: time.Now()})
			case <-eventCtx.Done():
			}
		}()
	} else {
		go func() {
			timer := time.NewTimer(s.config.LegacyToolTimeout)
			defer timer.Stop()
			select {
			case <-timer.C:
				sendEvent(toolProcessEvent{Kind: toolEventLegacyTimeout, FailureKind: "legacy_tool_timeout", Err: context.DeadlineExceeded, ObservedAt: time.Now()})
			case <-eventCtx.Done():
			}
		}()
	}
	for {
		event := <-events
		candidates := []toolProcessEvent{event}
		for {
			select {
			case candidate := <-events:
				candidates = append(candidates, candidate)
			default:
				event = chooseToolProcessEvent(candidates)
				terminate := event.Kind != toolEventProcessExited
				return s.finishToolProcess(control, process, event, terminate, handshakeCompleted)
			}
		}
	}
}

func waitForHandshake(ctx context.Context, waitCh <-chan error, handshakeCh <-chan error) (toolProcessEvent, bool) {
	for {
		if ctx.Err() != nil {
			return toolProcessEvent{Kind: toolEventParentCancelled, FailureKind: "user_cancelled", Err: ctx.Err(), ObservedAt: time.Now()}, false
		}
		select {
		case waitErr := <-waitCh:
			return toolProcessEvent{Kind: toolEventProcessExited, Err: waitErr, ObservedAt: time.Now()}, false
		case err := <-handshakeCh:
			if err == nil {
				return toolProcessEvent{}, true
			}
			return toolProcessEvent{Kind: toolEventWatchdogFailed, FailureKind: "tool_protocol_failed", Err: err, ObservedAt: time.Now()}, false
		case <-ctx.Done():
			return toolProcessEvent{Kind: toolEventParentCancelled, FailureKind: "user_cancelled", Err: ctx.Err(), ObservedAt: time.Now()}, false
		}
	}
}

func (s *system) finishToolProcess(control *toolcontrol.Server, process *toolProcess, event toolProcessEvent, terminate bool, handshakeCompleted bool) toolProcessOutcome {
	if control != nil {
		_ = control.Close()
	}
	var waitErr error
	terminationError := ""
	if event.Kind == toolEventProcessExited {
		waitErr = event.Err
	} else if terminate {
		if err := process.terminateTree(); err != nil {
			terminationError = err.Error()
		}
		var exited bool
		waitErr, exited = process.waitForExit(toolTerminationWait)
		if !exited {
			if terminationError == "" {
				terminationError = "process did not exit after termination"
			}
		} else {
			terminationError = ""
		}
	}
	process.mu.Lock()
	stdout := append([]byte(nil), process.stdout.Bytes()...)
	stderr := append([]byte(nil), process.stderr.Bytes()...)
	process.mu.Unlock()
	outcome := toolProcessOutcome{Stdout: stdout, Stderr: stderr, ExitError: waitErr, FailureKind: event.FailureKind}
	if terminationError != "" {
		outcome.FailureError = errors.New(terminationError)
	}
	if event.Kind == toolEventProcessExited && event.FailureKind == "" && control != nil && !handshakeCompleted && waitErr == nil {
		outcome.FailureKind = "tool_protocol_failed"
	}
	if event.Kind == toolEventWatchdogFailed && outcome.FailureKind == "" {
		outcome.FailureKind = "tool_protocol_failed"
	}
	if outcome.FailureKind != "" && len(strings.TrimSpace(string(stderr))) > 0 {
		outcome.FailureError = errors.New(strings.TrimSpace(string(stderr)))
	}
	if outcome.FailureKind != "" && event.Err != nil {
		outcome.FailureError = errors.Join(outcome.FailureError, event.Err)
	}
	return outcome
}

func chooseToolProcessEvent(events []toolProcessEvent) toolProcessEvent {
	if len(events) == 0 {
		return toolProcessEvent{}
	}
	copyEvents := append([]toolProcessEvent(nil), events...)
	sort.SliceStable(copyEvents, func(i, j int) bool {
		if copyEvents[i].ObservedAt.Equal(copyEvents[j].ObservedAt) || copyEvents[i].ObservedAt.Sub(copyEvents[j].ObservedAt) < time.Millisecond && copyEvents[j].ObservedAt.Sub(copyEvents[i].ObservedAt) < time.Millisecond {
			return eventPriority(copyEvents[i].Kind) > eventPriority(copyEvents[j].Kind)
		}
		return copyEvents[i].ObservedAt.Before(copyEvents[j].ObservedAt)
	})
	return copyEvents[0]
}

func eventPriority(kind string) int {
	switch kind {
	case toolEventParentCancelled:
		return 4
	case toolEventWatchdogFailed:
		return 3
	case toolEventLegacyTimeout:
		return 2
	case toolEventProcessExited:
		return 1
	default:
		return 0
	}
}

func replaceEnvironment(base []string, replacements map[string]string) []string {
	result := make([]string, 0, len(base)+len(replacements))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			result = append(result, item)
			continue
		}
		matched := false
		for replacementKey := range replacements {
			if strings.EqualFold(key, replacementKey) {
				matched = true
				break
			}
		}
		if !matched {
			result = append(result, item)
		}
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}

func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
