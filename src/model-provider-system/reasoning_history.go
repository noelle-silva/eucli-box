package modelprovider

import (
	"strings"

	"eucli-box/pkg/types"
)

func promptReasoningParts(message types.PromptMessage) []types.MessagePart {
	parts := make([]types.MessagePart, 0, len(message.Parts))
	for _, part := range message.Parts {
		if strings.TrimSpace(part.Type) != "reasoning" {
			continue
		}
		if strings.TrimSpace(part.Text) == "" && strings.TrimSpace(part.Signature) == "" && strings.TrimSpace(part.Data) == "" {
			continue
		}
		parts = append(parts, part)
	}
	return parts
}

func promptReasoningText(message types.PromptMessage) string {
	blocks := make([]string, 0, len(message.Parts))
	for _, part := range promptReasoningParts(message) {
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		blocks = append(blocks, text)
	}
	return strings.Join(blocks, "\n\n")
}

func firstPromptReasoningPart(message types.PromptMessage) (types.MessagePart, bool) {
	for _, part := range promptReasoningParts(message) {
		return part, true
	}
	return types.MessagePart{}, false
}

func firstPromptReasoningPartForSource(message types.PromptMessage, source string) (types.MessagePart, bool) {
	trimmedSource := strings.TrimSpace(source)
	for _, part := range promptReasoningParts(message) {
		if trimmedSource != "" && strings.TrimSpace(part.Source) != trimmedSource {
			continue
		}
		return part, true
	}
	return types.MessagePart{}, false
}
