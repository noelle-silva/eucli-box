package placeholder

import (
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func TestResolveExpandsRegisteredPlaceholdersRecursively(t *testing.T) {
	library := types.PlaceholderLibrary{Placeholders: []types.PlaceholderItem{{Name: "名字", Value: "晶晶"}, {Name: "问候", Value: "你好，{{ 名字 }}"}}}
	result := Resolve("{{问候}}！{{未知}}", library)
	if result.Text != "你好，晶晶！{{未知}}" {
		t.Fatalf("resolved text = %q", result.Text)
	}
	if len(result.Problems) != 0 {
		t.Fatalf("problems = %#v", result.Problems)
	}
}

func TestResolveKeepsCycleTokensAndReportsProblems(t *testing.T) {
	library := types.PlaceholderLibrary{Placeholders: []types.PlaceholderItem{{Name: "A", Value: "{{B}}"}, {Name: "B", Value: "{{A}}"}}}
	result := Resolve("start {{A}} end", library)
	if result.Text != "start {{A}} end" {
		t.Fatalf("resolved text = %q", result.Text)
	}
	joined := problemPairs(result.Problems)
	if !strings.Contains(joined, "A:cycle_reference") || !strings.Contains(joined, "B:cycle_reference") {
		t.Fatalf("problems = %#v", result.Problems)
	}
}

func TestResolveKeepsDuplicateNameTokenAndReportsProblem(t *testing.T) {
	library := types.PlaceholderLibrary{Placeholders: []types.PlaceholderItem{{Name: "A", Value: "one"}, {Name: "A", Value: "two"}}}
	result := Resolve("{{A}}", library)
	if result.Text != "{{A}}" {
		t.Fatalf("resolved text = %q", result.Text)
	}
	if len(result.Problems) != 1 || result.Problems[0].Name != "A" || result.Problems[0].Type != types.PlaceholderProblemDuplicateName {
		t.Fatalf("problems = %#v", result.Problems)
	}
}

func problemPairs(problems []types.PlaceholderProblem) string {
	pairs := make([]string, 0, len(problems))
	for _, problem := range problems {
		pairs = append(pairs, problem.Name+":"+problem.Type)
	}
	return strings.Join(pairs, ",")
}
