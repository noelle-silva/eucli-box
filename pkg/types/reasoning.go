package types

import "strings"

type ReasoningEffort string

const (
	ReasoningEffortVeryLow  ReasoningEffort = "very_low"
	ReasoningEffortLow      ReasoningEffort = "low"
	ReasoningEffortMedium   ReasoningEffort = "medium"
	ReasoningEffortHigh     ReasoningEffort = "high"
	ReasoningEffortVeryHigh ReasoningEffort = "very_high"

	DefaultReasoningEffort ReasoningEffort = ReasoningEffortMedium
)

func TrimReasoningEffort(value ReasoningEffort) ReasoningEffort {
	return ReasoningEffort(strings.TrimSpace(string(value)))
}

func IsReasoningEffort(value ReasoningEffort) bool {
	switch TrimReasoningEffort(value) {
	case ReasoningEffortVeryLow, ReasoningEffortLow, ReasoningEffortMedium, ReasoningEffortHigh, ReasoningEffortVeryHigh:
		return true
	default:
		return false
	}
}

func NormalizeReasoningEffort(value ReasoningEffort, fallback ReasoningEffort) ReasoningEffort {
	value = TrimReasoningEffort(value)
	if IsReasoningEffort(value) {
		return value
	}
	fallback = TrimReasoningEffort(fallback)
	if IsReasoningEffort(fallback) {
		return fallback
	}
	return DefaultReasoningEffort
}
