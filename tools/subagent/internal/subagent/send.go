package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"eucli-box/tools/subagent/internal/types"
)

// executeSend 把一个发送意图交给宿主：发送、等待角色停下、取回最新回复
// 全部由宿主按其日常发送口径完成，工具只做转发与回传。
func executeSend(ctx context.Context, input types.ToolExecutionInput, session SessionService) types.ToolExecutionOutput {
	request, err := parseSendRequest(input.Arguments)
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

// parseSendRequest 只校验工具自己必须有的两件事：发给谁、说什么；
// 工作区与会话的定位口径留给宿主，工具不猜。
func parseSendRequest(arguments map[string]any) (types.ToolMessageRequest, error) {
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
