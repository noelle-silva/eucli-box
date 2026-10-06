package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"

	"eucli-box/tools/zhihu_search/internal/toolcontrol"
	"eucli-box/tools/zhihu_search/internal/types"
	zhihusearch "eucli-box/tools/zhihu_search/internal/zhihusearch"
)

func main() {
	output := run()
	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(output); err != nil {
		os.Exit(1)
	}
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
	if input.RequestKind == migrationRequestKind {
		return runDataMigration(executionCtx, input)
	}
	client, err := toolcontrol.AdoptControl(executionCtx)
	if err != nil {
		return toolcontrol.ControlFailedOutput(err)
	}
	if client != nil {
		defer client.Close()
		go func() { _ = client.Serve(executionCtx) }()
	}
	return zhihusearch.Execute(executionCtx, input)
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
