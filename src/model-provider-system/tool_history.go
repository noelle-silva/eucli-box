package modelprovider

import (
	"encoding/json"
	"strings"

	"eucli-box/pkg/types"
)

func promptNativeToolParts(message types.PromptMessage) []types.MessagePart {
	parts := make([]types.MessagePart, 0, len(message.Parts))
	for _, part := range message.Parts {
		if part.Type != "tool" || strings.TrimSpace(part.CallID) == "" || strings.TrimSpace(part.ToolName) == "" {
			continue
		}
		if strings.TrimSpace(part.Source) == types.ToolCallSourceTextProtocol {
			continue
		}
		parts = append(parts, part)
	}
	return parts
}

func promptTextProtocolToolResultParts(message types.PromptMessage) []types.MessagePart {
	parts := make([]types.MessagePart, 0, len(message.Parts))
	for _, part := range message.Parts {
		if part.Type != "tool" || strings.TrimSpace(part.Source) != types.ToolCallSourceTextProtocol || part.Result == nil {
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

func textProtocolToolResultsText(parts []types.MessagePart) string {
	items := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		if part.Result == nil {
			continue
		}
		item := map[string]any{
			"toolName": textProtocolToolResultName(part),
			"status":   part.Result.Status,
		}
		if strings.TrimSpace(part.Result.Content) != "" {
			item["content"] = part.Result.Content
		}
		if strings.TrimSpace(part.Result.Error) != "" {
			item["error"] = part.Result.Error
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return ""
	}
	data, err := json.Marshal(map[string]any{"textProtocolToolResults": items})
	if err != nil {
		return "Text protocol tool results are available."
	}
	return "External tool results for text protocol requests:\n" + string(data)
}

func appendUserTextObservation(messages []map[string]any, text string) []map[string]any {
	if strings.TrimSpace(text) == "" {
		return messages
	}
	return append(messages, map[string]any{"role": "user", "content": text})
}

func textProtocolToolResultName(part types.MessagePart) string {
	if strings.TrimSpace(part.ToolName) != "" {
		return part.ToolName
	}
	if part.Result != nil && strings.TrimSpace(part.Result.ToolName) != "" {
		return part.Result.ToolName
	}
	return "tool"
}
