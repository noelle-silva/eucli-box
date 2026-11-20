package modelprovider

import (
	"eucli-box/pkg/types"
	"strings"
	"testing"
)

// TestAnthropicStopReasonErrorFailsFast 守卫"不再静默吞掉空回复"：
// 拒答与截断必须变成错误，正常结束必须放行。
func TestAnthropicStopReasonErrorFailsFast(t *testing.T) {
	cases := []struct {
		name        string
		stopReason  string
		wantFailure bool
	}{
		{"refusal 必须报错", "refusal", true},
		{"max_tokens 必须报错", "max_tokens", true},
		{"end_turn 正常", "end_turn", false},
		{"tool_use 正常", "tool_use", false},
		{"stop_sequence 正常", "stop_sequence", false},
		{"pause_turn 正常", "pause_turn", false},
		{"空值不猜", "", false},
	}
	for _, tc := range cases {
		err := anthropicStopReasonError(tc.stopReason, 8192, 8192)
		if tc.wantFailure && err == nil {
			t.Fatalf("%s: 期望报错，实际放行", tc.name)
		}
		if !tc.wantFailure && err != nil {
			t.Fatalf("%s: 不该报错，实际 %v", tc.name, err)
		}
	}
}

// TestAnthropicStopReasonErrorCarriesDiagnostics 报错要能说清为什么空。
func TestAnthropicStopReasonErrorCarriesDiagnostics(t *testing.T) {
	err := anthropicStopReasonError("max_tokens", 8192, 8192)
	if err == nil {
		t.Fatal("max_tokens 应报错")
	}
	message := err.Error()
	for _, want := range []string{"上限", "截断"} {
		if !strings.Contains(message, want) {
			t.Fatalf("错误信息缺少关键词 %q：%s", want, message)
		}
	}
}

// TestAnthropicMaxOutputTokensIsLargeEnough 守卫上限本身不再成为瓶颈：
// 思考与正文共用配额，上限必须远大于任何档位的思考预算。
func TestAnthropicMaxOutputTokensIsLargeEnough(t *testing.T) {
	var largestBudget int
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		if budget := anthropicThinkingBudgetTokens(types.ReasoningEffort(effort)); budget > largestBudget {
			largestBudget = budget
		}
	}
	if anthropicMaxOutputTokens <= largestBudget*2 {
		t.Fatalf("max_tokens = %d 相对最大思考预算 %d 余量不足，正文会被思考挤空",
			anthropicMaxOutputTokens, largestBudget)
	}
}
