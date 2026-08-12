package releaseverify

import (
	"regexp"
	"strings"
	"testing"
)

func TestEvidenceLogFileNameIsStableAndDistinct(t *testing.T) {
	first := evidenceLogFileName("官方来源、发行名称、重复发布与只读检查")
	if first != evidenceLogFileName("官方来源、发行名称、重复发布与只读检查") {
		t.Fatal("evidence log name is not stable")
	}
	second := evidenceLogFileName("业务端自动检查与维护入口")
	if first == second {
		t.Fatalf("distinct checks share a log name: %s", first)
	}
	if !strings.HasPrefix(first, "check-") {
		t.Fatalf("non-ASCII check did not receive a readable fallback: %s", first)
	}
}

func TestEvidenceLogFileNameIsSafeAndBounded(t *testing.T) {
	name := evidenceLogFileName(strings.Repeat("Release Check / ", 20))
	if len(name) > 109 {
		t.Fatalf("evidence log name is unexpectedly long: %d", len(name))
	}
	if !regexp.MustCompile(`^[a-z0-9_-]+-[a-f0-9]{64}\.log$`).MatchString(name) {
		t.Fatalf("evidence log name is not filesystem-safe: %s", name)
	}
}
