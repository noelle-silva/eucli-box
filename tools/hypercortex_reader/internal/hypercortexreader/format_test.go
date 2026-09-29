package hypercortexreader

import (
	"strings"
	"testing"
)

func TestTruncateBodyPrefersLineBoundary(t *testing.T) {
	kept, truncated := truncateBody("第一行\n第二行\n第三行", 10)
	if !truncated || kept != "第一行\n第二行" {
		t.Fatalf("kept = %q, truncated = %v", kept, truncated)
	}
}

func TestTruncateBodyFallsBackToRuneCutForLongSingleLine(t *testing.T) {
	kept, truncated := truncateBody(strings.Repeat("字", 50), 10)
	if !truncated || len([]rune(kept)) != 10 {
		t.Fatalf("kept runes = %d, truncated = %v", len([]rune(kept)), truncated)
	}
}

func TestTruncateBodyKeepsShortTextWhole(t *testing.T) {
	if kept, truncated := truncateBody("短文本", 10); truncated || kept != "短文本" {
		t.Fatalf("kept = %q, truncated = %v", kept, truncated)
	}
}

// 信息条不计入正文预算、永久完整；正文按预算截断，总输出允许超出预算（正文＋信息条）。
func TestComposeContentKeepsEnvelopeCompleteAndBodyWithinBudget(t *testing.T) {
	body := strings.Repeat("正文内容很长\n", 40)
	content, truncated := composeContent(body, "search_notes", "notes", []resultFact{intFact("count", 3)}, 100)
	if !truncated {
		t.Fatal("long body must be truncated")
	}
	index := strings.LastIndex(content, "\n[hypercortex_reader]")
	if index < 0 {
		t.Fatalf("envelope missing: %q", content)
	}
	bodyPart := content[:index]
	if runes := len([]rune(bodyPart)); runes > 100 {
		t.Fatalf("body runes = %d, want <= 100", runes)
	}
	for _, fragment := range []string{"[hypercortex_reader]", "action=search_notes", "repo=notes", "count=3", "truncated=true"} {
		if !strings.Contains(content, fragment) {
			t.Fatalf("envelope missing %q: %q", fragment, content)
		}
	}
}
