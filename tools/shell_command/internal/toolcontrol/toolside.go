package toolcontrol

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"

	"eucli-box/tools/shell_command/internal/types"
)

const (
	controlAddressEnv  = "EUCLI_TOOL_CONTROL_ADDR"
	controlTokenEnv    = "EUCLI_TOOL_CONTROL_TOKEN"
	controlVersionEnv  = "EUCLI_TOOL_CONTROL_VERSION"
	controlRequiredEnv = "EUCLI_TOOL_CONTROL_REQUIRED"
)

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
