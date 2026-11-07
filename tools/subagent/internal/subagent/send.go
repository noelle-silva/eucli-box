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
// 全部由宿主按其日常发送口径完成。
// 目标角色若卡在待确认上，就把那张审批单在本次调用里抛给宿主，
// 拿到裁决后接着等这条运行的结果，直到它真正停下。
func executeSend(ctx context.Context, input types.ToolExecutionInput, session SessionService) types.ToolExecutionOutput {
	request, err := parseSendRequest(input.Arguments, input.WorkspaceID)
	if err != nil {
		return failure("解析 subagent 请求失败", err, nil)
	}
	if session == nil {
		return failure("当前没有宿主控制通道，无法发送会话消息", nil, nil)
	}
	answer, err := requestMessage(ctx, session, request)
	for err == nil && answer.Status == types.ToolMessageStatusConfirmationRequired {
		next, relayErr := settleConfirmationAndResume(ctx, session, request, answer)
		if relayErr != nil {
			return failure("目标角色等待确认，无法继续", relayErr, map[string]any{"sessionId": answer.SessionID})
		}
		answer, err = next, nil
	}
	if err != nil {
		return failure("发送会话消息失败", err, nil)
	}
	if answer.Status != types.ToolMessageStatusReplied {
		return failure("目标角色未能给出答复", nil, map[string]any{"status": answer.Status, "sessionId": answer.SessionID})
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

// settleConfirmationAndResume 把待确认抛给宿主，拿到裁决后再接着等那条运行。
// 同意与拒绝都要续等：拒绝同样会让目标角色收尾，只是收尾方式不同。
func settleConfirmationAndResume(ctx context.Context, session SessionService, request types.ToolMessageRequest, answer types.ToolMessageResult) (types.ToolMessageResult, error) {
	if answer.Confirmation == nil {
		return types.ToolMessageResult{}, errors.New("宿主报告了待确认，却没有给出审批单事实")
	}
	runID := strings.TrimSpace(answer.RunID)
	if runID == "" {
		return types.ToolMessageResult{}, errors.New("宿主没有给出等待中的运行标识，无法续等")
	}
	outcome, err := relayConfirmation(ctx, session, *answer.Confirmation)
	if err != nil {
		return types.ToolMessageResult{}, err
	}
	switch strings.TrimSpace(outcome.Status) {
	case types.ToolConfirmationStatusExpired:
		return types.ToolMessageResult{}, fmt.Errorf("审批单 %s 已失效，目标角色不再等待这次裁决", outcome.DecisionID)
	case types.ToolConfirmationStatusFailed:
		reason := strings.TrimSpace(outcome.Reason)
		if reason == "" {
			reason = "应用裁决时出错"
		}
		return types.ToolMessageResult{}, fmt.Errorf("审批单 %s 的裁决未能应用：%s", outcome.DecisionID, reason)
	}
	return requestMessage(ctx, session, types.ToolMessageRequest{
		RoleID:      request.RoleID,
		WorkspaceID: request.WorkspaceID,
		SessionID:   answer.SessionID,
		ResumeRunID: runID,
	})
}

// requestMessage 发起一次发送或续等，并把宿主的能力裁决翻译成结果或错误。
func requestMessage(ctx context.Context, session SessionService, request types.ToolMessageRequest) (types.ToolMessageResult, error) {
	result, err := session.Request(ctx, types.ToolCapabilitySessionMessage, types.ToolCapabilityAccessWrite, request)
	if err != nil {
		return types.ToolMessageResult{}, err
	}
	if err := capabilityFailure(result); err != nil {
		return types.ToolMessageResult{}, err
	}
	payload, err := json.Marshal(result.Payload)
	if err != nil {
		return types.ToolMessageResult{}, errors.New("会话消息响应无效")
	}
	var answer types.ToolMessageResult
	if err := json.Unmarshal(payload, &answer); err != nil {
		return types.ToolMessageResult{}, errors.New("会话消息响应无效")
	}
	return answer, nil
}

// relayConfirmation 把审批单交给宿主摆到台面上，并等用户裁决。
func relayConfirmation(ctx context.Context, session SessionService, fact types.ToolConfirmationFact) (types.ToolConfirmationOutcome, error) {
	result, err := session.Request(ctx, types.ToolCapabilityToolConfirmation, types.ToolCapabilityAccessWrite, fact)
	if err != nil {
		return types.ToolConfirmationOutcome{}, err
	}
	if err := capabilityFailure(result); err != nil {
		return types.ToolConfirmationOutcome{}, err
	}
	payload, err := json.Marshal(result.Payload)
	if err != nil {
		return types.ToolConfirmationOutcome{}, errors.New("确认裁决响应无效")
	}
	var outcome types.ToolConfirmationOutcome
	if err := json.Unmarshal(payload, &outcome); err != nil {
		return types.ToolConfirmationOutcome{}, errors.New("确认裁决响应无效")
	}
	return outcome, nil
}

// parseSendRequest 只校验工具自己必须有的两件事：发给谁、说什么。
// 工作区定位：调用方显式写了就用写的；没写则用宿主注入的当前工作区标识，
// 于是工作区会话里不必重复交代工作区，普通会话里注入为空、照常走普通会话。
func parseSendRequest(arguments map[string]any, injectedWorkspaceID string) (types.ToolMessageRequest, error) {
	roleID := strings.TrimSpace(stringValue(arguments["roleId"]))
	message := strings.TrimSpace(stringValue(arguments["message"]))
	if roleID == "" {
		return types.ToolMessageRequest{}, errors.New("roleId 不能为空")
	}
	if message == "" {
		return types.ToolMessageRequest{}, errors.New("message 不能为空")
	}
	workspaceID := strings.TrimSpace(stringValue(arguments["workspaceId"]))
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(injectedWorkspaceID)
	}
	return types.ToolMessageRequest{
		RoleID:      roleID,
		WorkspaceID: workspaceID,
		SessionID:   strings.TrimSpace(stringValue(arguments["sessionId"])),
		Message:     message,
	}, nil
}
