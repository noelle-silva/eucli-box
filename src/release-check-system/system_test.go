package releasechecksystem

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"eucli-box/pkg/installsource"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

func TestRefreshUsesInstalledFactsAndKeepsSingleCheckInFlight(t *testing.T) {
	started := make(chan struct{})
	continueCheck := make(chan struct{})
	runner := &fakeRunner{run: func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot {
		close(started)
		<-continueCheck
		if len(installed) != 3 {
			t.Errorf("installed = %#v", installed)
		}
		return types.ReleaseCheckSnapshot{Status: types.ReleaseCheckStatusCompleted, Results: []types.ReleaseCheckResult{}}
	}}
	system := newTestSystem(t, runner, fakeTools{}, fakePlugins{})
	done := make(chan types.ReleaseCheckSnapshot, 1)
	go func() { done <- system.Refresh(context.Background(), "") }()
	<-started
	second := system.Refresh(context.Background(), "")
	if second.Status != types.ReleaseCheckStatusChecking {
		t.Fatalf("second snapshot = %#v", second)
	}
	close(continueCheck)
	if result := <-done; result.Status != types.ReleaseCheckStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if runner.calls() != 1 {
		t.Fatalf("runner calls = %d", runner.calls())
	}
}

func TestRefreshKeepsSourceResultsButMarksLocalInventoryFailure(t *testing.T) {
	runner := &fakeRunner{run: func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot {
		return types.ReleaseCheckSnapshot{Status: types.ReleaseCheckStatusCompleted, Results: []types.ReleaseCheckResult{
			{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: "eucli-box"}, Status: types.ReleaseCheckStatusCompleted},
			{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, Status: types.ReleaseCheckStatusCompleted, LatestVersion: "0.1.1"},
		}}
	}}
	system := newTestSystem(t, runner, fakeTools{err: errors.New("unavailable")}, fakePlugins{})
	snapshot := system.Refresh(context.Background(), "")
	if snapshot.Status != types.ReleaseCheckStatusFailed || snapshot.Results[0].Status != types.ReleaseCheckStatusCompleted || snapshot.Results[1].Status != types.ReleaseCheckStatusFailed || snapshot.Results[1].LatestVersion != "0.1.1" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestRefreshKeepsOtherKindsWhenRefreshingOneKind(t *testing.T) {
	runner := &fakeRunner{run: func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot {
		return types.ReleaseCheckSnapshot{Status: types.ReleaseCheckStatusCompleted, Results: []types.ReleaseCheckResult{
			{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, Status: types.ReleaseCheckStatusCompleted, LatestVersion: "0.1.2"},
			{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"}, Status: types.ReleaseCheckStatusCompleted, LatestVersion: "0.1.1"},
		}}
	}}
	system := newTestSystem(t, runner, fakeTools{}, fakePlugins{})
	first := system.Refresh(context.Background(), "")
	if first.Status != types.ReleaseCheckStatusCompleted || len(first.Results) != 2 {
		t.Fatalf("first = %#v", first)
	}
	runner.run = func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot {
		return types.ReleaseCheckSnapshot{Status: types.ReleaseCheckStatusCompleted, Results: []types.ReleaseCheckResult{
			{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"}, Status: types.ReleaseCheckStatusCompleted, LatestVersion: "0.2.0"},
		}}
	}
	second := system.Refresh(context.Background(), types.ReleaseArtifactKindPlugin)
	if second.Status != types.ReleaseCheckStatusCompleted || len(second.Results) != 2 {
		t.Fatalf("second = %#v", second)
	}
	toolResult := findKindResult(t, second, "tool", "context7")
	if toolResult.LatestVersion != "0.1.2" {
		t.Fatalf("tool result = %#v", toolResult)
	}
	pluginResult := findKindResult(t, second, "plugin", "time-plugin")
	if pluginResult.LatestVersion != "0.2.0" {
		t.Fatalf("plugin result = %#v", pluginResult)
	}
}

func TestRefreshRejectsUnknownKind(t *testing.T) {
	runner := &fakeRunner{run: func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot {
		return types.ReleaseCheckSnapshot{Status: types.ReleaseCheckStatusCompleted, Results: []types.ReleaseCheckResult{}}
	}}
	system := newTestSystem(t, runner, fakeTools{}, fakePlugins{})
	snapshot := system.Refresh(context.Background(), "unknown")
	if snapshot.Status != types.ReleaseCheckStatusFailed || snapshot.FailureReason == "" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

type fakeRunner struct {
	mu  sync.Mutex
	n   int
	run func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot
}

func (f *fakeRunner) CheckOnly(_ context.Context, installed []releasecheck.InstalledArtifact, _ string, _ []types.ReleaseArtifactIdentity) types.ReleaseCheckSnapshot {
	f.mu.Lock()
	f.n++
	f.mu.Unlock()
	return f.run(installed)
}

func (f *fakeRunner) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

type fakeTools struct{ err error }

func (f fakeTools) ListTools(context.Context) ([]types.ToolSummary, error) {
	return []types.ToolSummary{{ID: "context7", Version: "0.1.0", Status: types.ToolAvailabilityActive, EucliBoxCompatibility: types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}}}, f.err
}

type fakePlugins struct{ err error }

func (f fakePlugins) ListPlugins(context.Context) ([]types.SystemPluginSummary, error) {
	return []types.SystemPluginSummary{{ID: "time-plugin", Version: "0.1.0", Installed: true, EucliBoxCompatibility: types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}}}, f.err
}

func newTestSystem(t *testing.T, runner checkRunner, tools ToolSystem, plugins PluginSystem) System {
	t.Helper()
	system, err := NewSystemWithChecker(Config{Now: time.Now}, runner, tools, plugins, "0.1.0")
	if err != nil {
		t.Fatalf("NewSystemWithChecker error = %v", err)
	}
	return system
}

func TestRefreshOnlyAcceptsInstalledFacts(t *testing.T) {
	runner := &fakeRunner{run: func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot {
		if len(installed) != 3 {
			t.Errorf("installed = %#v", installed)
		}
		return types.ReleaseCheckSnapshot{Status: types.ReleaseCheckStatusCompleted, Results: []types.ReleaseCheckResult{}}
	}}
	system := newTestSystem(t, runner, fakeTools{}, fakePlugins{})
	_ = system.Refresh(context.Background(), "")
}

func TestRefreshKeepsUnavailablePluginWithVersionForImpact(t *testing.T) {
	runner := &fakeRunner{run: func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot {
		if len(installed) != 3 {
			t.Errorf("installed = %#v", installed)
		}
		return types.ReleaseCheckSnapshot{Status: types.ReleaseCheckStatusCompleted, Results: []types.ReleaseCheckResult{}}
	}}
	unavailablePlugins := func(context.Context) ([]types.SystemPluginSummary, error) {
		return []types.SystemPluginSummary{{ID: "time-plugin", Version: "0.1.0", Installed: true, Status: types.SystemPluginStatusUnavailable, EucliBoxCompatibility: types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}}}, nil
	}
	system := newTestSystem(t, runner, fakeTools{}, pluginSystemFunc(unavailablePlugins))
	_ = system.Refresh(context.Background(), "")
}

func TestRefreshDropsUninstalledAndInactiveTools(t *testing.T) {
	runner := &fakeRunner{run: func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot {
		if len(installed) != 2 {
			t.Errorf("installed = %#v", installed)
		}
		return types.ReleaseCheckSnapshot{Status: types.ReleaseCheckStatusCompleted, Results: []types.ReleaseCheckResult{}}
	}}
	tools := func(context.Context) ([]types.ToolSummary, error) {
		return []types.ToolSummary{
			{ID: "unavailable-tool", Version: "0.1.0", Status: types.ToolAvailabilityUnavailable, EucliBoxCompatibility: types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}},
			{ID: "bad-version", Version: "nope", Status: types.ToolAvailabilityActive, EucliBoxCompatibility: types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}},
		}, nil
	}
	system := newTestSystem(t, runner, toolSystemFunc(tools), fakePlugins{})
	_ = system.Refresh(context.Background(), "")
}

type pluginSystemFunc func(context.Context) ([]types.SystemPluginSummary, error)

func (f pluginSystemFunc) ListPlugins(ctx context.Context) ([]types.SystemPluginSummary, error) {
	return f(ctx)
}

type toolSystemFunc func(context.Context) ([]types.ToolSummary, error)

func (f toolSystemFunc) ListTools(ctx context.Context) ([]types.ToolSummary, error) {
	return f(ctx)
}

func findKindResult(t *testing.T, snapshot types.ReleaseCheckSnapshot, kind string, id string) types.ReleaseCheckResult {
	t.Helper()
	for _, result := range snapshot.Results {
		if result.Artifact.Kind == kind && result.Artifact.ID == id {
			return result
		}
	}
	t.Fatalf("missing result %s:%s in %#v", kind, id, snapshot)
	return types.ReleaseCheckResult{}
}

type fakeLocalShelf struct {
	items []releasecheck.LocalShelfItem
	err   error
}

func (f *fakeLocalShelf) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	for _, item := range f.items {
		if item.Artifact == identity {
			return item.Candidate, nil
		}
	}
	return nil, fmt.Errorf("货架没有 %s", identity.ID)
}

func (f *fakeLocalShelf) List(context.Context) ([]releasecheck.LocalShelfItem, error) {
	return f.items, f.err
}

func localToolCandidate(version string) *releasecheck.ReleaseCandidate {
	return &releasecheck.ReleaseCandidate{
		Artifact:         types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"},
		Version:          version,
		SourceRevision:   "0123456789abcdef0123456789abcdef01234567",
		SourceRepository: "https://github.com/noelle-silva/eucli-box",
		Compatibility:    &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		OfficialSource:   "https://github.com/noelle-silva/eucli-box-ai-tools",
		Local:            true,
	}
}

func localPluginCandidate(version string) *releasecheck.ReleaseCandidate {
	return &releasecheck.ReleaseCandidate{
		Artifact:         types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"},
		Version:          version,
		SourceRevision:   "0123456789abcdef0123456789abcdef01234567",
		SourceRepository: "https://github.com/noelle-silva/eucli-box",
		Compatibility:    &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		OfficialSource:   "https://github.com/noelle-silva/eucli-box-system-plugins",
		Local:            true,
	}
}

func TestLocalModeDoesNotRunOfficialCheck(t *testing.T) {
	runner := &fakeRunner{run: func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot {
		t.Fatal("official checker must not run in local mode")
		return types.ReleaseCheckSnapshot{}
	}}
	system, err := NewSystemWithChecker(Config{Now: time.Now, CurrentSource: func() installsource.Kind { return installsource.KindLocal }, LocalSource: &fakeLocalShelf{items: []releasecheck.LocalShelfItem{
		{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, Candidate: localToolCandidate("0.1.0")},
		{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"}, Candidate: localPluginCandidate("0.1.0")},
	}}}, runner, fakeTools{}, fakePlugins{}, "0.1.0")
	if err != nil {
		t.Fatalf("NewSystemWithChecker error = %v", err)
	}
	snapshot := system.Refresh(context.Background(), "")
	if snapshot.Status != types.ReleaseCheckStatusCompleted {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.SourceKind != string(installsource.KindLocal) {
		t.Fatalf("SourceKind = %q, want %q", snapshot.SourceKind, installsource.KindLocal)
	}
	// 本地快照只描述本地商店货架事实；本体候选不上架，不出现 box 结果。
	if len(snapshot.Results) != 2 {
		t.Fatalf("results = %#v, want context7/time-plugin only", snapshot.Results)
	}
	tool := findKindResult(t, snapshot, "tool", "context7")
	if tool.CurrentVersion != "0.1.0" || !tool.Installed || tool.UpdateAvailable || tool.LatestVersion != "0.1.0" {
		t.Fatalf("tool result = %#v", tool)
	}
	plugin := findKindResult(t, snapshot, "plugin", "time-plugin")
	if plugin.CurrentVersion != "0.1.0" || !plugin.Installed || plugin.UpdateAvailable || plugin.LatestVersion != "0.1.0" {
		t.Fatalf("plugin result = %#v", plugin)
	}
	if runner.calls() != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls())
	}
}

func TestLocalModeListsShelfCandidates(t *testing.T) {
	runner := &fakeRunner{run: func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot {
		return types.ReleaseCheckSnapshot{Status: types.ReleaseCheckStatusCompleted, Results: []types.ReleaseCheckResult{
			{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, Status: types.ReleaseCheckStatusCompleted, LatestVersion: "9.9.9"},
		}}
	}}
	current := installsource.KindOfficial
	checked, err := NewSystemWithChecker(Config{Now: time.Now, CurrentSource: func() installsource.Kind { return current }, LocalSource: &fakeLocalShelf{items: []releasecheck.LocalShelfItem{
		{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, Candidate: localToolCandidate("0.1.0")},
	}}}, runner, fakeTools{}, fakePlugins{}, "0.1.0")
	if err != nil {
		t.Fatalf("NewSystemWithChecker error = %v", err)
	}
	current = installsource.KindLocal
	snapshot := checked.Refresh(context.Background(), types.ReleaseArtifactKindTool)
	if snapshot.SourceKind != string(installsource.KindLocal) {
		t.Fatalf("SourceKind = %q, want local", snapshot.SourceKind)
	}
	if len(snapshot.Results) != 1 || snapshot.Results[0].LatestVersion != "0.1.0" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestLocalModeShelfFailureMarksFailed(t *testing.T) {
	runner := &fakeRunner{run: func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot {
		return types.ReleaseCheckSnapshot{}
	}}
	system, err := NewSystemWithChecker(Config{Now: time.Now, CurrentSource: func() installsource.Kind { return installsource.KindLocal }, LocalSource: &fakeLocalShelf{err: errors.New("shelf unreadable")}}, runner, fakeTools{}, fakePlugins{}, "0.1.0")
	if err != nil {
		t.Fatalf("NewSystemWithChecker error = %v", err)
	}
	snapshot := system.Refresh(context.Background(), "")
	if snapshot.Status != types.ReleaseCheckStatusFailed || len(snapshot.Results) != 0 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestOfficialModeSnapshotIsMarkedOfficial(t *testing.T) {
	runner := &fakeRunner{run: func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot {
		return types.ReleaseCheckSnapshot{Status: types.ReleaseCheckStatusCompleted, Results: []types.ReleaseCheckResult{}}
	}}
	system := newTestSystem(t, runner, fakeTools{}, fakePlugins{})
	snapshot := system.Refresh(context.Background(), "")
	if snapshot.SourceKind != string(installsource.KindOfficial) {
		t.Fatalf("SourceKind = %q, want %q", snapshot.SourceKind, installsource.KindOfficial)
	}
}

func TestSourceSnapshotsDoNotOverwriteEachOther(t *testing.T) {
	runner := &fakeRunner{run: func(installed []releasecheck.InstalledArtifact) types.ReleaseCheckSnapshot {
		return types.ReleaseCheckSnapshot{Status: types.ReleaseCheckStatusCompleted, Results: []types.ReleaseCheckResult{
			{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, Status: types.ReleaseCheckStatusCompleted, LatestVersion: "0.2.0"},
		}}
	}}
	current := installsource.KindOfficial
	system, err := NewSystemWithChecker(Config{Now: time.Now, CurrentSource: func() installsource.Kind { return current }, LocalSource: &fakeLocalShelf{items: []releasecheck.LocalShelfItem{
		{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, Candidate: localToolCandidate("0.1.0")},
	}}}, runner, fakeTools{}, fakePlugins{}, "0.1.0")
	if err != nil {
		t.Fatalf("NewSystemWithChecker error = %v", err)
	}
	official := system.Refresh(context.Background(), types.ReleaseArtifactKindTool)
	if official.SourceKind != string(installsource.KindOfficial) || len(official.Results) != 1 || official.Results[0].LatestVersion != "0.2.0" {
		t.Fatalf("official snapshot = %#v", official)
	}

	current = installsource.KindLocal
	local := system.Refresh(context.Background(), types.ReleaseArtifactKindTool)
	if local.SourceKind != string(installsource.KindLocal) || len(local.Results) != 1 || local.Results[0].LatestVersion != "0.1.0" {
		t.Fatalf("local snapshot = %#v", local)
	}

	current = installsource.KindOfficial
	restored := system.Snapshot()
	if restored.SourceKind != string(installsource.KindOfficial) || len(restored.Results) != 1 || restored.Results[0].LatestVersion != "0.2.0" {
		t.Fatalf("restored official snapshot = %#v", restored)
	}
	if runner.calls() != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls())
	}
}
