package types

import "testing"

func TestCanonicalReasoningEffort(t *testing.T) {
	cases := []struct {
		input string
		want  ReasoningEffort
	}{
		{"minimal", ReasoningEffortMinimal},
		{"low", ReasoningEffortLow},
		{"medium", ReasoningEffortMedium},
		{"high", ReasoningEffortHigh},
		{"xhigh", ReasoningEffortXHigh},
		{"max", ReasoningEffortMax},
		{"ultra", ReasoningEffortUltra},
		{" high ", ReasoningEffortHigh},
		{"very_low", ReasoningEffortMinimal},
		{"very_high", ReasoningEffortXHigh},
		{"", ""},
		{"extreme", ""},
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
