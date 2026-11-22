package modelprovider

import (
	"strings"

	"eucli-box/pkg/types"
)

// 思维链历史按 source 标记隔离：source 是协议维度标记（"an" / "op"），
// 不是厂商名或模型名。回传时明文与签名/加密体必须取自同一 source，同进同退。
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

// promptReasoningPartsForSource 只保留 source 精确匹配的思维链片段。
// source 为空表示按"未标记来源"匹配，不做任何跨来源兜底。
func promptReasoningPartsForSource(message types.PromptMessage, source string) []types.MessagePart {
	trimmedSource := strings.TrimSpace(source)
	matched := make([]types.MessagePart, 0, len(message.Parts))
	for _, part := range promptReasoningParts(message) {
		if strings.TrimSpace(part.Source) != trimmedSource {
			continue
		}
		matched = append(matched, part)
	}
	return matched
}

// promptReasoningTextForSource 拼接指定来源的思维链明文。
func promptReasoningTextForSource(message types.PromptMessage, source string) string {
	blocks := make([]string, 0, len(message.Parts))
	for _, part := range promptReasoningPartsForSource(message, source) {
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		blocks = append(blocks, text)
	}
	return strings.Join(blocks, "\n\n")
}

// firstPromptReasoningPartForSource 取指定来源的第一个思维链片段，
// 它是该来源签名与加密体的唯一载体。
func firstPromptReasoningPartForSource(message types.PromptMessage, source string) (types.MessagePart, bool) {
	parts := promptReasoningPartsForSource(message, source)
	if len(parts) == 0 {
		return types.MessagePart{}, false
	}
	return parts[0], true
}
