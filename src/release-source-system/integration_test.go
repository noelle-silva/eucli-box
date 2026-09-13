package releasesourcesystem

import (
	"context"
	"testing"

	"eucli-box/pkg/installsource"
	"eucli-box/pkg/types"
)

// TestRealCheckerReadsIndexThroughNarrowInterface 用真实 Checker 与本地假索引服务
// 验证「读已装事实 + 读分类候选」全链：候选清单身份、版本、比对结论来自真实索引读取。
func TestRealCheckerReadsIndexThroughNarrowInterface(t *testing.T) {
	fixture := newReleaseFixture(t)
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.2")
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "web_search"}, "0.1.9")
	checker := fixture.checker(t)
	system, err := NewSystemWithChecker(
		Config{BoxVersion: "0.1.0", CurrentSource: func() installsource.Kind { return installsource.KindOfficial }},
		checker,
		&fakeToolSystem{tools: []types.ToolSummary{
			{ID: "context7", Version: "0.1.0", Status: types.ToolAvailabilityActive, EucliBoxCompatibility: compatibility("0.1.0", "0.2.0")},
			{ID: "web_search", Version: "0.1.9", Status: types.ToolAvailabilityActive, EucliBoxCompatibility: compatibility("0.1.0", "0.2.0")},
		}},
		&fakePluginSystem{},
	)
	if err != nil {
		t.Fatalf("NewSystemWithChecker() error = %v", err)
	}
	list, err := system.ListCandidates(context.Background(), types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("ListCandidates() error = %v", err)
	}
	context7 := findCandidate(t, list, types.ReleaseArtifactKindTool, "context7")
	if context7.LatestVersion != "0.1.2" || !context7.Installed || context7.CurrentVersion != "0.1.0" || !context7.UpdateAvailable {
		t.Fatalf("context7 = %#v", context7)
	}
	webSearch := findCandidate(t, list, types.ReleaseArtifactKindTool, "web_search")
	if webSearch.LatestVersion != "0.1.9" || webSearch.UpdateAvailable {
		t.Fatalf("web_search = %#v", webSearch)
	}
	if fixture.kindRequests[types.ReleaseArtifactKindTool] != 1 || len(fixture.kindRequests) != 1 {
		t.Fatalf("index requests = %#v", fixture.kindRequests)
	}
}
