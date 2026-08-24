package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
	everything "eucli-box/tools/everything/internal/everything"
)

// defaultToolBudgetMs is the tool's own default execution budget; a
// caller-specified budget overrides it, and both are clamped by the unified
// platform cap.
const defaultToolBudgetMs = 60_000

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
	budget := toolcontrol.ClampToolBudget(input.TimeoutMs, defaultToolBudgetMs)
	executionCtx, cancel := context.WithTimeout(context.Background(), budget)
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
