package modelprovider

import "strings"

// Anthropic 的 stop_reason 说明本轮生成为什么停下。
//
// 绝大多数取值是正常终止，但有两类必须显式失败，否则会被上层当成
// 一次"成功的空回复"，让使用者面对一个没有任何解释的空白：
//
//   refusal            模型拒绝回答，响应体内容块可能完全为空；
//   max_tokens         撞到输出上限被截断，思考与正文共用配额时正文可能一个字都没有。
//
// 其余取值（end_turn / stop_sequence / tool_use / pause_turn）都是可预期的正常结束。

const (
	anthropicStopReasonRefusal   = "refusal"
	anthropicStopReasonMaxTokens = "max_tokens"
)

// anthropicStopReasonError 把需要让使用者知道的停止原因转成错误。
//
// 返回 nil 表示这是正常结束，调用方照常返回结果。
// 这里的判定只依赖停止原因本身，不猜测内容是否为空：
// 空内容可能是合法的（例如只有思考块），不能作为失败依据。
func anthropicStopReasonError(stopReason string, outputTokens int, thinkingTokens int) error {
	switch strings.TrimSpace(stopReason) {
	case anthropicStopReasonRefusal:
		return providerServiceFailedWithDetails(
			"模型拒绝回答本次请求",
			nil,
			map[string]any{"stopReason": stopReason},
		)
	case anthropicStopReasonMaxTokens:
		return providerServiceFailedWithDetails(
			"模型输出达到上限被截断，正文可能为空",
			nil,
			map[string]any{
				"stopReason":     stopReason,
				"outputTokens":   outputTokens,
				"thinkingTokens": thinkingTokens,
			},
		)
	default:
		return nil
	}
}
