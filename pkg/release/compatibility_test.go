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

func TestValidateDevelopmentVersionKeepsSourceBaseline(t *testing.T) {
	if err := ValidateDevelopmentVersion("0.2.0", "0.2.0.1"); err != nil {
		t.Fatalf("ValidateDevelopmentVersion() error = %v", err)
	}
	for _, development := range []string{"0.1.0.1", "0.2.0", "0.2.1.1"} {
		if err := ValidateDevelopmentVersion("0.2.0", development); err == nil {
			t.Fatalf("ValidateDevelopmentVersion(0.2.0, %q) error = nil", development)
		}
	}
	if err := ValidateDevelopmentVersion("0.2.0.1", "0.2.0.1"); err == nil {
		t.Fatal("ValidateDevelopmentVersion() should reject a development baseline")
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

func TestNextFormalVersion(t *testing.T) {
	tests := []struct {
		value string
		level VersionLevel
		want  string
	}{
		{value: "0.1.2", level: LevelPatch, want: "0.1.3"},
		{value: "0.1.9", level: LevelPatch, want: "0.1.10"},
		{value: "0.1.2", level: LevelMinor, want: "0.2.0"},
		{value: "0.1.2", level: LevelMajor, want: "1.0.0"},
		{value: "1.9.9", level: LevelMajor, want: "2.0.0"},
	}
	for _, test := range tests {
		got, err := NextFormalVersion(test.value, test.level)
		if err != nil {
			t.Fatalf("NextFormalVersion(%q, %q) error = %v", test.value, test.level, err)
		}
		if got != test.want {
			t.Fatalf("NextFormalVersion(%q, %q) = %q, want %q", test.value, test.level, got, test.want)
		}
	}
	for _, value := range []string{"0.1", "0.1.2.1", "", "01.2.3"} {
		if _, err := NextFormalVersion(value, LevelPatch); err == nil {
			t.Fatalf("NextFormalVersion(%q) error = nil", value)
		}
	}
	if _, err := NextFormalVersion("0.1.2", VersionLevel("tiny")); err == nil {
		t.Fatal("NextFormalVersion() should reject unknown levels")
	}
}

func TestParseVersionLevel(t *testing.T) {
	for _, test := range []struct {
		text string
		want VersionLevel
	}{
		{text: "patch", want: LevelPatch},
		{text: "minor", want: LevelMinor},
		{text: "major", want: LevelMajor},
	} {
		got, err := ParseVersionLevel(test.text)
		if err != nil {
			t.Fatalf("ParseVersionLevel(%q) error = %v", test.text, err)
		}
		if got != test.want {
			t.Fatalf("ParseVersionLevel(%q) = %q, want %q", test.text, got, test.want)
		}
	}
	for _, text := range []string{"", "tiny", "Patch"} {
		if _, err := ParseVersionLevel(text); err == nil {
			t.Fatalf("ParseVersionLevel(%q) error = nil", text)
		}
	}
}
