// Package subagent 是 subagent 工具的业务动作实现：两个动作共用同一份底座。
//   - send：把一条消息发给指定角色，等它停下，再把它的最新回复交回；
//   - list：按当前工作区范围列出角色名、标识与描述。
package subagent

import (
	"context"
	"errors"
	"strings"

	"eucli-box/tools/subagent/internal/toolcontrol"
	"eucli-box/tools/subagent/internal/types"
)

// SessionService 是工具访问宿主能力的窄接口：由控制通道客户端实现；
// 独立运行（无宿主控制通道）时为 nil，需要宿主的能力动作明确失败。
type SessionService interface {
	Request(ctx context.Context, capability string, access string, payload any) (toolcontrol.CapabilityResult, error)
}

// 动作名：发送消息是默认动作，列出角色是第二个动作。
const (
	ActionSend = "send"
	ActionList = "list"
)

// Execute 执行一次 subagent 工具动作。
func Execute(ctx context.Context, input types.ToolExecutionInput, session SessionService) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("tool execution cancelled", err, nil)
	}
	switch actionOf(input.Arguments) {
	case ActionList:
		return executeList(ctx, input, session)
	case ActionSend:
		return executeSend(ctx, input, session)
	default:
		return failure("解析 subagent 请求失败", errors.New("action 只能是 send 或 list"), nil)
	}
}

// actionOf 归一化动作名：未指定即发送，保持旧调用不需要改参数。
func actionOf(arguments map[string]any) string {
	action := strings.TrimSpace(stringValue(arguments["action"]))
	if action == "" {
		return ActionSend
	}
	return action
}

// capabilityFailure 把能力裁决翻译成工具侧错误：宿主给出的原因原样保留。
func capabilityFailure(result toolcontrol.CapabilityResult) error {
	if result.Status == toolcontrol.CapabilityStatusSuccess {
		return nil
	}
	if text := strings.TrimSpace(result.Error); text != "" {
		return errors.New(text)
	}
	if result.Status == toolcontrol.CapabilityStatusDenied {
		return errors.New("用户未授权此能力，请前往工具设置页开启能力授权")
	}
	return errors.New("宿主未能处理这次能力请求")
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func failure(message string, err error, metadata map[string]any) types.ToolExecutionOutput {
	errorMessage := message
	if err != nil {
		errorMessage = message + ": " + err.Error()
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["error"] = errorMessage
	return types.ToolExecutionOutput{
		Status:   types.ToolStatusFailed,
		Content:  errorMessage,
		Error:    errorMessage,
		Metadata: metadata,
	}
}
