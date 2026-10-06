// 本入口是 AI 工具的公共底座：从标准输入读取执行输入、接入宿主控制协议，
// 并按请求种类分流到数据迁移或业务动作；业务动作位于 internal/everything。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"

	"eucli-box/tools/everything/internal/toolcontrol"
	"eucli-box/tools/everything/internal/types"
	everything "eucli-box/tools/everything/internal/everything"
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
	// timeoutMs 的声明语义是搜索请求时限，只在业务层作用于对 Everything 应用的请求；
	// 工具执行上下文保持可取消，绝不把该时限施加到控制通道与本地动作上。
	executionCtx, cancel := context.WithCancel(context.Background())
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
