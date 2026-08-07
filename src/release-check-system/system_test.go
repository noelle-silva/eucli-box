package releasechecksystem

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
