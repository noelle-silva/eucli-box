package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
	shellcommand "eucli-box/tools/shell_command/internal/shellcommand"
)

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
	output := shellcommand.Execute(executionCtx, input)
	select {
	case controlErr := <-controlErrorCh:
		output = failedControlOutput(controlErr)
	default:
	}
	return output, client, executionCancel, serveDone
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
