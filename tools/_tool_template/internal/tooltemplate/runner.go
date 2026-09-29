// Package tooltemplate 是 AI 工具的业务动作所在位置。
//
// 模板不包含任何业务功能：Execute 是业务动作的唯一入口，
// 新工具在此处接入自己的基础动作，或把动作拆分为本包内的多个文件。
// 公共底座（入口、控制协议接入、数据迁移骨架）不属于本包，不要在此重复实现。
package tooltemplate

import (
	"context"

	"eucli-box/pkg/types"
)

// Execute 执行一次工具动作。
//
// 模板尚未实现任何业务动作：它明确失败，如实说明「未实现」，
// 不返回假成功、不静默忽略，让未完成的工具在第一次调用时就暴露出来。
// 新工具在实现自己的动作后删除本提示。
func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("tool execution cancelled", err)
	}
	return failure("no business action implemented", nil)
}

func failure(message string, err error) types.ToolExecutionOutput {
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
