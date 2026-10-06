// Package aiimage 是 ai-image 工具的业务动作实现：
// 多运营商生图、会话图片参考、生成结果回写会话，以及工具自身配置区的读写。
package aiimage

import (
	"context"
	"fmt"

	"eucli-box/tools/ai-image/internal/toolcontrol"
	"eucli-box/tools/ai-image/internal/types"
)

// SessionService 是工具访问宿主会话能力的窄接口：由控制通道客户端实现；
// 独立运行（无宿主控制通道）时为 nil，会话相关动作明确失败。
type SessionService interface {
	Request(ctx context.Context, capability string, access string, payload any) (toolcontrol.CapabilityResult, error)
}

// Execute 执行一次 ai-image 工具动作。
func Execute(ctx context.Context, input types.ToolExecutionInput, session SessionService) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("tool execution cancelled", err, nil)
	}
	args, err := parseArguments(input.Arguments)
	if err != nil {
		return failure("parse ai-image request", err, nil)
	}
	switch args.Action {
	case actionGenerate:
		return runGenerate(ctx, input, args, session)
	case actionSessionImages:
		return runSessionImages(ctx, session)
	case actionConfigList:
		return runConfigList(input)
	case actionConfigRead:
		return runConfigRead(input, args)
	case actionConfigWrite:
		return runConfigWrite(input, args)
	case actionConfigDelete:
		return runConfigDelete(input, args)
	case actionConfigEdit:
		return runConfigEdit(input, args)
	default:
		return failure("parse ai-image request", fmt.Errorf("unsupported action %q", args.Action), map[string]any{"action": args.Action})
	}
}
