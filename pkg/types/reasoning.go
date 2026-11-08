package types

import "strings"

// ReasoningEffort 是全局统一的思考等级词汇表。
// 取值直接采用社区通用的厂商原词（minimal / low / medium / high / xhigh / max / ultra），
// 存储、网关入参、客户端展示共用同一套字面量，不再维护中文档位与英文档位之间的二次翻译。
type ReasoningEffort string

const (
	ReasoningEffortMinimal ReasoningEffort = "minimal"
	ReasoningEffortLow     ReasoningEffort = "low"
	ReasoningEffortMedium  ReasoningEffort = "medium"
	ReasoningEffortHigh    ReasoningEffort = "high"
	ReasoningEffortXHigh   ReasoningEffort = "xhigh"
	ReasoningEffortMax     ReasoningEffort = "max"
	ReasoningEffortUltra   ReasoningEffort = "ultra"

	DefaultReasoningEffort ReasoningEffort = ReasoningEffortMedium
)

// reasoningEffortLegacyAliases 记录词汇表统一前落库的旧取值，只在解析入口归一一次。
var reasoningEffortLegacyAliases = map[ReasoningEffort]ReasoningEffort{
	"very_low":  ReasoningEffortMinimal,
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
	case ReasoningEffortMinimal,
		ReasoningEffortLow,
		ReasoningEffortMedium,
		ReasoningEffortHigh,
		ReasoningEffortXHigh,
		ReasoningEffortMax,
		ReasoningEffortUltra:
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
