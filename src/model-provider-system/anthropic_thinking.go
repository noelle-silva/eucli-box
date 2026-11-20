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

// anthropicMaxOutputTokens 是本适配器统一使用的输出上限。
//
// 取 64000 的依据是实测：该值对全部受支持的 Claude 模型都被接受；
// 更高的 128000 只在新一代模型可用，老模型会拒绝。选用统一值而非按模型分支，
// 是因为上限本身不该成为我们的限制——它只需要"足够大且不会触发报错"。
//
// 关键点：思考 token 计入 max_tokens。上限给小会让深度思考吃光配额，
// 正文一个字都分不到，表现为「思考很久后返回空回复」。因此这里不按档位
// 压低头寸，一律给到统一上限，由模型自行决定实际用量。
const anthropicMaxOutputTokens = 64000

// anthropicThinkingMode 标识一条 Anthropic 轨道使用哪一代思考参数形态。
type anthropicThinkingMode int

const (
	// anthropicThinkingAdaptive 使用 adaptive + output_config.effort。
	anthropicThinkingAdaptive anthropicThinkingMode = iota
	// anthropicThinkingBudget 使用 enabled + budget_tokens。
	anthropicThinkingBudget
)

// anthropicThinkingBuilder 把统一的思考档位落成某一条轨道的思考参数。
//
// 输出上限始终是 anthropicMaxOutputTokens：思考与正文共用同一份配额时，
// 替模型预设一个"够用"的小额度只会让我们成为瓶颈。
type anthropicThinkingBuilder struct {
	mode anthropicThinkingMode
}

// apply 把思考档位写入请求体，返回本次请求的 max_tokens。
//
// effort 为空表示本次调用不需要思考（模型未开启推理能力），
// auto 表示不表态。这两种情况都不得发送任何思考字段。
func (b anthropicThinkingBuilder) apply(body map[string]any, effort types.ReasoningEffort) int {
	normalized := types.NormalizeReasoningEffort(effort, types.DefaultReasoningEffort)
	if effort == "" || normalized == types.ReasoningEffortAuto {
		return anthropicMaxOutputTokens
	}
	switch b.mode {
	case anthropicThinkingBudget:
		b.applyBudget(body, normalized)
	default:
		b.applyAdaptive(body, normalized)
	}
	return anthropicMaxOutputTokens
}

// applyAdaptive 落成新一代形态：思考程度由 effort 表达，没有预算概念。
func (b anthropicThinkingBuilder) applyAdaptive(body map[string]any, effort types.ReasoningEffort) {
	body["thinking"] = map[string]any{"type": "adaptive"}
	body["output_config"] = map[string]any{"effort": string(effort)}
}

// applyBudget 落成老一代形态：思考预算由 budget_tokens 表达。
// 预算必须小于 max_tokens，统一上限远大于任何档位的预算，因此始终成立。
func (b anthropicThinkingBuilder) applyBudget(body map[string]any, effort types.ReasoningEffort) {
	body["thinking"] = map[string]any{
		"type":          "enabled",
		"budget_tokens": anthropicThinkingBudgetTokens(effort),
	}
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
