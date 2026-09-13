package releasecheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

func TestListCandidatesReadsOneIndexPerKind(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.1")
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.2")
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "web_search"}, "0.1.9")
	checker := fixture.checker(t)
	records, err := checker.ListCandidates(context.Background(), types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("ListCandidates() error = %v", err)
	}
	context7 := findCandidateRecord(t, records, types.ReleaseArtifactKindTool, "context7")
	if context7.Candidate == nil || context7.Candidate.Version != "0.1.2" {
		t.Fatalf("context7 record = %#v", context7)
	}
	if context7.Candidate.PublishedAt.IsZero() || context7.Candidate.SizeBytes <= 0 {
		t.Fatalf("context7 candidate facts missing: %#v", context7.Candidate)
	}
	webSearch := findCandidateRecord(t, records, types.ReleaseArtifactKindTool, "web_search")
	if webSearch.Candidate == nil || webSearch.Candidate.Version != "0.1.9" {
		t.Fatalf("web_search record = %#v", webSearch)
	}
	if fixture.kindRequests[types.ReleaseArtifactKindTool] != 1 {
		t.Fatalf("tool index requests = %d, want 1", fixture.kindRequests[types.ReleaseArtifactKindTool])
	}
	if fixture.repositoryRequests[fixture.repositoryFor(t, types.ReleaseArtifactKindTool)] != 1 {
		t.Fatalf("repository requests = %#v", fixture.repositoryRequests)
	}
	if len(fixture.repositoryRequests) != 1 {
		t.Fatalf("ListCandidates touched other repositories: %#v", fixture.repositoryRequests)
	}
}

func TestListCandidatesMarksMissingArtifactAndInvalidIndex(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.1")
	checker := fixture.checker(t)
	records, err := checker.ListCandidates(context.Background(), types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("ListCandidates() error = %v", err)
	}
	missing := findCandidateRecord(t, records, types.ReleaseArtifactKindTool, "zhihu_search")
	if missing.Candidate != nil || missing.FailureReason == "" {
		t.Fatalf("missing record = %#v", missing)
	}
	fixture.brokenKinds[types.ReleaseArtifactKindTool] = true
	if _, err := checker.ListCandidates(context.Background(), types.ReleaseArtifactKindTool); err == nil {
		t.Fatal("ListCandidates() with broken index error = nil")
	}
}

func TestListCandidatesForUnknownKindIsEmpty(t *testing.T) {
	fixture := newGitHubFixture(t)
	checker := fixture.checker(t)
	records, err := checker.ListCandidates(context.Background(), "unknown")
	if err != nil {
		t.Fatalf("ListCandidates() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records = %#v", records)
	}
	if len(fixture.kindRequests) != 0 {
		t.Fatalf("unexpected requests = %#v", fixture.kindRequests)
	}
}

type githubFixture struct {
	t                  *testing.T
	server             *httptest.Server
	indexes            map[string]releasecatalog.Index
	brokenKinds        map[string]bool
	kindRequests       map[string]int
	repositoryRequests map[string]int
	now                time.Time
}

func newGitHubFixture(t *testing.T) *githubFixture {
	t.Helper()
	fixture := &githubFixture{
		t:                  t,
		indexes:            map[string]releasecatalog.Index{},
		brokenKinds:        map[string]bool{},
		kindRequests:       map[string]int{},
		repositoryRequests: map[string]int{},
		now:                time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *githubFixture) checker(t *testing.T) *Checker {
	t.Helper()
	checker, err := New(Config{Client: f.server.Client(), IndexBase: f.server.URL})
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
	f.kindRequests[kind]++
	f.repositoryRequests[repositoryForIndexPath(r.URL.Path)]++
	if f.brokenKinds[kind] {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(f.indexes[kind])
}

func repositoryForIndexPath(path string) string {
	switch {
	case strings.Contains(path, "/noelle-silva/eucli-box-ai-tools/"):
		return "noelle-silva/eucli-box-ai-tools"
	case strings.Contains(path, "/noelle-silva/eucli-box-system-plugins/"):
		return "noelle-silva/eucli-box-system-plugins"
	case strings.Contains(path, "/noelle-silva/eucli-box/"):
		return "noelle-silva/eucli-box"
	default:
		return ""
	}
}

func (f *githubFixture) repositoryFor(t *testing.T, kind string) string {
	t.Helper()
	catalog, err := releasecatalog.Load()
	if err != nil {
		t.Fatalf("Load catalog error = %v", err)
	}
	source, err := catalog.SourceFor(kind)
	if err != nil {
		t.Fatalf("SourceFor() error = %v", err)
	}
	parsed, err := url.Parse(source.Repository)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return strings.Trim(parsed.Path, "/")
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

func findCandidateRecord(t *testing.T, records []CandidateRecord, kind string, id string) CandidateRecord {
	t.Helper()
	for _, record := range records {
		if record.Artifact.Kind == kind && record.Artifact.ID == id {
			return record
		}
	}
	t.Fatalf("missing candidate record %s:%s in %#v", kind, id, records)
	return CandidateRecord{}
}
