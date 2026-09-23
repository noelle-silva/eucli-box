package shellcommand

import (
	"context"

	"eucli-box/pkg/types"
)

// Warmup 执行一次工具预热：宿主通过预热请求要求工具做自我准备，
// 让执行链上的程序与依赖被系统读取并进入扫描缓存。
// 当前阶段只建立预热请求的识别与统一回执；真正的热身动作（命令分析器与
// Provider 预热）在后续推进中补齐，执行链与用户命令完全不受影响。
func Warmup(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	return types.ToolExecutionOutput{
		Status:  types.ToolStatusSuccess,
		Content: "shell_command warmup completed",
		Metadata: map[string]any{
			"warmup": true,
		},
	}
}
