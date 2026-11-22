package modelprovider

import (
	"reflect"
	"testing"

	"eucli-box/pkg/types"
)

// 守卫「按协议隔离思维链」：明文与签名/加密体必须来自同一 source（"an" / "op"），
// 同进同退；别家的思考不会被并进自己协议的载体，也不会伪造出无来源的载体。

func reasoningPart(source string, text string, signature string, data string) types.MessagePart {
	return types.MessagePart{Type: "reasoning", Source: source, Text: text, Signature: signature, Data: data}
}

func assistantPromptMessage(content string, parts ...types.MessagePart) types.PromptMessage {
	return types.PromptMessage{Role: "assistant", Content: content, Parts: parts}
}

func anthropicTextContent(text string) []map[string]any {
	return []map[string]any{{"type": "text", "text": text}}
}

func anthropicThinkingBlocks(t *testing.T, content any) []map[string]any {
	t.Helper()
	blocks, ok := content.([]map[string]any)
	if !ok {
		t.Fatalf("Anthropic content 不是内容块列表: %#v", content)
	}
	thinking := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		if block["type"] == "thinking" {
			thinking = append(thinking, block)
		}
	}
	return thinking
}

func TestAnthropicReasoningContentDropsOtherSourceText(t *testing.T) {
	// 从 OpenAI 切到 Claude：OpenAI 的明文不能被并进 thinking 块，
	// 也不能因为别家明文而凭空造出一个没有签名的 thinking 块。
	message := assistantPromptMessage("正式回答", reasoningPart("op", "OpenAI 的思路", "", "op-cipher"))

	content := anthropicTextContent("正式回答")
	got := appendAnthropicReasoningContent(content, message)

	if blocks := anthropicThinkingBlocks(t, got); len(blocks) != 0 {
		t.Fatalf("无 an 来源记录时不应生成 thinking 块: %#v", blocks)
	}
	if !reflect.DeepEqual(got, anthropicTextContent("正式回答")) {
		t.Fatalf("正文应原样保留: %#v", got)
	}
}

func TestAnthropicReasoningContentWithoutAnyRecordKeepsContent(t *testing.T) {
	// 完全没有思维链记录时，直接返回原 content，不做任何伪造。
	message := assistantPromptMessage("正式回答")

	if got := appendAnthropicReasoningContent("正式回答", message); got != "正式回答" {
		t.Fatalf("无任何思维链记录时应原样返回正文, got %#v", got)
	}
	if got := appendAnthropicReasoningContent(anthropicTextContent("正式回答"), message); !reflect.DeepEqual(got, anthropicTextContent("正式回答")) {
		t.Fatalf("无任何思维链记录时不应新增内容块: %#v", got)
	}
}

func TestAnthropicReasoningContentCarriesAnSourceTextAndSignature(t *testing.T) {
	message := assistantPromptMessage("正式回答", reasoningPart("an", "Claude 的思路", "an-signature", "an-cipher"))

	got := appendAnthropicReasoningContent(anthropicTextContent("正式回答"), message)

	blocks := anthropicThinkingBlocks(t, got)
	if len(blocks) != 1 {
		t.Fatalf("thinking 块数量 = %d, want 1: %#v", len(blocks), got)
	}
	thinking := blocks[0]
	if thinking["thinking"] != "Claude 的思路" {
		t.Fatalf("thinking 明文 = %#v", thinking["thinking"])
	}
	if thinking["signature"] != "an-signature" {
		t.Fatalf("thinking 签名 = %#v", thinking["signature"])
	}
	if thinking["data"] != "an-cipher" {
		t.Fatalf("thinking 载体 = %#v", thinking["data"])
	}
}

func TestOpenAIAssistantMessageDropsOtherSourceText(t *testing.T) {
	// 对称守卫：Claude 的明文与签名都不进 OpenAI 的 reasoning_content / reasoning。
	message := assistantPromptMessage("正式回答", reasoningPart("an", "Claude 的思路", "an-signature", "an-cipher"))

	assistant := openAIAssistantMessage(message)

	if reasoning, ok := assistant["reasoning_content"]; ok {
		t.Fatalf("无 op 来源记录时不应发 reasoning_content: %#v", reasoning)
	}
	if reasoning, ok := assistant["reasoning"]; ok {
		t.Fatalf("无 op 来源记录时不应发 reasoning 载体: %#v", reasoning)
	}
}

func TestOpenAIAssistantMessageWithoutAnyRecordSendsNoReasoning(t *testing.T) {
	message := assistantPromptMessage("正式回答")

	assistant := openAIAssistantMessage(message)

	if _, ok := assistant["reasoning_content"]; ok {
		t.Fatalf("无任何思维链记录时不应发 reasoning_content: %#v", assistant)
	}
	if _, ok := assistant["reasoning"]; ok {
		t.Fatalf("无任何思维链记录时不应发 reasoning 载体: %#v", assistant)
	}
}

func TestOpenAIAssistantMessageCarriesOpSourceTextAndCipher(t *testing.T) {
	message := assistantPromptMessage("正式回答", reasoningPart("op", "OpenAI 的思路", "", "op-cipher"))

	assistant := openAIAssistantMessage(message)

	if reasoning, ok := assistant["reasoning_content"]; !ok || reasoning != "OpenAI 的思路" {
		t.Fatalf("reasoning_content = %#v", assistant["reasoning_content"])
	}
	reasoning, ok := assistant["reasoning"].(map[string]any)
	if !ok || reasoning["encrypted_content"] != "op-cipher" {
		t.Fatalf("reasoning 载体 = %#v", assistant["reasoning"])
	}
}

func TestReasoningSourceIsolationSplitsMixedMessage(t *testing.T) {
	// 同一消息里既有 an 又有 op：各自只取自己那份，不互相串味。
	message := assistantPromptMessage(
		"正式回答",
		reasoningPart("op", "OpenAI 的思路", "", "op-cipher"),
		reasoningPart("an", "Claude 的第一段", "an-signature", "an-cipher"),
		reasoningPart("an", "Claude 的第二段", "", ""),
	)

	anthropicBlocks := anthropicThinkingBlocks(t, appendAnthropicReasoningContent(anthropicTextContent("正式回答"), message))
	if len(anthropicBlocks) != 1 {
		t.Fatalf("thinking 块数量 = %d, want 1: %#v", len(anthropicBlocks), anthropicBlocks)
	}
	if got := anthropicBlocks[0]["thinking"]; got != "Claude 的第一段\n\nClaude 的第二段" {
		t.Fatalf("thinking 明文 = %#v", got)
	}
	if got := anthropicBlocks[0]["signature"]; got != "an-signature" {
		t.Fatalf("thinking 签名 = %#v", got)
	}
	if got := anthropicBlocks[0]["data"]; got != "an-cipher" {
		t.Fatalf("thinking 载体 = %#v", got)
	}

	assistant := openAIAssistantMessage(message)
	if got := assistant["reasoning_content"]; got != "OpenAI 的思路" {
		t.Fatalf("reasoning_content = %#v", got)
	}
	reasoning, ok := assistant["reasoning"].(map[string]any)
	if !ok || reasoning["encrypted_content"] != "op-cipher" {
		t.Fatalf("reasoning 载体 = %#v", assistant["reasoning"])
	}
}
