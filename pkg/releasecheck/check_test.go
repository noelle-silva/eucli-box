package releasecheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

func TestCheckFindsIndependentLatestVersionsWithoutDownloadingArchives(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: "eucli-box"}, "0.1.1")
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.2")
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "web_search"}, "0.1.4")
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "web_search"}, "0.1.9")
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"}, "0.1.0")
	checker := fixture.checker(t)
	compatibility := types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}
	snapshot := checker.Check(context.Background(), []InstalledArtifact{
		{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: "eucli-box"}, Version: "0.1.0"},
		{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, Version: "0.1.0", Compatibility: &compatibility},
		{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "web_search"}, Version: "0.1.4", Compatibility: &compatibility},
		{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"}, Version: "0.1.0", Compatibility: &compatibility},
	}, "0.1.0")
	assertCheckResult(t, snapshot, "eucli-box", "eucli-box", "0.1.1", true, types.ReleaseCheckStatusCompleted)
	assertCheckResult(t, snapshot, "tool", "context7", "0.1.2", true, types.ReleaseCheckStatusCompleted)
	assertCheckResult(t, snapshot, "tool", "web_search", "0.1.9", true, types.ReleaseCheckStatusCompleted)
	assertCheckResult(t, snapshot, "plugin", "time-plugin", "0.1.0", false, types.ReleaseCheckStatusCompleted)
}

func TestCheckKeepsOtherArtifactsWhenOneIndexIsInvalid(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.1")
	fixture.brokenKinds[types.ReleaseArtifactKindTool] = true
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"}, "0.1.0")
	checker := fixture.checker(t)
	snapshot := checker.Check(context.Background(), nil, "0.1.0")
	assertCheckResult(t, snapshot, "plugin", "time-plugin", "0.1.0", true, types.ReleaseCheckStatusCompleted)
	failed := findCheckResult(t, snapshot, "tool", "context7")
	if failed.Status != types.ReleaseCheckStatusFailed || failed.FailureReason == "" {
		t.Fatalf("broken result = %#v", failed)
	}
}

func TestCheckDistinguishesSourceFailureFromMissingArtifact(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.1")
	fixture.brokenKinds[types.ReleaseArtifactKindPlugin] = true
	checker := fixture.checker(t)
	snapshot := checker.Check(context.Background(), nil, "0.1.0")
	missing := findCheckResult(t, snapshot, "tool", "zhihu_search")
	if missing.Status != types.ReleaseCheckStatusFailed || missing.FailureReason == "" {
		t.Fatalf("missing result = %#v", missing)
	}
	failed := findCheckResult(t, snapshot, "plugin", "time-plugin")
	if failed.Status != types.ReleaseCheckStatusFailed || !strings.Contains(failed.FailureReason, "503") {
		t.Fatalf("failed result = %#v", failed)
	}
}

func TestCheckOnlyReadsRequestedSource(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: "eucli-box"}, "0.1.1")
	checker := fixture.checker(t)
	snapshot := checker.CheckOnly(context.Background(), nil, "", []types.ReleaseArtifactIdentity{{Kind: types.ReleaseArtifactKindBox, ID: "eucli-box"}})
	if len(snapshot.Results) != 1 || snapshot.Results[0].Artifact.Kind != types.ReleaseArtifactKindBox {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if fixture.sourceRequests[types.ReleaseArtifactKindBox] != 1 || fixture.sourceRequests[types.ReleaseArtifactKindTool] != 0 || fixture.sourceRequests[types.ReleaseArtifactKindPlugin] != 0 {
		t.Fatalf("source requests = %#v", fixture.sourceRequests)
	}
}

func TestCheckExposesIndexDates(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.1")
	checker := fixture.checker(t)
	snapshot := checker.Check(context.Background(), nil, "0.1.0")
	result := findCheckResult(t, snapshot, "tool", "context7")
	if result.PublishedAt.IsZero() || result.IndexUpdatedAt.IsZero() {
		t.Fatalf("result dates missing: %#v", result)
	}
}

type githubFixture struct {
	t              *testing.T
	server         *httptest.Server
	indexes        map[string]releasecatalog.Index
	brokenKinds    map[string]bool
	sourceRequests map[string]int
	now            time.Time
}

func newGitHubFixture(t *testing.T) *githubFixture {
	t.Helper()
	fixture := &githubFixture{
		t:              t,
		indexes:        map[string]releasecatalog.Index{},
		brokenKinds:    map[string]bool{},
		sourceRequests: map[string]int{},
		now:            time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *githubFixture) checker(t *testing.T) *Checker {
	t.Helper()
	checker, err := New(Config{Client: f.server.Client(), APIBaseURL: f.server.URL, IndexBase: f.server.URL, Now: func() time.Time { return f.now }})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return checker
}

func (f *githubFixture) addIndexVersion(identity types.ReleaseArtifactIdentity, version string) {
	f.t.Helper()
	index := f.indexes[identity.Kind]
	if index.SchemaVersion == 0 {
		index = releasecatalog.Index{SchemaVersion: releasecatalog.IndexSchemaVersion, UpdatedAt: f.now}
	}
	artifactIndex := -1
	for at := range index.Artifacts {
		if index.Artifacts[at].Kind == identity.Kind && index.Artifacts[at].ID == identity.ID {
			artifactIndex = at
			break
		}
	}
	if artifactIndex < 0 {
		index.Artifacts = append(index.Artifacts, releasecatalog.IndexArtifact{Kind: identity.Kind, ID: identity.ID, Versions: []releasecatalog.IndexVersion{}})
		artifactIndex = len(index.Artifacts) - 1
	}
	catalog, err := releasecatalog.Load()
	if err != nil {
		f.t.Fatalf("Load catalog error = %v", err)
	}
	tag, err := releasecatalog.TagName(identity, version)
	if err != nil {
		f.t.Fatalf("TagName error = %v", err)
	}
	fileName, err := releasecatalog.ArchiveName(identity, version)
	if err != nil {
		f.t.Fatalf("ArchiveName error = %v", err)
	}
	compatibility := &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}
	dataVersion := ""
	if identity.Kind == types.ReleaseArtifactKindBox {
		compatibility = nil
		dataVersion = "1.0.0"
	}
	_ = catalog
	index.Artifacts[artifactIndex].Versions = append(index.Artifacts[artifactIndex].Versions, releasecatalog.IndexVersion{
		Version:        version,
		PublishedAt:    f.now,
		SourceRevision: "0123456789abcdef0123456789abcdef01234567",
		DataVersion:    dataVersion,
		Compatibility:  compatibility,
		ReleaseNotes:   "发行说明 " + version,
		Packages: []releasecatalog.IndexPackage{{
			Platform:   types.ReleasePlatformWindowsX64,
			ReleaseTag: tag,
			FileName:   fileName,
			SizeBytes:  123,
			SHA256:     release.SHA256([]byte("archive")),
		}},
	})
	if err := releasecatalog.ValidateIndex(index); err != nil {
		f.t.Fatalf("ValidateIndex error = %v", err)
	}
	f.indexes[identity.Kind] = index
}

func (f *githubFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	kind := kindForIndexPath(r.URL.Path)
	if kind == "" {
		http.NotFound(w, r)
		return
	}
	f.sourceRequests[kind]++
	if f.brokenKinds[kind] {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(f.indexes[kind])
}

func kindForIndexPath(path string) string {
	switch {
	case strings.Contains(path, "/noelle-silva/eucli-box-ai-tools/"):
		return types.ReleaseArtifactKindTool
	case strings.Contains(path, "/noelle-silva/eucli-box-system-plugins/"):
		return types.ReleaseArtifactKindPlugin
	case strings.Contains(path, "/noelle-silva/eucli-box/"):
		return types.ReleaseArtifactKindBox
	default:
		return ""
	}
}

func assertCheckResult(t *testing.T, snapshot types.ReleaseCheckSnapshot, kind string, id string, latest string, available bool, status string) {
	t.Helper()
	result := findCheckResult(t, snapshot, kind, id)
	if result.LatestVersion != latest || result.UpdateAvailable != available || result.Status != status {
		t.Fatalf("result = %#v", result)
	}
}

func findCheckResult(t *testing.T, snapshot types.ReleaseCheckSnapshot, kind string, id string) types.ReleaseCheckResult {
	t.Helper()
	for _, result := range snapshot.Results {
		if result.Artifact.Kind == kind && result.Artifact.ID == id {
			return result
		}
	}
	t.Fatalf("missing result %s:%s in %#v", kind, id, snapshot)
	return types.ReleaseCheckResult{}
}
