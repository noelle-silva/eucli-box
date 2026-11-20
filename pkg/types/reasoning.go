package types

import "strings"

// ReasoningEffort 是全局统一的思考等级词汇表。
//
// 取值域收敛为各厂商共同接受的交集：auto 与 low / medium / high / xhigh / max。
// 其中 auto 的语义是"不表态"：适配器不得向任何上游发送思考相关字段，
// 由模型自身的默认行为决定思考程度。它不是一个可以换算成预算的档位，
// 因此换算函数必须显式处理它，而不能落进 default 分支。
//
// 历史上曾存在 minimal 与 ultra 两档，实测在 OpenAI 与 Anthropic 两种
// 现行协议下都会被上游以 400 拒绝（不在其接受的值域内），故整档移除。
type ReasoningEffort string

const (
	ReasoningEffortAuto   ReasoningEffort = "auto"
	ReasoningEffortLow    ReasoningEffort = "low"
	ReasoningEffortMedium ReasoningEffort = "medium"
	ReasoningEffortHigh   ReasoningEffort = "high"
	ReasoningEffortXHigh  ReasoningEffort = "xhigh"
	ReasoningEffortMax    ReasoningEffort = "max"

	DefaultReasoningEffort ReasoningEffort = ReasoningEffortMedium
)

// reasoningEffortLegacyAliases 记录词汇表统一前落库的旧取值，只在解析入口归一一次。
// 已移除档位（minimal / ultra）不在此列：它们不是同义改写，而是被删掉的能力档位，
// 静默映射会改变使用者的真实意图，因此交由调用方按非法输入快速失败。
var reasoningEffortLegacyAliases = map[ReasoningEffort]ReasoningEffort{
	"very_low":  ReasoningEffortLow,
	"very_high": ReasoningEffortXHigh,
}

// CanonicalReasoningEffort 把任意输入归一为规范等级。
// 空串与非法值一律返回空串，由调用方决定这是"清除覆盖"还是"参数非法"。
func CanonicalReasoningEffort(value ReasoningEffort) ReasoningEffort {
	trimmed := TrimReasoningEffort(value)
	if alias, ok := reasoningEffortLegacyAliases[trimmed]; ok {
		return alias
	}
	if isCanonicalReasoningEffort(trimmed) {
		return trimmed
	}
	return ""
}

func TrimReasoningEffort(value ReasoningEffort) ReasoningEffort {
	return ReasoningEffort(strings.TrimSpace(string(value)))
}

func isCanonicalReasoningEffort(value ReasoningEffort) bool {
	switch value {
	case ReasoningEffortAuto,
		ReasoningEffortLow,
		ReasoningEffortMedium,
		ReasoningEffortHigh,
		ReasoningEffortXHigh,
		ReasoningEffortMax:
		return true
	default:
		return false
	}
}

// IsReasoningEffort 判断输入能否解析成规范等级（历史别名视为合法）。
func IsReasoningEffort(value ReasoningEffort) bool {
	return CanonicalReasoningEffort(value) != ""
}

// NormalizeReasoningEffort 把输入归一为规范等级，非法时依次退到 fallback 与默认等级。
func NormalizeReasoningEffort(value ReasoningEffort, fallback ReasoningEffort) ReasoningEffort {
	if canonical := CanonicalReasoningEffort(value); canonical != "" {
		return canonical
	}
	if canonical := CanonicalReasoningEffort(fallback); canonical != "" {
		return canonical
	}
	return DefaultReasoningEffort
}
