package types

import "testing"

func TestCanonicalReasoningEffort(t *testing.T) {
	cases := []struct {
		input string
		want  ReasoningEffort
	}{
		{"auto", ReasoningEffortAuto},
		{"low", ReasoningEffortLow},
		{"medium", ReasoningEffortMedium},
		{"high", ReasoningEffortHigh},
		{"xhigh", ReasoningEffortXHigh},
		{"max", ReasoningEffortMax},
		{" high ", ReasoningEffortHigh},
		{"very_low", ReasoningEffortLow},
		{"very_high", ReasoningEffortXHigh},
		{"", ""},
		{"extreme", ""},
		// 已移除档位不再被接受：静默映射会改变使用者真实意图。
		{"minimal", ""},
		{"ultra", ""},
	}
	for _, tc := range cases {
		if got := CanonicalReasoningEffort(ReasoningEffort(tc.input)); got != tc.want {
			t.Fatalf("CanonicalReasoningEffort(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeReasoningEffortFallback(t *testing.T) {
	if got := NormalizeReasoningEffort("extreme", ""); got != DefaultReasoningEffort {
		t.Fatalf("NormalizeReasoningEffort(extreme) = %q, want default %q", got, DefaultReasoningEffort)
	}
	if got := NormalizeReasoningEffort("", ReasoningEffortMax); got != ReasoningEffortMax {
		t.Fatalf("NormalizeReasoningEffort(empty, max) = %q, want max", got)
	}
}
