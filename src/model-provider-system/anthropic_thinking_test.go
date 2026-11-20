package modelprovider

import (
	"encoding/json"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// anthropicThinkingModeCases 覆盖两条轨道的思考形态构造。
func anthropicThinkingModeCases() []struct {
	name string
	mode anthropicThinkingMode
} {
	return []struct {
		name string
		mode anthropicThinkingMode
	}{
		{"latest", anthropicThinkingAdaptive},
		{"legacy", anthropicThinkingBudget},
	}
}

func buildThinkingBody(mode anthropicThinkingMode, effort types.ReasoningEffort) (map[string]any, int) {
	body := map[string]any{}
	maxTokens := anthropicThinkingBuilder{mode: mode}.apply(body, effort)
	return body, maxTokens
}

func TestAnthropicThinkingAutoOmitsThinkingFields(t *testing.T) {
	for _, tc := range anthropicThinkingModeCases() {
		body, maxTokens := buildThinkingBody(tc.mode, types.ReasoningEffortAuto)
		if _, ok := body["thinking"]; ok {
			t.Fatalf("%s: auto 档位不应发送 thinking 字段: %#v", tc.name, body["thinking"])
		}
		if _, ok := body["output_config"]; ok {
			t.Fatalf("%s: auto 档位不应发送 output_config 字段: %#v", tc.name, body["output_config"])
		}
		if maxTokens != anthropicAutoReplyTokens {
			t.Fatalf("%s: auto 档位 max_tokens = %d, want %d", tc.name, maxTokens, anthropicAutoReplyTokens)
		}
	}
}

func TestAnthropicThinkingEmptyEffortOmitsThinkingFields(t *testing.T) {
	for _, tc := range anthropicThinkingModeCases() {
		body, _ := buildThinkingBody(tc.mode, "")
		if _, ok := body["thinking"]; ok {
			t.Fatalf("%s: 未开启推理时不应发送 thinking 字段: %#v", tc.name, body["thinking"])
		}
	}
}

func TestAnthropicThinkingLatestUsesAdaptiveAndOutputConfig(t *testing.T) {
	body, maxTokens := buildThinkingBody(anthropicThinkingAdaptive, types.ReasoningEffortHigh)
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "adaptive" {
		t.Fatalf("thinking = %#v", body["thinking"])
	}
	if _, ok := thinking["budget_tokens"]; ok {
		t.Fatalf("adaptive 形态不应携带 budget_tokens: %#v", thinking)
	}
	outputConfig, ok := body["output_config"].(map[string]any)
	if !ok || outputConfig["effort"] != "high" {
		t.Fatalf("output_config = %#v", body["output_config"])
	}
	if maxTokens != anthropicAutoReplyTokens {
		t.Fatalf("adaptive 形态 max_tokens = %d, want %d", maxTokens, anthropicAutoReplyTokens)
	}
}

func TestAnthropicThinkingLegacyUsesBudgetTokens(t *testing.T) {
	body, maxTokens := buildThinkingBody(anthropicThinkingBudget, types.ReasoningEffortHigh)
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" || thinking["budget_tokens"] != 3072 {
		t.Fatalf("thinking = %#v", body["thinking"])
	}
	if _, ok := body["output_config"]; ok {
		t.Fatalf("legacy 形态不应携带 output_config: %#v", body["output_config"])
	}
	if maxTokens != 3072+anthropicReplyTokenReserve {
		t.Fatalf("legacy 形态 max_tokens = %d, want %d", maxTokens, 3072+anthropicReplyTokenReserve)
	}
}

// TestAdapterForWiresAnthropicThinkingMode 确认协议轨道确实落到对应思考形态，
// 这是"协议选择决定行为、代码里没有模型名判断"的守卫。
func TestAdapterForWiresAnthropicThinkingMode(t *testing.T) {
	cases := []struct {
		protocol types.ProviderProtocol
		want     anthropicThinkingMode
	}{
		{types.ProviderProtocolAnthropicLatest, anthropicThinkingAdaptive},
		{types.ProviderProtocolAnthropicLegacy, anthropicThinkingBudget},
	}
	for _, tc := range cases {
		adapter, err := adapterFor(tc.protocol)
		if err != nil {
			t.Fatalf("adapterFor(%s) error = %v", tc.protocol, err)
		}
		anthropic, ok := adapter.(anthropicAdapter)
		if !ok {
			t.Fatalf("adapterFor(%s) 未返回 anthropicAdapter", tc.protocol)
		}
		if anthropic.thinking.mode != tc.want {
			t.Fatalf("adapterFor(%s) thinking.mode = %d, want %d", tc.protocol, anthropic.thinking.mode, tc.want)
		}
	}
}

// TestOpenAIReasoningEffortOmitsAuto 确认 OpenAI 通道下 auto 与未开启推理都不发字段。
func TestOpenAIReasoningEffortOmitsAuto(t *testing.T) {
	if got := openAIReasoningEffort(types.ReasoningEffortAuto); got != "" {
		t.Fatalf("auto 应映射为空串，得到 %q", got)
	}
	if got := openAIReasoningEffort(""); got != "" {
		t.Fatalf("空档位应映射为空串，得到 %q", got)
	}
	if got := openAIReasoningEffort(types.ReasoningEffortMax); got != "max" {
		t.Fatalf("max 应原样下传，得到 %q", got)
	}
}

// TestReasoningEffortVocabularyMatchesProviderIntersection 守卫词表：
// 只保留各协议共同接受的值域，否则会直接把上游 400 漏给使用者。
func TestReasoningEffortVocabularyMatchesProviderIntersection(t *testing.T) {
	allowed := []types.ReasoningEffort{
		types.ReasoningEffortAuto,
		types.ReasoningEffortLow,
		types.ReasoningEffortMedium,
		types.ReasoningEffortHigh,
		types.ReasoningEffortXHigh,
		types.ReasoningEffortMax,
	}
	for _, effort := range allowed {
		if !types.IsReasoningEffort(effort) {
			t.Fatalf("词表应接受 %q", effort)
		}
	}
	for _, rejected := range []types.ReasoningEffort{"minimal", "ultra", "extreme"} {
		if types.IsReasoningEffort(rejected) {
			t.Fatalf("词表不应以规范档位接受 %q", rejected)
		}
	}
}

// TestBuildCompleteRequestBodyJSONShape 端到端守卫：两条轨道产出的 body
// 必须能被 JSON 编码，且不含交叉形态的字段。
func TestBuildCompleteRequestBodyJSONShape(t *testing.T) {
	for _, tc := range anthropicThinkingModeCases() {
		body, _ := buildThinkingBody(tc.mode, types.ReasoningEffortMedium)
		encoded, err := encodeJSONBody(body)
		if err != nil {
			t.Fatalf("%s: encodeJSONBody() error = %v", tc.name, err)
		}
		text := string(encoded)
		if tc.mode == anthropicThinkingAdaptive && strings.Contains(text, "budget_tokens") {
			t.Fatalf("%s: latest 形态混入了 budget_tokens: %s", tc.name, text)
		}
		if tc.mode == anthropicThinkingBudget && strings.Contains(text, "output_config") {
			t.Fatalf("%s: legacy 形态混入了 output_config: %s", tc.name, text)
		}
		var roundTrip map[string]any
		if err := json.Unmarshal(encoded, &roundTrip); err != nil {
			t.Fatalf("%s: 编码结果不可解析: %v", tc.name, err)
		}
	}
}
