package toolcontrol

import (
	"context"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

const (
	controlAddressEnv  = "EUCLI_TOOL_CONTROL_ADDR"
	controlTokenEnv    = "EUCLI_TOOL_CONTROL_TOKEN"
	controlVersionEnv  = "EUCLI_TOOL_CONTROL_VERSION"
	controlRequiredEnv = "EUCLI_TOOL_CONTROL_REQUIRED"
)

// ExecutionContext derives the tool execution context from the
// caller-specified timeout: a positive timeoutMs sets a deadline, zero or a
// negative timeoutMs mean the caller did not set one and no deadline is
// applied. Values too large to fit a time.Duration are clamped instead of
// overflowing, so an extreme timeout can never turn into an already-expired
// deadline.
func ExecutionContext(timeoutMs int64) (context.Context, context.CancelFunc) {
	if timeoutMs > 0 {
		return context.WithTimeout(context.Background(), deadlineDuration(timeoutMs))
	}
	return context.WithCancel(context.Background())
}

// deadlineDuration converts a positive millisecond value into the largest
// time.Duration that is guaranteed to sit far in the future. Directly
// multiplying by time.Millisecond would overflow for extreme values and make
// the derived context expire immediately.
func deadlineDuration(timeoutMs int64) time.Duration {
	const maxMilliseconds = math.MaxInt64 / int64(time.Millisecond)
	if timeoutMs > maxMilliseconds {
		timeoutMs = maxMilliseconds
	}
	return time.Duration(timeoutMs) * time.Millisecond
}

// AdoptControl connects a tool binary to the host control channel when the
// host manages it. Without a full control environment the tool runs standalone
// and a nil client is returned without error.
func AdoptControl(ctx context.Context) (*Client, error) {
	address := strings.TrimSpace(os.Getenv(controlAddressEnv))
	token := strings.TrimSpace(os.Getenv(controlTokenEnv))
	version := strings.TrimSpace(os.Getenv(controlVersionEnv))
	required := strings.TrimSpace(os.Getenv(controlRequiredEnv)) == "1"

	if !required && address == "" && token == "" && version == "" {
		return nil, nil
	}
	if address == "" || token == "" || version == "" {
		return nil, errors.New("tool control environment is incomplete")
	}
	parsedVersion, err := strconv.Atoi(version)
	if err != nil || parsedVersion != ProtocolVersion {
		return nil, errors.New("tool control protocol version is invalid")
	}
	client, err := Connect(ctx, address, token)
	if err != nil {
		return nil, err
	}
	if err := client.WaitReady(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// ControlFailedOutput builds the unified protocol-failure tool result shared by
// every tool binary.
func ControlFailedOutput(err error) types.ToolExecutionOutput {
	message := "tool control protocol failed"
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
