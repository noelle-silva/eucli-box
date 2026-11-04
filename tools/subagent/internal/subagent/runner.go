// Package subagent 是 subagent 工具的业务动作实现：单一职责——
// 把一条消息发给指定角色，等它停下，再把它的最新回复交回。
package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"eucli-box/tools/subagent/internal/toolcontrol"
	"eucli-box/tools/subagent/internal/types"
)

// SessionService 是工具访问宿主会话能力的窄接口：由控制通道客户端实现；
// 独立运行（无宿主控制通道）时为 nil，发送动作明确失败。
type SessionService interface {
	Request(ctx context.Context, capability string, access string, payload any) (toolcontrol.CapabilityResult, error)
}

// Execute 执行一次 subagent 工具动作。
func Execute(ctx context.Context, input types.ToolExecutionInput, session SessionService) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("tool execution cancelled", err, nil)
	}
	request, err := parseRequest(input.Arguments)
	if err != nil {
		return failure("解析 subagent 请求失败", err, nil)
	}
	if session == nil {
		return failure("当前没有宿主控制通道，无法发送会话消息", nil, nil)
	}
	result, err := session.Request(ctx, types.ToolCapabilitySessionMessage, types.ToolCapabilityAccessWrite, request)
	if err != nil {
		return failure("发送会话消息失败", err, nil)
	}
	if err := capabilityFailure(result); err != nil {
		return failure("发送会话消息失败", err, nil)
	}
	payload, err := json.Marshal(result.Payload)
	if err != nil {
		return failure("会话消息响应无效", err, nil)
	}
	var answer types.ToolMessageResult
	if err := json.Unmarshal(payload, &answer); err != nil {
		return failure("会话消息响应无效", err, nil)
	}
	content := fmt.Sprintf("角色 %s 已回复（会话 %s，消息 %s）：\n%s", answer.RoleID, answer.SessionID, answer.MessageID, answer.Message)
	return types.ToolExecutionOutput{
		Status:  types.ToolStatusSuccess,
		Content: content,
		Metadata: map[string]any{
			"roleId":    answer.RoleID,
			"sessionId": answer.SessionID,
			"messageId": answer.MessageID,
			"message":   answer.Message,
		},
	}
}

// parseRequest 只校验工具自己必须有的两件事：发给谁、说什么；
// 工作区与会话的定位口径留给宿主，工具不猜。
func parseRequest(arguments map[string]any) (types.ToolMessageRequest, error) {
	roleID := strings.TrimSpace(stringValue(arguments["roleId"]))
	message := strings.TrimSpace(stringValue(arguments["message"]))
	if roleID == "" {
		return types.ToolMessageRequest{}, errors.New("roleId 不能为空")
	}
	if message == "" {
		return types.ToolMessageRequest{}, errors.New("message 不能为空")
	}
	return types.ToolMessageRequest{
		RoleID:      roleID,
		WorkspaceID: strings.TrimSpace(stringValue(arguments["workspaceId"])),
		SessionID:   strings.TrimSpace(stringValue(arguments["sessionId"])),
		Message:     message,
	}, nil
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
