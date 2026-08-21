package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"unicode/utf8"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
	shellcommand "eucli-box/tools/shell_command/internal/shellcommand"
)

// outputUpdatePreviewBytes caps the preview text carried by output updates.
const outputUpdatePreviewBytes = 8 * 1024

func main() {
	output, client, cancel, serveDone := run()
	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(output); err != nil {
		os.Exit(1)
	}
	cancel()
	if client != nil {
		_ = client.Close()
	}
	if serveDone != nil {
		<-serveDone
	}
}

func run() (types.ToolExecutionOutput, *toolcontrol.Client, context.CancelFunc, <-chan struct{}) {
	noopCancel := func() {}
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		return failedOutput("failed to read tool input", err), nil, noopCancel, nil
	}
	var input types.ToolExecutionInput
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return failedOutput("failed to decode tool input", err), nil, noopCancel, nil
	}
	executionCtx, executionCancel := context.WithCancel(context.Background())
	client, err := connectToolControl(executionCtx)
	if err != nil {
		executionCancel()
		return failedControlOutput(err), client, noopCancel, nil
	}
	if client == nil {
		return shellcommand.Execute(executionCtx, input), client, executionCancel, nil
	}
	controlErrorCh := make(chan error, 1)
	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		err := client.Serve(executionCtx)
		if err == nil || executionCtx.Err() != nil {
			return
		}
		select {
		case controlErrorCh <- err:
		default:
		}
		executionCancel()
	}()
	relay := newOutputUpdateRelay(client)
	output := shellcommand.ExecuteWithOutputHook(executionCtx, input, relay.report)
	relay.stop()
	select {
	case controlErr := <-controlErrorCh:
		output = failedControlOutput(controlErr)
	default:
	}
	return output, client, executionCancel, serveDone
}

// outputUpdateRelay streams raw command output chunks to the host over the
// tool control channel, capped at MaxOutputUpdates updates per run. Sending is
// best-effort: relay failures stop further updates silently, never the command.
type outputUpdateRelay struct {
	client *toolcontrol.Client

	mu       sync.Mutex
	sent     int64
	dropped  int64
	sequence uint64
	total    uint64
	preview  []byte
	stopped  bool
	failed   bool
}

func newOutputUpdateRelay(client *toolcontrol.Client) *outputUpdateRelay {
	return &outputUpdateRelay{client: client}
}

func (r *outputUpdateRelay) report(payload []byte) {
	r.mu.Lock()
	if r.stopped || r.failed {
		r.mu.Unlock()
		return
	}
	r.total += uint64(len(payload))
	r.preview = append(r.preview, payload...)
	if len(r.preview) > outputUpdatePreviewBytes {
		r.preview = r.preview[len(r.preview)-outputUpdatePreviewBytes:]
	}
	if r.sent >= toolcontrol.MaxOutputUpdates {
		r.dropped++
		r.mu.Unlock()
		return
	}
	r.sequence++
	update := toolcontrol.OutputUpdate{Bytes: r.total, Preview: safePreview(r.preview)}
	r.sent++
	r.mu.Unlock()

	if err := r.client.SendOutputUpdate(r.sequence, update); err != nil {
		r.mu.Lock()
		r.failed = true
		r.mu.Unlock()
	}
}

func (r *outputUpdateRelay) stop() {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
}

func safePreview(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	// Drop a partial rune at the window start, then trim an incomplete tail.
	index := 0
	for index < len(data) && data[index]&0xC0 == 0x80 {
		index++
	}
	trimmed := data[index:]
	end := 0
	for end < len(trimmed) {
		if !utf8.FullRune(trimmed[end:]) {
			trimmed = trimmed[:end]
			break
		}
		_, size := utf8.DecodeRune(trimmed[end:])
		end += size
	}
	return string(trimmed)
}

func failedControlOutput(err error) types.ToolExecutionOutput {
	message := "shell_command control protocol failed"
	if err != nil {
		message += ": " + err.Error()
	}
	return types.ToolExecutionOutput{
		Status:  types.ToolStatusFailed,
		Content: message,
		Error:   message,
		Metadata: map[string]any{
			"error":       message,
			"failureKind": "tool_protocol_failed",
		},
	}
}

func failedOutput(message string, err error) types.ToolExecutionOutput {
	errorMessage := message
	if err != nil {
		errorMessage = message + ": " + err.Error()
	}
	return types.ToolExecutionOutput{
		Status:  types.ToolStatusFailed,
		Content: errorMessage,
		Error:   errorMessage,
		Metadata: map[string]any{
			"error": errorMessage,
		},
	}
}
