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
	fixture.addRelease(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: "eucli-box"}, "0.1.1", false, false)
	fixture.addRelease(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.2", false, false)
	fixture.addRelease(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "web_search"}, "0.1.4", false, false)
	fixture.addRelease(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "web_search"}, "0.1.9", false, true)
	fixture.addRelease(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"}, "0.1.0", false, false)
	checker := fixture.checker(t)
	compatibility := types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}
	snapshot := checker.Check(context.Background(), []InstalledArtifact{
		{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: "eucli-box"}, Version: "0.1.0"},
		{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, Version: "0.1.0", Compatibility: &compatibility},
		{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "web_search"}, Version: "0.1.4", Compatibility: &compatibility},
		{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"}, Version: "0.1.0", Compatibility: &compatibility},
	}, "0.1.0")
	if snapshot.Status != types.ReleaseCheckStatusCompleted || len(snapshot.Results) != 11 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	assertCheckResult(t, snapshot, "tool", "context7", "0.1.2", true, types.ReleaseCheckStatusCompleted)
	assertCheckResult(t, snapshot, "tool", "web_search", "0.1.4", false, types.ReleaseCheckStatusCompleted)
	assertCheckResult(t, snapshot, "plugin", "time-plugin", "0.1.0", false, types.ReleaseCheckStatusCompleted)
	if fixture.archiveRequests != 0 {
		t.Fatalf("archive requests = %d, want 0", fixture.archiveRequests)
	}
}

func TestCheckKeepsOtherArtifactsWhenOneManifestIsInvalid(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.addRelease(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.1", false, false)
	fixture.addBrokenRelease(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "web_search"}, "0.1.1")
	checker := fixture.checker(t)
	snapshot := checker.Check(context.Background(), nil, "0.1.0")
	assertCheckResult(t, snapshot, "tool", "context7", "0.1.1", true, types.ReleaseCheckStatusCompleted)
	broken := findCheckResult(t, snapshot, "tool", "web_search")
	if broken.Status != types.ReleaseCheckStatusFailed || broken.FailureReason == "" {
		t.Fatalf("broken result = %#v", broken)
	}
}

func TestCheckDistinguishesSourceFailureFromNoRelease(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.failedKinds[types.ReleaseArtifactKindPlugin] = true
	checker := fixture.checker(t)
	snapshot := checker.Check(context.Background(), nil, "0.1.0")
	noRelease := findCheckResult(t, snapshot, "tool", "context7")
	if noRelease.Status != types.ReleaseCheckStatusCompleted || noRelease.LatestVersion != "" || noRelease.FailureReason != "" {
		t.Fatalf("no release result = %#v", noRelease)
	}
	failed := findCheckResult(t, snapshot, "plugin", "time-plugin")
	if failed.Status != types.ReleaseCheckStatusFailed || !strings.Contains(failed.FailureReason, "503") {
		t.Fatalf("failed result = %#v", failed)
	}
}

func TestCheckOnlyReadsRequestedSource(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.addRelease(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: "eucli-box"}, "0.1.1", false, false)
	checker := fixture.checker(t)
	snapshot := checker.CheckOnly(context.Background(), nil, "", []types.ReleaseArtifactIdentity{{Kind: types.ReleaseArtifactKindBox, ID: "eucli-box"}})
	if len(snapshot.Results) != 1 || snapshot.Results[0].Artifact.Kind != types.ReleaseArtifactKindBox {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if fixture.sourceRequests[types.ReleaseArtifactKindBox] != 1 || fixture.sourceRequests[types.ReleaseArtifactKindTool] != 0 || fixture.sourceRequests[types.ReleaseArtifactKindPlugin] != 0 {
		t.Fatalf("source requests = %#v", fixture.sourceRequests)
	}
}

type githubFixture struct {
	t               *testing.T
	server          *httptest.Server
	releases        map[string][]githubRelease
	manifests       map[string][]byte
	failedKinds     map[string]bool
	sourceRequests  map[string]int
	archiveRequests int
	now             time.Time
}

func newGitHubFixture(t *testing.T) *githubFixture {
	t.Helper()
	fixture := &githubFixture{
		t:              t,
		releases:       map[string][]githubRelease{},
		manifests:      map[string][]byte{},
		failedKinds:    map[string]bool{},
		sourceRequests: map[string]int{},
		now:            time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *githubFixture) checker(t *testing.T) *Checker {
	t.Helper()
	checker, err := New(Config{Client: f.server.Client(), APIBaseURL: f.server.URL, Now: func() time.Time { return f.now }})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return checker
}

func (f *githubFixture) addRelease(identity types.ReleaseArtifactIdentity, version string, draft bool, prerelease bool) {
	f.addReleaseWithManifest(identity, version, draft, prerelease, false)
}

func (f *githubFixture) addBrokenRelease(identity types.ReleaseArtifactIdentity, version string) {
	f.addReleaseWithManifest(identity, version, false, false, true)
}

func (f *githubFixture) addReleaseWithManifest(identity types.ReleaseArtifactIdentity, version string, draft bool, prerelease bool, broken bool) {
	f.t.Helper()
	catalog, err := releasecatalog.Load()
	if err != nil {
		f.t.Fatalf("Load catalog error = %v", err)
	}
	source, err := catalog.SourceFor(identity.Kind)
	if err != nil {
		f.t.Fatalf("SourceFor error = %v", err)
	}
	tag, err := releasecatalog.TagName(identity, version)
	if err != nil {
		f.t.Fatalf("TagName error = %v", err)
	}
	archiveName, err := releasecatalog.ArchiveName(identity, version)
	if err != nil {
		f.t.Fatalf("ArchiveName error = %v", err)
	}
	compatibility := &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}
	dataVersion := ""
	if identity.Kind == types.ReleaseArtifactKindBox {
		compatibility = nil
		dataVersion = "1.0.0"
	}
	manifest := types.ReleaseManifest{
		SchemaVersion:  release.ReleaseManifestSchemaVersion,
		Artifact:       identity,
		Version:        version,
		Platform:       types.ReleasePlatformWindowsX64,
		TagName:        tag,
		OfficialSource: source.Repository,
		Compatibility:  compatibility,
		DataVersion:    dataVersion,
		Source:         types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
		Archive:        types.ReleaseFileRecord{Name: archiveName, Size: 123, SHA256: release.SHA256([]byte("archive"))},
		Files:          []types.ReleaseFileRecord{{Name: "release-product.json", Size: 2, SHA256: release.SHA256([]byte("{}"))}},
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		f.t.Fatalf("Marshal manifest error = %v", err)
	}
	if broken {
		payload = []byte(`{"broken":true}`)
	}
	key := identity.Kind + "/" + identity.ID + "/" + version
	f.manifests[key] = payload
	manifestName := strings.TrimSuffix(archiveName, ".zip") + ".manifest.json"
	f.releases[identity.Kind] = append(f.releases[identity.Kind], githubRelease{
		TagName:    tag,
		Draft:      draft,
		Prerelease: prerelease,
		HTMLURL:    source.Repository + "/releases/tag/" + urlPath(tag),
		Body:       "发行说明 " + version,
		Assets: []githubReleaseAsset{
			{Name: manifestName, Size: int64(len(payload)), BrowserDownloadURL: f.server.URL + "/manifest/" + key},
			{Name: archiveName, Size: 123, BrowserDownloadURL: source.Repository + "/releases/download/" + urlPath(tag) + "/" + archiveName},
		},
	})
}

func (f *githubFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/manifest/") {
		key := strings.TrimPrefix(r.URL.Path, "/manifest/")
		payload, ok := f.manifests[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
		return
	}
	if strings.Contains(r.URL.Path, "/releases/download/") {
		f.archiveRequests++
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	kind := kindForRepositoryPath(r.URL.Path)
	if kind == "" {
		http.NotFound(w, r)
		return
	}
	f.sourceRequests[kind]++
	if f.failedKinds[kind] {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	_ = json.NewEncoder(w).Encode(f.releases[kind])
}

func kindForRepositoryPath(path string) string {
	switch {
	case strings.Contains(path, "/repos/noelle-silva/eucli-box-ai-tools/releases"):
		return types.ReleaseArtifactKindTool
	case strings.Contains(path, "/repos/noelle-silva/eucli-box-system-plugins/releases"):
		return types.ReleaseArtifactKindPlugin
	case strings.Contains(path, "/repos/noelle-silva/eucli-box/releases"):
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

func urlPath(value string) string {
	return strings.ReplaceAll(value, "/", "%2F")
}
