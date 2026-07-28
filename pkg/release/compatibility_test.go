package release

import (
	"testing"

	"eucli-box/pkg/types"
)

func TestAssessEucliBoxCompatibilityAcceptsCurrentInitialRange(t *testing.T) {
	status := AssessEucliBoxCompatibility("0.1.0", "0.1.0", types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"})
	if !status.Compatible || status.Reason != "" {
		t.Fatalf("status = %#v", status)
	}
}

func TestAssessEucliBoxCompatibilityRejectsRangeBoundaries(t *testing.T) {
	compatibility := types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}
	for _, current := range []string{"0.0.9", "0.2.0"} {
		status := AssessEucliBoxCompatibility("0.1.0", current, compatibility)
		if status.Compatible || status.Reason == "" {
			t.Fatalf("current %s status = %#v", current, status)
		}
	}
}

func TestAssessEucliBoxCompatibilityRejectsInvalidMetadata(t *testing.T) {
	status := AssessEucliBoxCompatibility("0.1", "0.1.0", types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"})
	if status.Compatible || status.Reason == "" {
		t.Fatalf("invalid artifact status = %#v", status)
	}
	status = AssessEucliBoxCompatibility("0.1.0", "0.1.0", types.EucliBoxCompatibility{MinimumVersion: "0.2.0", MaximumVersionExclusive: "0.1.0"})
	if status.Compatible || status.Reason == "" {
		t.Fatalf("invalid range status = %#v", status)
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{left: "0.1.0", right: "0.1.0", want: 0},
		{left: "0.1.1", right: "0.1.0", want: 1},
		{left: "0.2.0", right: "0.10.0", want: -1},
		{left: "1.0.0", right: "0.99.99", want: 1},
	}
	for _, test := range tests {
		got, err := CompareVersions(test.left, test.right)
		if err != nil {
			t.Fatalf("CompareVersions(%q, %q) error = %v", test.left, test.right, err)
		}
		if got != test.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
	if _, err := CompareVersions("0.1", "0.1.0"); err == nil {
		t.Fatal("CompareVersions() should reject invalid versions")
	}
}
