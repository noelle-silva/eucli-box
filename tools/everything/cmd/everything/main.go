package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
	everything "eucli-box/tools/everything/internal/everything"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--keepalive-guard":
			os.Exit(runKeepAliveGuard(os.Args[2:]))
		case "--steward-install":
			os.Exit(runStewardInstall(os.Args[2:]))
		}
	}
	output := run()
	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(output); err != nil {
		os.Exit(1)
	}
}

func runKeepAliveGuard(args []string) int {
	if len(args) != 2 {
		return 1
	}
	if err := everything.RunKeepAliveGuard(args[0], args[1]); err != nil {
		return 2
	}
	return 0
}

func runStewardInstall(args []string) int {
	if len(args) != 1 {
		return 1
	}
	if err := everything.RunStewardInstall(args[0]); err != nil {
		return 2
	}
	return 0
}

func run() types.ToolExecutionOutput {
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		return failedOutput("failed to read tool input", err)
	}
	var input types.ToolExecutionInput
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return failedOutput("failed to decode tool input", err)
	}
	executionCtx, cancel := toolcontrol.ExecutionContext(input.TimeoutMs)
	defer cancel()
	client, err := toolcontrol.AdoptControl(executionCtx)
	if err != nil {
		return toolcontrol.ControlFailedOutput(err)
	}
	if client != nil {
		defer client.Close()
		go func() { _ = client.Serve(executionCtx) }()
	}
	return everything.Execute(executionCtx, input)
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
