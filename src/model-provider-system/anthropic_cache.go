package modelprovider

import "eucli-box/pkg/types"

const anthropicCacheControlEphemeral = "ephemeral"

func anthropicCacheControl() map[string]any {
	return map[string]any{"type": anthropicCacheControlEphemeral}
}

// anthropicSystemContent 把合并后的系统提示词包装成文本内容块，
// 并只在该块上打缓存标记。
func anthropicSystemContent(systemText string) []map[string]any {
	return []map[string]any{{
		"type":          "text",
		"text":          systemText,
		"cache_control": anthropicCacheControl(),
	}}
}

// anthropicToolsWithCacheBreakpoint 在工具列表非空时只标记最后一个工具。
func anthropicToolsWithCacheBreakpoint(tools []types.ToolDefinition) []map[string]any {
	converted := anthropicTools(tools)
	if len(converted) == 0 {
		return converted
	}
	converted[len(converted)-1]["cache_control"] = anthropicCacheControl()
	return converted
}

// anthropicMarkLastMessageCacheBreakpoint 在最后一条消息的最后一个可用内容块上
// 打缓存标记；思考块不可用，向前找最近的可用块。正文为纯文本时先转成内容块。
func anthropicMarkLastMessageCacheBreakpoint(messages []map[string]any) {
	for index := len(messages) - 1; index >= 0; index-- {
		if anthropicMarkMessageCacheBreakpoint(messages[index]) {
			return
		}
	}
}

func anthropicMarkMessageCacheBreakpoint(message map[string]any) bool {
	switch content := message["content"].(type) {
	case string:
		message["content"] = []map[string]any{{
			"type":          "text",
			"text":          content,
			"cache_control": anthropicCacheControl(),
		}}
		return true
	case []map[string]any:
		for index := len(content) - 1; index >= 0; index-- {
			if !anthropicCacheableContentBlock(content[index]) {
				continue
			}
			content[index]["cache_control"] = anthropicCacheControl()
			return true
		}
	}
	return false
}

func anthropicCacheableContentBlock(block map[string]any) bool {
	switch block["type"] {
	case "text", "image", "tool_use", "tool_result":
		return true
	default:
		return false
	}
}
