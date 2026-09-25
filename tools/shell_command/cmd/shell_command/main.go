package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
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

	if types.IsToolWarmupRequest(input) {
		return runWarmup(executionCtx, executionCancel, input)
	}

	analysis, err := analyzeRequestedCommand(input)
	if err != nil {
		executionCancel()
		return failedOutput("command analysis failed", err), nil, noopCancel, nil
	}

	client, err := toolcontrol.AdoptControl(executionCtx)
	if err != nil {
		executionCancel()
		return toolcontrol.ControlFailedOutput(err), client, noopCancel, nil
	}
	if client == nil {
		output := shellcommand.Execute(executionCtx, input)
		attachAnalysis(output, analysis)
		return output, client, executionCancel, nil
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
		output = toolcontrol.ControlFailedOutput(controlErr)
	default:
	}
	attachAnalysis(output, analysis)
	return output, client, executionCancel, serveDone
}

// runWarmup handles a host warm-up request: it never analyzes or executes any
// user command, and only attaches the control channel (when the host provides
// one) before reporting the tool's warm-up outcome.
func runWarmup(ctx context.Context, cancel context.CancelFunc, input types.ToolExecutionInput) (types.ToolExecutionOutput, *toolcontrol.Client, context.CancelFunc, <-chan struct{}) {
	noopCancel := func() {}
	client, err := toolcontrol.AdoptControl(ctx)
	if err != nil {
		cancel()
		return toolcontrol.ControlFailedOutput(err), client, noopCancel, nil
	}
	var serveDone <-chan struct{}
	if client != nil {
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = client.Serve(ctx)
		}()
		serveDone = done
	}
	return warmupTool(ctx, input), client, cancel, serveDone
}

// warmupTool 组合热身动作：先运行一次无害分析以真实加载命令分析器，
// 再拉起默认 Provider 的 shell 执行一次无害命令；任一步失败都让预热如实失败。
func warmupTool(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	analyzer, err := newCommandAnalyzer(input.ToolBodyDirectory)
	if err != nil {
		return failedOutput("warmup command analyzer failed", err)
	}
	providerID, _ := shellcommand.WarmupProviderID(input)
	if _, err := analyzer.analyze(ctx, shellcommand.WarmupCommand, providerID, ""); err != nil {
		return failedOutput("warmup command analysis failed", err)
	}
	return shellcommand.Warmup(ctx, input)
}

// analyzeRequestedCommand runs the unified command analyzer over the requested
// command. The analyzer is a shipped component of the tool: any failure stops
// the tool rather than running the command without analysis.
func analyzeRequestedCommand(input types.ToolExecutionInput) (map[string]any, error) {
	command, ok := input.Arguments["command"].(string)
	if !ok || command == "" {
		return nil, nil
	}
	analyzer, err := newCommandAnalyzer(input.ToolBodyDirectory)
	if err != nil {
		return nil, err
	}
	provider := ""
	if value, ok := input.Arguments["provider"].(string); ok {
		provider = value
	}
	if strings.TrimSpace(provider) == "" {
		if defaultProvider, ok := shellcommand.DefaultProviderFor(input.ToolBodyDirectory); ok {
			provider = defaultProvider
		}
	}
	workdir := ""
	if value, ok := input.Arguments["workdir"].(string); ok {
		workdir = value
	}
	// 分析器与实际执行共用同一工作目录基准，影响路径判断才不会张冠李戴。
	return analyzer.analyze(context.Background(), command, provider, shellcommand.AnalyzerWorkdir(input, workdir))
}

// attachAnalysis merges the analysis report into the tool result metadata.
func attachAnalysis(output types.ToolExecutionOutput, analysis map[string]any) types.ToolExecutionOutput {
	if analysis == nil {
		return output
	}
	if output.Metadata == nil {
		output.Metadata = map[string]any{}
	}
	output.Metadata["commandAnalysis"] = analysis
	return output
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
