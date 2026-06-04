package modelprovider

import (
	"encoding/json"
	"strings"

	"eucli-box/pkg/types"
)

func promptToolParts(message types.PromptMessage) []types.MessagePart {
	parts := make([]types.MessagePart, 0, len(message.Parts))
	for _, part := range message.Parts {
		if part.Type != "tool" || strings.TrimSpace(part.CallID) == "" || strings.TrimSpace(part.ToolName) == "" {
			continue
		}
		if toolPartHidden(part) {
			continue
		}
		parts = append(parts, part)
	}
	return parts
}

func toolPartHidden(part types.MessagePart) bool {
	if len(part.Display) == 0 {
		return false
	}
	return truthy(part.Display["hideInvocation"]) || truthy(part.Display["hideResult"])
}

func truthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
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
	if len(part.Result.Metadata) > 0 {
		payload["metadata"] = part.Result.Metadata
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return string(part.Result.Status)
	}
	return string(data)
}
