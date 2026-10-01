// 本入口是 AI 工具的公共底座：从标准输入读取执行输入、接入宿主控制协议，
// 并按请求种类分流到数据迁移或业务动作；业务动作位于 internal/imagereader。
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
	imagereader "eucli-box/tools/image_reader/internal/imagereader"
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
	return imagereader.Execute(executionCtx, input, client)
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
