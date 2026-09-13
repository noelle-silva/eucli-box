package releasesourcesystem

import (
	"context"
	"fmt"
	"testing"
	"time"

	"eucli-box/pkg/installsource"
	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

func TestListInstallationsReadsBoxToolsAndPlugins(t *testing.T) {
	system := newTestSystem(t, testOptions{
		tools: []types.ToolSummary{
			{ID: "context7", Version: "0.1.2", Status: types.ToolAvailabilityActive, EucliBoxCompatibility: compatibility("0.1.0", "0.2.0")},
			{ID: "unavailable", Version: "0.1.0", Status: types.ToolAvailabilityUnavailable, EucliBoxCompatibility: compatibility("0.1.0", "0.2.0")},
		},
		plugins: []types.SystemPluginSummary{
			{ID: "time-plugin", Version: "0.1.0", Installed: true, EucliBoxCompatibility: compatibility("0.1.0", "0.2.0")},
			{ID: "not-installed", Version: "0.1.0", Installed: false, EucliBoxCompatibility: compatibility("0.1.0", "0.2.0")},
		},
	})
	list, err := system.ListInstallations(context.Background())
	if err != nil {
		t.Fatalf("ListInstallations() error = %v", err)
	}
	box := findInstallation(t, list, types.ReleaseArtifactKindBox, types.ReleaseArtifactKindBox)
	if box.Version != "0.1.0" {
		t.Fatalf("box = %#v", box)
	}
	context7 := findInstallation(t, list, types.ReleaseArtifactKindTool, "context7")
	if context7.Version != "0.1.2" || context7.Compatibility == nil {
		t.Fatalf("context7 = %#v", context7)
	}
	findInstallation(t, list, types.ReleaseArtifactKindPlugin, "time-plugin")
	for _, item := range list.Artifacts {
		if item.Artifact.ID == "unavailable" || item.Artifact.ID == "not-installed" {
			t.Fatalf("list included non-active artifact: %#v", item)
		}
	}
}

func TestListInstallationsReadsFailuresAsFacts(t *testing.T) {
	system := newTestSystem(t, testOptions{toolErr: fmt.Errorf("tool store down")})
	list, err := system.ListInstallations(context.Background())
	if err != nil {
		t.Fatalf("ListInstallations() error = %v", err)
	}
	tool := findInstallation(t, list, types.ReleaseArtifactKindTool, types.ReleaseArtifactKindTool)
	if tool.FailureReason == "" {
		t.Fatalf("tool failure = %#v", tool)
	}
}

func TestListCandidatesComparesInstalledFacts(t *testing.T) {
	system := newTestSystem(t, testOptions{
		tools: []types.ToolSummary{
			{ID: "context7", Version: "0.1.0", Status: types.ToolAvailabilityActive, EucliBoxCompatibility: compatibility("0.1.0", "0.2.0")},
			{ID: "web_search", Version: "0.1.9", Status: types.ToolAvailabilityActive, EucliBoxCompatibility: compatibility("0.1.0", "0.2.0")},
		},
		candidates: map[string][]releasecheck.CandidateRecord{
			types.ReleaseArtifactKindTool: {
				{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, Candidate: candidate(types.ReleaseArtifactKindTool, "context7", "0.1.2")},
				{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "web_search"}, Candidate: candidate(types.ReleaseArtifactKindTool, "web_search", "0.1.9")},
				{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "zhihu_search"}, FailureReason: "官方索引没有该发布物的正式版本"},
			},
		},
	})
	list, err := system.ListCandidates(context.Background(), types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("ListCandidates() error = %v", err)
	}
	if list.SourceKind != string(installsource.KindOfficial) {
		t.Fatalf("source kind = %q", list.SourceKind)
	}
	context7 := findCandidate(t, list, types.ReleaseArtifactKindTool, "context7")
	if !context7.Installed || context7.CurrentVersion != "0.1.0" || context7.LatestVersion != "0.1.2" || !context7.UpdateAvailable {
		t.Fatalf("context7 = %#v", context7)
	}
	webSearch := findCandidate(t, list, types.ReleaseArtifactKindTool, "web_search")
	if webSearch.UpdateAvailable {
		t.Fatalf("web_search = %#v", webSearch)
	}
	zhihu := findCandidate(t, list, types.ReleaseArtifactKindTool, "zhihu_search")
	if zhihu.Status != types.ReleaseCandidateStatusFailed || zhihu.FailureReason == "" {
		t.Fatalf("zhihu = %#v", zhihu)
	}
}

func TestListCandidatesMarksInstalledArtifactMissingFromSource(t *testing.T) {
	system := newTestSystem(t, testOptions{
		tools: []types.ToolSummary{
			{ID: "legacy_tool", Version: "0.1.0", Status: types.ToolAvailabilityActive, EucliBoxCompatibility: compatibility("0.1.0", "0.2.0")},
		},
		candidates: map[string][]releasecheck.CandidateRecord{
			types.ReleaseArtifactKindTool: {
				{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "legacy_tool"}, FailureReason: "官方索引没有该发布物的正式版本"},
			},
		},
	})
	list, err := system.ListCandidates(context.Background(), types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("ListCandidates() error = %v", err)
	}
	legacy := findCandidate(t, list, types.ReleaseArtifactKindTool, "legacy_tool")
	if !legacy.Installed || legacy.CurrentVersion != "0.1.0" || legacy.Status != types.ReleaseCandidateStatusFailed {
		t.Fatalf("legacy = %#v", legacy)
	}
}

func TestListCandidatesReportsIndexReadFailure(t *testing.T) {
	system := newTestSystem(t, testOptions{
		candidateErr: map[string]error{types.ReleaseArtifactKindTool: fmt.Errorf("index 503")},
	})
	list, err := system.ListCandidates(context.Background(), types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("ListCandidates() error = %v", err)
	}
	if len(list.Candidates) != 1 || list.Candidates[0].Status != types.ReleaseCandidateStatusFailed || list.Candidates[0].FailureReason == "" {
		t.Fatalf("candidates = %#v", list.Candidates)
	}
}

func TestListCandidatesRejectsUnknownKind(t *testing.T) {
	system := newTestSystem(t, testOptions{})
	if _, err := system.ListCandidates(context.Background(), "unknown"); err == nil {
		t.Fatal("ListCandidates() unknown kind error = nil")
	}
}

func TestLocalCandidatesReadShelfWithoutOfficialIndex(t *testing.T) {
	shelfItem := releasecheck.LocalShelfItem{
		Artifact:  types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"},
		Candidate: localCandidate(types.ReleaseArtifactKindTool, "context7", "0.1.2"),
	}
	system := newTestSystem(t, testOptions{
		currentSource: installsource.KindLocal,
		shelf:         &fakeLocalShelf{items: []releasecheck.LocalShelfItem{shelfItem}},
		tools: []types.ToolSummary{
			{ID: "context7", Version: "0.1.0", Status: types.ToolAvailabilityActive, EucliBoxCompatibility: compatibility("0.1.0", "0.2.0")},
		},
	})
	list, err := system.ListCandidates(context.Background(), types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("ListCandidates() error = %v", err)
	}
	if list.SourceKind != string(installsource.KindLocal) {
		t.Fatalf("source kind = %q", list.SourceKind)
	}
	context7 := findCandidate(t, list, types.ReleaseArtifactKindTool, "context7")
	if !context7.Installed || context7.LatestVersion != "0.1.2" || !context7.UpdateAvailable {
		t.Fatalf("context7 = %#v", context7)
	}
	if _, err := system.ListCandidates(context.Background(), types.ReleaseArtifactKindBox); err == nil {
		t.Fatal("local box candidate error = nil")
	}
}

func TestLocalCandidatesRequireActiveShelf(t *testing.T) {
	system := newTestSystem(t, testOptions{currentSource: installsource.KindLocal})
	list, err := system.ListCandidates(context.Background(), types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("ListCandidates() error = %v", err)
	}
	if len(list.Candidates) != 1 || list.Candidates[0].FailureReason == "" {
		t.Fatalf("candidates = %#v", list.Candidates)
	}
}

func TestBoxUpdateAnnotatesIncompatibleInstalledArtifacts(t *testing.T) {
	system := newTestSystem(t, testOptions{
		tools: []types.ToolSummary{
			{ID: "old_tool", Version: "0.1.0", Status: types.ToolAvailabilityActive, EucliBoxCompatibility: compatibility("0.1.0", "0.2.0")},
			{ID: "future_tool", Version: "0.2.0", Status: types.ToolAvailabilityActive, EucliBoxCompatibility: compatibility("0.2.0", "0.3.0")},
		},
		candidates: map[string][]releasecheck.CandidateRecord{
			types.ReleaseArtifactKindBox: {
				{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox}, Candidate: candidate(types.ReleaseArtifactKindBox, types.ReleaseArtifactKindBox, "0.2.0")},
			},
		},
	})
	list, err := system.ListCandidates(context.Background(), types.ReleaseArtifactKindBox)
	if err != nil {
		t.Fatalf("ListCandidates() error = %v", err)
	}
	box := findCandidate(t, list, types.ReleaseArtifactKindBox, types.ReleaseArtifactKindBox)
	if !box.UpdateAvailable || len(box.AffectedArtifacts) != 1 || box.AffectedArtifacts[0].ID != "old_tool" {
		t.Fatalf("box = %#v", box)
	}
}

type testOptions struct {
	boxVersion    string
	currentSource installsource.Kind
	tools         []types.ToolSummary
	plugins       []types.SystemPluginSummary
	toolErr       error
	candidates    map[string][]releasecheck.CandidateRecord
	candidateErr  map[string]error
	shelf         releasecheck.LocalShelf
}

func newTestSystem(t *testing.T, options testOptions) System {
	t.Helper()
	boxVersion := options.boxVersion
	if boxVersion == "" {
		boxVersion = "0.1.0"
	}
	currentSource := options.currentSource
	if currentSource == "" {
		currentSource = installsource.KindOfficial
	}
	checker := &fakeCandidateLister{candidates: options.candidates, errors: options.candidateErr}
	system, err := NewSystemWithChecker(
		Config{BoxVersion: boxVersion, CurrentSource: func() installsource.Kind { return currentSource }, LocalSource: options.shelf},
		checker,
		&fakeToolSystem{tools: options.tools, err: options.toolErr},
		&fakePluginSystem{plugins: options.plugins},
	)
	if err != nil {
		t.Fatalf("NewSystemWithChecker() error = %v", err)
	}
	return system
}

type fakeCandidateLister struct {
	candidates map[string][]releasecheck.CandidateRecord
	errors     map[string]error
}

func (f *fakeCandidateLister) ListCandidates(_ context.Context, kind string) ([]releasecheck.CandidateRecord, error) {
	if err, ok := f.errors[kind]; ok {
		return nil, err
	}
	records := f.candidates[kind]
	if records == nil {
		records = []releasecheck.CandidateRecord{}
	}
	return records, nil
}

type fakeToolSystem struct {
	tools []types.ToolSummary
	err   error
}

func (f *fakeToolSystem) ListTools(context.Context) ([]types.ToolSummary, error) {
	return f.tools, f.err
}

type fakePluginSystem struct {
	plugins []types.SystemPluginSummary
}

func (f *fakePluginSystem) ListPlugins(context.Context) ([]types.SystemPluginSummary, error) {
	return f.plugins, nil
}

type fakeLocalShelf struct {
	items []releasecheck.LocalShelfItem
}

func (f *fakeLocalShelf) LatestCandidate(context.Context, types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	return nil, fmt.Errorf("not used")
}

func (f *fakeLocalShelf) List(context.Context) ([]releasecheck.LocalShelfItem, error) {
	return f.items, nil
}

func candidate(kind string, id string, version string) *releasecheck.ReleaseCandidate {
	return &releasecheck.ReleaseCandidate{
		Artifact:       types.ReleaseArtifactIdentity{Kind: kind, ID: id},
		Version:        version,
		PublishedAt:    time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC),
		SourceRevision: "0123456789abcdef0123456789abcdef01234567",
		Compatibility:  compatibilityPtr("0.1.0", "0.2.0"),
		ReleaseNotes:   "发行说明 " + version,
		ReleaseURL:     "https://example.com/releases/tag/" + id + "/v" + version,
		ArchiveURL:     "https://example.com/releases/download/" + id + "/v" + version + "/" + id + ".zip",
		SizeBytes:      123,
		SHA256:         release.SHA256([]byte("archive")),
	}
}

func localCandidate(kind string, id string, version string) *releasecheck.ReleaseCandidate {
	result := candidate(kind, id, version)
	result.Local = true
	result.ReleaseURL = ""
	return result
}

func compatibility(minimum string, maximum string) types.EucliBoxCompatibility {
	return types.EucliBoxCompatibility{MinimumVersion: minimum, MaximumVersionExclusive: maximum}
}

func compatibilityPtr(minimum string, maximum string) *types.EucliBoxCompatibility {
	result := compatibility(minimum, maximum)
	return &result
}

func findInstallation(t *testing.T, list types.ArtifactInstallationList, kind string, id string) types.ArtifactInstallation {
	t.Helper()
	for _, item := range list.Artifacts {
		if item.Artifact.Kind == kind && item.Artifact.ID == id {
			return item
		}
	}
	t.Fatalf("missing installation %s:%s in %#v", kind, id, list)
	return types.ArtifactInstallation{}
}

func findCandidate(t *testing.T, list types.ArtifactCandidateList, kind string, id string) types.ArtifactReleaseCandidate {
	t.Helper()
	for _, item := range list.Candidates {
		if item.Artifact.Kind == kind && item.Artifact.ID == id {
			return item
		}
	}
	t.Fatalf("missing candidate %s:%s in %#v", kind, id, list)
	return types.ArtifactReleaseCandidate{}
}
