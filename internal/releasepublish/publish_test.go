package releasepublish

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

func TestPublishCreatesDraftVerifiesArchiveAndPublishes(t *testing.T) {
	fixture := newPublishFixture(t)
	publisher := fixture.publisher(t)
	input := testPublishInput(t)
	manifest := manifestForInput(t, input)

	result, err := publisher.Publish(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.draft || fixture.patchCount != 1 || fixture.downloadCount != 2 {
		t.Fatalf("fixture state: draft=%v patch=%d downloads=%d", fixture.draft, fixture.patchCount, fixture.downloadCount)
	}
	if result.TagName != manifest.TagName || len(result.Assets) != 1 || result.ReleaseURL == "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestPublishRejectsDuplicateVersionBeforeCreatingDraft(t *testing.T) {
	fixture := newPublishFixture(t)
	input := testPublishInput(t)
	fixture.duplicateTag = manifestForInput(t, input).TagName
	publisher := fixture.publisher(t)

	if _, err := publisher.Publish(context.Background(), input); err == nil || !strings.Contains(err.Error(), "相同发布物和版本") {
		t.Fatalf("Publish() error = %v", err)
	}
	if fixture.createCount != 0 {
		t.Fatalf("create count = %d", fixture.createCount)
	}
}

func TestPublishKeepsDraftWhenUploadFails(t *testing.T) {
	fixture := newPublishFixture(t)
	fixture.failUpload = true
	publisher := fixture.publisher(t)

	result, err := publisher.Publish(context.Background(), testPublishInput(t))
	if err == nil || !strings.Contains(err.Error(), "未公开发行已保留") {
		t.Fatalf("Publish() error = %v", err)
	}
	if result.ReleaseID == 0 || !fixture.draft || fixture.patchCount != 0 {
		t.Fatalf("result=%#v draft=%v patch=%d", result, fixture.draft, fixture.patchCount)
	}
}

func TestPublishKeepsDraftWhenDownloadedAssetDiffers(t *testing.T) {
	fixture := newPublishFixture(t)
	fixture.changeDownload = true
	publisher := fixture.publisher(t)

	if _, err := publisher.Publish(context.Background(), testPublishInput(t)); err == nil || !strings.Contains(err.Error(), "未公开发行已保留") {
		t.Fatalf("Publish() error = %v", err)
	}
	if !fixture.draft || fixture.patchCount != 0 {
		t.Fatalf("draft=%v patch=%d", fixture.draft, fixture.patchCount)
	}
}

func TestDownloadPublishedReadsIndexAndDownloadsArchive(t *testing.T) {
	fixture := newPublishFixture(t)
	input := testPublishInput(t)
	manifest := manifestForInput(t, input)
	fixture.seedPublished(input)
	publisher, err := New(Config{Client: fixture.server.Client(), APIBaseURL: fixture.server.URL})
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	result, err := publisher.DownloadPublished(context.Background(), manifest.Artifact, manifest.Version, target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Product.Version != manifest.Version || result.Product.Artifact != manifest.Artifact || result.ReleaseURL == "" {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(result.ArchivePath); err != nil {
		t.Fatalf("archive missing: %v", err)
	}
}

func TestUpdateIndexAppendsVersionRecord(t *testing.T) {
	fixture := newPublishFixture(t)
	fixture.indexPayload = nil
	fixture.indexSHA = ""
	publisher := fixture.publisher(t)
	catalog, err := releasecatalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}
	source, err := catalog.SourceFor(identity.Kind)
	if err != nil {
		t.Fatal(err)
	}
	input := testPublishInput(t)
	manifest := manifestForInput(t, input)
	update := IndexUpdate{
		Artifact:       identity,
		Version:        manifest.Version,
		PublishedAt:    time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC),
		SourceRevision: manifest.Source.Commit,
		Compatibility:  manifest.Compatibility,
		ReleaseNotes:   "## 0.1.0\n\n- 测试正式发布。",
		Package: releasecatalog.IndexPackage{
			Platform:   types.ReleasePlatformWindowsX64,
			ReleaseTag: manifest.TagName,
			FileName:   manifest.Archive.Name,
			SizeBytes:  manifest.Archive.Size,
			SHA256:     manifest.Archive.SHA256,
		},
	}
	if err := publisher.UpdateIndex(context.Background(), source, update); err != nil {
		t.Fatal(err)
	}
	if fixture.putIndexCount != 1 {
		t.Fatalf("put count = %d", fixture.putIndexCount)
	}
	index, err := releasecatalog.DecodeIndex(fixture.indexPayload)
	if err != nil {
		t.Fatalf("stored index invalid: %v", err)
	}
	version, ok := index.LatestVersion(identity)
	if !ok || version.Version != manifest.Version {
		t.Fatalf("index version = %#v ok=%v", version, ok)
	}
}

func TestUpdateIndexRejectsDuplicateVersion(t *testing.T) {
	fixture := newPublishFixture(t)
	fixture.indexPayload = nil
	fixture.indexSHA = ""
	publisher := fixture.publisher(t)
	catalog, err := releasecatalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}
	source, err := catalog.SourceFor(identity.Kind)
	if err != nil {
		t.Fatal(err)
	}
	input := testPublishInput(t)
	manifest := manifestForInput(t, input)
	update := IndexUpdate{
		Artifact:       identity,
		Version:        manifest.Version,
		PublishedAt:    time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC),
		SourceRevision: manifest.Source.Commit,
		Compatibility:  manifest.Compatibility,
		Package: releasecatalog.IndexPackage{
			Platform:   types.ReleasePlatformWindowsX64,
			ReleaseTag: manifest.TagName,
			FileName:   manifest.Archive.Name,
			SizeBytes:  manifest.Archive.Size,
			SHA256:     manifest.Archive.SHA256,
		},
	}
	if err := publisher.UpdateIndex(context.Background(), source, update); err != nil {
		t.Fatal(err)
	}
	if err := publisher.UpdateIndex(context.Background(), source, update); err == nil || !strings.Contains(err.Error(), "相同发布物和版本") {
		t.Fatalf("second UpdateIndex() error = %v", err)
	}
}

type publishFixture struct {
	t              *testing.T
	server         *httptest.Server
	mu             sync.Mutex
	draft          bool
	tag            string
	body           string
	assets         []githubReleaseAsset
	payloads       map[int64][]byte
	duplicateTag   string
	failUpload     bool
	changeDownload bool
	createCount    int
	patchCount     int
	downloadCount  int
	nextAssetID    int64
	indexPayload   []byte
	indexSHA       string
	putIndexCount  int
}

func newPublishFixture(t *testing.T) *publishFixture {
	fixture := &publishFixture{t: t, draft: true, payloads: map[int64][]byte{}, nextAssetID: 10}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *publishFixture) publisher(t *testing.T) *Publisher {
	t.Helper()
	publisher, err := New(Config{Client: f.server.Client(), APIBaseURL: f.server.URL, Token: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	return publisher
}

func (f *publishFixture) seedPublished(input PublishInput) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.draft = false
	manifest := manifestForInput(f.t, input)
	f.tag = manifest.TagName
	notes, _ := os.ReadFile(input.NotesPath)
	f.body = strings.TrimSpace(string(notes))
	payload, _ := os.ReadFile(input.ArchivePath)
	f.nextAssetID++
	id := f.nextAssetID
	f.payloads[id] = payload
	f.assets = append(f.assets, githubReleaseAsset{ID: id, Name: filepath.Base(input.ArchivePath), Size: int64(len(payload)), URL: fmt.Sprintf("%s/assets/%d", f.server.URL, id), BrowserDownloadURL: fmt.Sprintf("%s/assets/%d", f.server.URL, id)})
	index := releasecatalog.Index{
		SchemaVersion: releasecatalog.IndexSchemaVersion,
		UpdatedAt:     time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC),
		Artifacts: []releasecatalog.IndexArtifact{{
			Kind: manifest.Artifact.Kind,
			ID:   manifest.Artifact.ID,
			Versions: []releasecatalog.IndexVersion{{
				Version:        manifest.Version,
				PublishedAt:    time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC),
				SourceRevision: manifest.Source.Commit,
				Compatibility:  manifest.Compatibility,
				ReleaseNotes:   strings.TrimSpace(string(notes)),
				Packages: []releasecatalog.IndexPackage{{
					Platform:   types.ReleasePlatformWindowsX64,
					ReleaseTag: manifest.TagName,
					FileName:   manifest.Archive.Name,
					SizeBytes:  manifest.Archive.Size,
					SHA256:     manifest.Archive.SHA256,
				}},
			}},
		}},
	}
	payloadJSON, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		f.t.Fatal(err)
	}
	payloadJSON = append(payloadJSON, '\n')
	f.indexPayload = payloadJSON
	f.indexSHA = "blob-sha-of-index"
}

func (f *publishFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		f.downloadCount++
		var id int64
		_, _ = fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/assets/"), "%d", &id)
		payload, ok := f.payloads[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if f.changeDownload {
			payload = append([]byte(nil), payload...)
			payload[0] ^= 0xff
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/contents/release-catalog/index.json") && r.Method == http.MethodGet {
		if f.indexPayload == nil {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":  base64.StdEncoding.EncodeToString(f.indexPayload),
			"sha":      f.indexSHA,
			"encoding": "base64",
		})
		return
	}
	if strings.HasSuffix(r.URL.Path, "/contents/release-catalog/index.json") && r.Method == http.MethodPut {
		f.putIndexCount++
		var request struct {
			Content string `json:"content"`
			SHA     string `json:"sha"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		payload, err := base64.StdEncoding.DecodeString(request.Content)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.indexPayload = payload
		f.indexSHA = "blob-sha-" + fmt.Sprint(f.putIndexCount)
		_ = json.NewEncoder(w).Encode(map[string]any{"commit": map[string]any{"sha": "commit-sha-of-index"}})
		return
	}
	if strings.HasSuffix(r.URL.Path, "/noelle-silva/eucli-box-ai-tools/main/release-catalog/index.json") {
		if f.indexPayload == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(f.indexPayload)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/releases/download/") {
		if len(f.assets) == 0 {
			http.NotFound(w, r)
			return
		}
		asset := f.assets[0]
		payload, ok := f.payloads[asset.ID]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/upload/1") {
		if f.failUpload {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		payload, _ := io.ReadAll(r.Body)
		f.nextAssetID++
		id := f.nextAssetID
		name := r.URL.Query().Get("name")
		asset := githubReleaseAsset{ID: id, Name: name, Size: int64(len(payload)), URL: fmt.Sprintf("%s/assets/%d", f.server.URL, id), BrowserDownloadURL: fmt.Sprintf("%s/assets/%d", f.server.URL, id)}
		f.assets = append(f.assets, asset)
		f.payloads[id] = payload
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(asset)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/releases") && r.Method == http.MethodGet {
		items := []githubRelease{}
		if f.duplicateTag != "" {
			items = append(items, githubRelease{ID: 9, TagName: f.duplicateTag, Draft: false})
		} else if f.tag != "" && !f.draft {
			items = append(items, f.release())
		}
		_ = json.NewEncoder(w).Encode(items)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/releases") && r.Method == http.MethodPost {
		f.createCount++
		var request struct {
			TagName string `json:"tag_name"`
			Body    string `json:"body"`
			Draft   bool   `json:"draft"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		f.tag = request.TagName
		f.body = request.Body
		f.draft = request.Draft
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(f.release())
		return
	}
	if strings.HasSuffix(r.URL.Path, "/releases/1") && r.Method == http.MethodGet {
		_ = json.NewEncoder(w).Encode(f.release())
		return
	}
	if strings.HasSuffix(r.URL.Path, "/releases/1") && r.Method == http.MethodPatch {
		f.patchCount++
		var request struct {
			Draft bool `json:"draft"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		f.draft = request.Draft
		_ = json.NewEncoder(w).Encode(f.release())
		return
	}
	http.NotFound(w, r)
}

func (f *publishFixture) release() githubRelease {
	return githubRelease{
		ID:         1,
		TagName:    f.tag,
		Draft:      f.draft,
		Prerelease: false,
		HTMLURL:    f.server.URL + "/release/1",
		UploadURL:  f.server.URL + "/upload/1{?name,label}",
		Body:       f.body,
		Assets:     append([]githubReleaseAsset(nil), f.assets...),
	}
}

func testPublishInput(t *testing.T) PublishInput {
	t.Helper()
	catalog, err := releasecatalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}
	source, err := catalog.SourceFor(identity.Kind)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := releasecatalog.TagName(identity, "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	archiveName, err := releasecatalog.ArchiveName(identity, "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	archivePayload := []byte("verified archive")
	archivePath := filepath.Join(directory, archiveName)
	if err := os.WriteFile(archivePath, archivePayload, 0o644); err != nil {
		t.Fatal(err)
	}
	compatibility := &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}
	manifest := types.ReleaseManifest{
		SchemaVersion:  release.ReleaseManifestSchemaVersion,
		Artifact:       identity,
		Version:        "0.1.0",
		Platform:       types.ReleasePlatformWindowsX64,
		TagName:        tag,
		OfficialSource: source.Repository,
		Compatibility:  compatibility,
		Source:         types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
		Archive:        types.ReleaseFileRecord{Name: archiveName, Size: int64(len(archivePayload)), SHA256: release.SHA256(archivePayload)},
		Files:          []types.ReleaseFileRecord{{Name: "release-product.json", Size: 2, SHA256: release.SHA256([]byte("{}"))}},
	}
	manifestPayload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestPayload = append(manifestPayload, '\n')
	manifestPath := filepath.Join(directory, strings.TrimSuffix(archiveName, ".zip")+".manifest.json")
	if err := os.WriteFile(manifestPath, manifestPayload, 0o644); err != nil {
		t.Fatal(err)
	}
	notesPath := filepath.Join(directory, "release-notes.md")
	if err := os.WriteFile(notesPath, []byte("## 0.1.0\n\n- 测试正式发布。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return PublishInput{ArchivePath: archivePath, ManifestPath: manifestPath, NotesPath: notesPath}
}

func manifestForInput(t *testing.T, input PublishInput) types.ReleaseManifest {
	t.Helper()
	payload, err := os.ReadFile(input.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := release.DecodeReleaseManifest(payload)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func escapedTag(tag string) string {
	return url.PathEscape(tag)
}
