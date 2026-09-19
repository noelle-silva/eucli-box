package releasepublish

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

func TestDelistRemovesWholeArtifactFromIndexReleasesAndTags(t *testing.T) {
	fixture := newDelistFixture(t)
	context7 := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}
	fixture.seed(t, context7, "0.1.0")
	fixture.seed(t, context7, "0.2.0")
	fixture.seed(t, types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "everything"}, "0.1.0")
	publisher := fixture.publisher(t)

	result, err := publisher.Delist(context.Background(), delistSource(t, types.ReleaseArtifactKindTool), DelistInput{Artifact: context7})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IndexRemoved || len(result.RemovedVersions) != 2 || len(result.RemovedReleases) != 2 || len(result.RemovedTags) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if fixture.putTotal() != 1 {
		t.Fatalf("put count = %d", fixture.putTotal())
	}
	index := fixture.currentIndex()
	if indexHasDelistedVersion(index, context7, "") {
		t.Fatal("context7 仍存在于索引")
	}
	if !indexHasDelistedVersion(index, types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "everything"}, "0.1.0") {
		t.Fatal("everything 不应受到影响")
	}
	releases := fixture.currentReleases()
	refs := fixture.currentRefs()
	if len(releases) != 1 || releases[0].TagName != "everything/v0.1.0" {
		t.Fatalf("releases = %#v", releases)
	}
	if len(refs) != 1 || refs[0] != "refs/tags/everything/v0.1.0" {
		t.Fatalf("refs = %#v", refs)
	}
}

func TestDelistSingleVersionKeepsRemainingVersions(t *testing.T) {
	fixture := newDelistFixture(t)
	context7 := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}
	fixture.seed(t, context7, "0.1.0")
	fixture.seed(t, context7, "0.2.0")
	publisher := fixture.publisher(t)

	result, err := publisher.Delist(context.Background(), delistSource(t, types.ReleaseArtifactKindTool), DelistInput{Artifact: context7, Version: "0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RemovedVersions) != 1 || result.RemovedVersions[0] != "0.2.0" || len(result.RemovedReleases) != 1 || len(result.RemovedTags) != 1 {
		t.Fatalf("result = %#v", result)
	}
	index := fixture.currentIndex()
	if indexHasDelistedVersion(index, context7, "0.2.0") {
		t.Fatal("0.2.0 仍存在于索引")
	}
	if !indexHasDelistedVersion(index, context7, "0.1.0") {
		t.Fatal("0.1.0 不应受到影响")
	}
	releases := fixture.currentReleases()
	if len(releases) != 1 || releases[0].TagName != "context7/v0.1.0" {
		t.Fatalf("releases = %#v", releases)
	}
	refs := fixture.currentRefs()
	if len(refs) != 1 || refs[0] != "refs/tags/context7/v0.1.0" {
		t.Fatalf("refs = %#v", refs)
	}
}

func TestDelistRemovesNonWhitelistedArtifact(t *testing.T) {
	fixture := newDelistFixture(t)
	fixture.seed(t, types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "everything"}, "0.1.0")
	fileOperator := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "file_operator"}
	fixture.seed(t, fileOperator, "0.1.0")
	publisher := fixture.publisher(t)

	result, err := publisher.Delist(context.Background(), delistSource(t, types.ReleaseArtifactKindTool), DelistInput{Artifact: fileOperator})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IndexRemoved || len(result.RemovedVersions) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if indexHasDelistedVersion(fixture.currentIndex(), fileOperator, "") {
		t.Fatal("file_operator 仍存在于索引")
	}
	if releaseCount, refCount := countRemoteByPrefix(fixture.currentReleases(), fixture.currentRefs(), "file_operator/"); releaseCount != 0 || refCount != 0 {
		t.Fatalf("file_operator 远端仍有残留: releases=%d refs=%d", releaseCount, refCount)
	}
}

func TestDelistHealsRemoteResidueWithoutIndexEntry(t *testing.T) {
	fixture := newDelistFixture(t)
	fixture.seed(t, types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "everything"}, "0.1.0")
	context7 := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}
	fixture.seedRemote(t, context7, "0.1.0")
	publisher := fixture.publisher(t)

	result, err := publisher.Delist(context.Background(), delistSource(t, types.ReleaseArtifactKindTool), DelistInput{Artifact: context7})
	if err != nil {
		t.Fatal(err)
	}
	if result.IndexRemoved || len(result.RemovedVersions) != 0 || len(result.RemovedReleases) != 1 || len(result.RemovedTags) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if fixture.putTotal() != 0 {
		t.Fatalf("不应提交索引: put count = %d", fixture.putTotal())
	}
	if releaseCount, refCount := countRemoteByPrefix(fixture.currentReleases(), fixture.currentRefs(), "context7/"); releaseCount != 0 || refCount != 0 {
		t.Fatalf("context7 远端仍有残留: releases=%d refs=%d", releaseCount, refCount)
	}
	if releaseCount, refCount := countRemoteByPrefix(fixture.currentReleases(), fixture.currentRefs(), "everything/"); releaseCount != 1 || refCount != 1 {
		t.Fatalf("everything 不应受到影响: releases=%d refs=%d", releaseCount, refCount)
	}
}

func TestDelistRejectsUnknownTargetWithoutWrites(t *testing.T) {
	fixture := newDelistFixture(t)
	fixture.seed(t, types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "everything"}, "0.1.0")
	publisher := fixture.publisher(t)

	_, err := publisher.Delist(context.Background(), delistSource(t, types.ReleaseArtifactKindTool), DelistInput{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "missing"}})
	if err == nil || !strings.Contains(err.Error(), "无需下架") {
		t.Fatalf("Delist() error = %v", err)
	}
	if fixture.putTotal() != 0 || fixture.deleteReleaseTotal() != 0 || fixture.deleteRefTotal() != 0 {
		t.Fatalf("不应有任何远端写操作")
	}
}

func TestDelistRejectsLastArtifact(t *testing.T) {
	fixture := newDelistFixture(t)
	context7 := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}
	fixture.seed(t, context7, "0.1.0")
	publisher := fixture.publisher(t)

	_, err := publisher.Delist(context.Background(), delistSource(t, types.ReleaseArtifactKindTool), DelistInput{Artifact: context7})
	if err == nil || !strings.Contains(err.Error(), "最后一个发布物") {
		t.Fatalf("Delist() error = %v", err)
	}
	if fixture.putTotal() != 0 {
		t.Fatalf("不应提交索引: put count = %d", fixture.putTotal())
	}
	if len(fixture.currentReleases()) != 1 || len(fixture.currentRefs()) != 1 {
		t.Fatalf("远端不应被改动")
	}
}

func TestDelistRejectsNonFormalVersion(t *testing.T) {
	fixture := newDelistFixture(t)
	context7 := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}
	fixture.seed(t, context7, "0.1.0")
	publisher := fixture.publisher(t)

	_, err := publisher.Delist(context.Background(), delistSource(t, types.ReleaseArtifactKindTool), DelistInput{Artifact: context7, Version: "0.1.0.1"})
	if err == nil || !strings.Contains(err.Error(), "三段正式版本") {
		t.Fatalf("Delist() error = %v", err)
	}
	if fixture.putTotal() != 0 || fixture.deleteReleaseTotal() != 0 {
		t.Fatalf("不应有任何远端写操作")
	}
}

type delistFixture struct {
	t                 *testing.T
	server            *httptest.Server
	mu                sync.Mutex
	index             releasecatalog.Index
	sha               string
	putCount          int
	releases          []githubRelease
	refs              []string
	nextReleaseID     int64
	deleteReleaseHits int
	deleteRefHits     int
}

func newDelistFixture(t *testing.T) *delistFixture {
	fixture := &delistFixture{t: t, sha: "index-blob-sha", nextReleaseID: 100}
	fixture.index = releasecatalog.Index{
		SchemaVersion: releasecatalog.IndexSchemaVersion,
		UpdatedAt:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Artifacts:     []releasecatalog.IndexArtifact{},
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *delistFixture) publisher(t *testing.T) *Publisher {
	t.Helper()
	publisher, err := New(Config{Client: f.server.Client(), APIBaseURL: f.server.URL, Token: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	return publisher
}

// seed 登记一个发布物的一个正式版本：索引记录、远端发行与标签同时就位。
func (f *delistFixture) seed(t *testing.T, identity types.ReleaseArtifactIdentity, version string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	record := f.versionRecord(t, identity, version)
	for position := range f.index.Artifacts {
		if f.index.Artifacts[position].Kind == identity.Kind && f.index.Artifacts[position].ID == identity.ID {
			f.index.Artifacts[position].Versions = append(f.index.Artifacts[position].Versions, record)
			f.seedRemoteLocked(t, identity, version)
			return
		}
	}
	f.index.Artifacts = append(f.index.Artifacts, releasecatalog.IndexArtifact{
		Kind:     identity.Kind,
		ID:       identity.ID,
		Versions: []releasecatalog.IndexVersion{record},
	})
	f.seedRemoteLocked(t, identity, version)
}

// seedRemote 只登记远端发行与标签，用于构造索引缺失但远端残留的部分下架现场。
func (f *delistFixture) seedRemote(t *testing.T, identity types.ReleaseArtifactIdentity, version string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seedRemoteLocked(t, identity, version)
}

func (f *delistFixture) seedRemoteLocked(t *testing.T, identity types.ReleaseArtifactIdentity, version string) {
	t.Helper()
	tag, err := releasecatalog.TagName(identity, version)
	if err != nil {
		t.Fatal(err)
	}
	f.nextReleaseID++
	f.releases = append(f.releases, githubRelease{
		ID:      f.nextReleaseID,
		TagName: tag,
		HTMLURL: fmt.Sprintf("%s/release/%d", f.server.URL, f.nextReleaseID),
	})
	f.refs = append(f.refs, "refs/tags/"+tag)
}

func (f *delistFixture) versionRecord(t *testing.T, identity types.ReleaseArtifactIdentity, version string) releasecatalog.IndexVersion {
	t.Helper()
	tag, err := releasecatalog.TagName(identity, version)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := releasecatalog.ArchiveName(identity, version)
	if err != nil {
		t.Fatal(err)
	}
	return releasecatalog.IndexVersion{
		Version:        version,
		PublishedAt:    time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		SourceRevision: "0123456789abcdef0123456789abcdef01234567",
		Compatibility:  &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		ReleaseNotes:   "## " + version + "\n\n- 测试下架。",
		Packages: []releasecatalog.IndexPackage{{
			Platform:   types.ReleasePlatformWindowsX64,
			ReleaseTag: tag,
			FileName:   archive,
			SizeBytes:  1024,
			SHA256:     strings.Repeat("ab", 32),
		}},
	}
}

func (f *delistFixture) currentIndex() releasecatalog.Index {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.index
}

func (f *delistFixture) currentReleases() []githubRelease {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]githubRelease(nil), f.releases...)
}

func (f *delistFixture) currentRefs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.refs...)
}

func (f *delistFixture) putTotal() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.putCount
}

func (f *delistFixture) deleteReleaseTotal() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deleteReleaseHits
}

func (f *delistFixture) deleteRefTotal() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deleteRefHits
}

func (f *delistFixture) indexPayloadLocked() []byte {
	payload, err := json.MarshalIndent(f.index, "", "  ")
	if err != nil {
		f.t.Fatal(err)
	}
	return append(payload, '\n')
}

func (f *delistFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/contents/release-catalog/index.json") && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":  base64.StdEncoding.EncodeToString(f.indexPayloadLocked()),
			"sha":      f.sha,
			"encoding": "base64",
		})
	case strings.HasSuffix(path, "/contents/release-catalog/index.json") && r.Method == http.MethodPut:
		f.putCount++
		var request struct {
			Content string `json:"content"`
			SHA     string `json:"sha"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		payload, err := base64.StdEncoding.DecodeString(request.Content)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var index releasecatalog.Index
		if err := json.Unmarshal(payload, &index); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.index = index
		f.sha = fmt.Sprintf("blob-sha-%d", f.putCount)
		_ = json.NewEncoder(w).Encode(map[string]any{"commit": map[string]any{"sha": "commit-sha"}})
	case strings.Contains(path, "/noelle-silva/eucli-box-ai-tools/main/release-catalog/index.json"):
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(f.indexPayloadLocked())
	case strings.HasSuffix(path, "/releases") && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(append([]githubRelease(nil), f.releases...))
	case strings.Contains(path, "/releases/") && r.Method == http.MethodDelete:
		id, err := strconv.ParseInt(path[strings.LastIndex(path, "/")+1:], 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.deleteReleaseHits++
		kept := f.releases[:0:0]
		for _, item := range f.releases {
			if item.ID != id {
				kept = append(kept, item)
			}
		}
		f.releases = kept
		w.WriteHeader(http.StatusNoContent)
	case strings.Contains(path, "/git/matching-refs/tags/") && r.Method == http.MethodGet:
		prefix := path[strings.Index(path, "/git/matching-refs/tags/")+len("/git/matching-refs/tags/"):]
		items := []map[string]string{}
		for _, ref := range f.refs {
			if strings.HasPrefix(ref, "refs/tags/"+prefix) {
				items = append(items, map[string]string{"ref": ref})
			}
		}
		_ = json.NewEncoder(w).Encode(items)
	case strings.Contains(path, "/git/refs/") && r.Method == http.MethodDelete:
		full := "refs/" + path[strings.Index(path, "/git/refs/")+len("/git/refs/"):]
		f.deleteRefHits++
		kept := f.refs[:0:0]
		for _, ref := range f.refs {
			if ref != full {
				kept = append(kept, ref)
			}
		}
		f.refs = kept
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func countRemoteByPrefix(releases []githubRelease, refs []string, tagPrefix string) (int, int) {
	releaseCount := 0
	for _, item := range releases {
		if strings.HasPrefix(item.TagName, tagPrefix) {
			releaseCount++
		}
	}
	refCount := 0
	for _, ref := range refs {
		if strings.HasPrefix(ref, "refs/tags/"+tagPrefix) {
			refCount++
		}
	}
	return releaseCount, refCount
}

func delistSource(t *testing.T, kind string) types.OfficialReleaseSource {
	t.Helper()
	catalog, err := releasecatalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	source, err := catalog.SourceFor(kind)
	if err != nil {
		t.Fatal(err)
	}
	return source
}
