package modelprovider

import "eucli-box/pkg/types"

// Anthropic 的思考参数存在两代互不兼容的形态，本文件是唯一的构造入口。
//
// 两代形态不是"新旧版本"关系，而是不同模型各自只接受其中一种：
//   adaptive（latest）：thinking.type=adaptive + output_config.effort，新一代模型
//                       唯一接受；省略 thinking 时模型同样会自行思考。
//   budget（legacy）  ：thinking.type=enabled + budget_tokens，老模型专用。
//
// auto 档位在两代形态下语义一致：不发送任何思考字段，交给模型默认行为。
// 因此自动档位不区分轨道，也不参与预算换算。

const (
	// anthropicReplyTokenReserve 是思考预算之外留给正文输出的 token 余量。
	anthropicReplyTokenReserve = 4096

	// anthropicAutoReplyTokens 是 auto 档位的输出上限。
	// auto 不承诺思考预算，固定上限即为其完整语义，不再叠加任何余量。
	anthropicAutoReplyTokens = 8192
)

// anthropicThinkingMode 标识一条 Anthropic 轨道使用哪一代思考参数形态。
type anthropicThinkingMode int

const (
	// anthropicThinkingAdaptive 使用 adaptive + output_config.effort。
	anthropicThinkingAdaptive anthropicThinkingMode = iota
	// anthropicThinkingBudget 使用 enabled + budget_tokens。
	anthropicThinkingBudget
)

// anthropicThinkingBuilder 把统一的思考档位落成某一条轨道的思考参数，
// 并给出该档位对应的输出上限。
type anthropicThinkingBuilder struct {
	mode anthropicThinkingMode
}

// apply 把思考档位写入请求体。
//
// effort 为空表示本次调用不需要思考（模型未开启推理能力），
// auto 表示不表态。这两种情况都不得发送任何思考字段。
func (b anthropicThinkingBuilder) apply(body map[string]any, effort types.ReasoningEffort) int {
	normalized := types.NormalizeReasoningEffort(effort, types.DefaultReasoningEffort)
	if effort == "" || normalized == types.ReasoningEffortAuto {
		return anthropicAutoReplyTokens
	}
	switch b.mode {
	case anthropicThinkingBudget:
		return b.applyBudget(body, normalized)
	default:
		return b.applyAdaptive(body, normalized)
	}
}

// applyAdaptive 落成新一代形态：思考程度由 effort 表达，不再有预算概念，
// 输出上限因此是一个固定值。
func (b anthropicThinkingBuilder) applyAdaptive(body map[string]any, effort types.ReasoningEffort) int {
	body["thinking"] = map[string]any{"type": "adaptive"}
	body["output_config"] = map[string]any{"effort": string(effort)}
	return anthropicAutoReplyTokens
}

// applyBudget 落成老一代形态：思考预算由 budget_tokens 表达。
// Anthropic 要求预算小于 max_tokens，输出上限因此由预算推导。
func (b anthropicThinkingBuilder) applyBudget(body map[string]any, effort types.ReasoningEffort) int {
	budget := anthropicThinkingBudgetTokens(effort)
	body["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
	return budget + anthropicReplyTokenReserve
}

// anthropicThinkingBudgetTokens 把统一档位换算成老一代的预算 token 数。
// auto 不参与换算：它表示不表态，由 apply 提前返回。
func anthropicThinkingBudgetTokens(effort types.ReasoningEffort) int {
	switch types.NormalizeReasoningEffort(effort, types.DefaultReasoningEffort) {
	case types.ReasoningEffortLow:
		return 1536
	case types.ReasoningEffortHigh:
		return 3072
	case types.ReasoningEffortXHigh:
		return 4096
	case types.ReasoningEffortMax:
		return 6144
	default:
		return 2048
	}
}
