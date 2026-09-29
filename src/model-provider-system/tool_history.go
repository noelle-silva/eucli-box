package modelprovider

import (
	"encoding/json"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

func promptToolParts(message types.PromptMessage) []types.MessagePart {
	parts := make([]types.MessagePart, 0, len(message.Parts))
	for _, part := range message.Parts {
		if part.Type != "tool" || strings.TrimSpace(part.CallID) == "" || strings.TrimSpace(part.ToolName) == "" {
			continue
		}
		parts = append(parts, part)
	}
	return parts
}

func toolArgumentsJSON(part types.MessagePart) (string, error) {
	if part.Input == nil {
		return "{}", nil
	}
	payload, err := json.Marshal(part.Input)
	if err != nil {
		return "", providerInvalid("failed to encode tool call arguments", err)
	}
	return string(payload), nil
}

func requireToolResults(parts []types.MessagePart) error {
	for _, part := range parts {
		if part.Result == nil {
			return providerInvalid("assistant tool history is missing tool result", nil)
		}
	}
	return nil
}

func toolResultText(part types.MessagePart) string {
	if part.Result == nil {
		return ""
	}
	payload := map[string]any{
		"status": part.Result.Status,
	}
	if strings.TrimSpace(part.Result.Content) != "" {
		payload["content"] = part.Result.Content
	}
	if strings.TrimSpace(part.Result.Error) != "" {
		payload["error"] = part.Result.Error
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return string(part.Result.Status)
	}
	return string(data)
}

// toolImageAnchorText 是工具产物图片的统一锚点文字：让模型明确这张图
// 是哪个工具哪次调用返回的，不会误认成用户发来的素材。
func toolImageAnchorText(toolName string, callID string, attachmentID string) string {
	name := strings.TrimSpace(toolName)
	if name == "" {
		name = "tool"
	}
	return fmt.Sprintf("工具 %s（调用 %s）返回的图片附件 %s：", name, strings.TrimSpace(callID), strings.TrimSpace(attachmentID))
}

// toolNameForCall 取某次工具调用的工具名，用于锚点文字。
func toolNameForCall(parts []types.MessagePart, callID string) string {
	target := strings.TrimSpace(callID)
	for _, part := range parts {
		if strings.TrimSpace(part.CallID) == target {
			return part.ToolName
		}
	}
	return ""
}
