package releasesourcesystem

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

type releaseFixture struct {
	t            *testing.T
	server       *httptest.Server
	indexes      map[string]releasecatalog.Index
	kindRequests map[string]int
}

func newReleaseFixture(t *testing.T) *releaseFixture {
	t.Helper()
	fixture := &releaseFixture{t: t, indexes: map[string]releasecatalog.Index{}, kindRequests: map[string]int{}}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *releaseFixture) checker(t *testing.T) *releasecheck.Checker {
	t.Helper()
	checker, err := releasecheck.New(releasecheck.Config{Client: f.server.Client(), IndexBase: f.server.URL})
	if err != nil {
		t.Fatalf("releasecheck.New() error = %v", err)
	}
	return checker
}

func (f *releaseFixture) addIndexVersion(identity types.ReleaseArtifactIdentity, version string) {
	f.t.Helper()
	now := time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC)
	index := f.indexes[identity.Kind]
	if index.SchemaVersion == 0 {
		index = releasecatalog.Index{SchemaVersion: releasecatalog.IndexSchemaVersion, UpdatedAt: now}
	}
	artifactIndex := -1
	for at := range index.Artifacts {
		if index.Artifacts[at].Kind == identity.Kind && index.Artifacts[at].ID == identity.ID {
			artifactIndex = at
			break
		}
	}
	if artifactIndex < 0 {
		index.Artifacts = append(index.Artifacts, releasecatalog.IndexArtifact{Kind: identity.Kind, ID: identity.ID})
		artifactIndex = len(index.Artifacts) - 1
	}
	tag, err := releasecatalog.TagName(identity, version)
	if err != nil {
		f.t.Fatalf("TagName() error = %v", err)
	}
	fileName, err := releasecatalog.ArchiveName(identity, version)
	if err != nil {
		f.t.Fatalf("ArchiveName() error = %v", err)
	}
	compatibility := &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}
	index.Artifacts[artifactIndex].Versions = append(index.Artifacts[artifactIndex].Versions, releasecatalog.IndexVersion{
		Version:        version,
		PublishedAt:    now,
		SourceRevision: "0123456789abcdef0123456789abcdef01234567",
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
	f.indexes[identity.Kind] = index
}

func (f *releaseFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	kind := ""
	switch {
	case strings.Contains(r.URL.Path, "/noelle-silva/eucli-box-ai-tools/"):
		kind = types.ReleaseArtifactKindTool
	case strings.Contains(r.URL.Path, "/noelle-silva/eucli-box-system-plugins/"):
		kind = types.ReleaseArtifactKindPlugin
	}
	if kind == "" {
		http.NotFound(w, r)
		return
	}
	f.kindRequests[kind]++
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(f.indexes[kind])
}

// writeShelfEntry 在货架根下按「类别/发布物/版本」摆放一份最小可用成品（zip + 清单）。
func writeShelfEntry(t *testing.T, root string, kind string, id string, version string) {
	t.Helper()
	shelfDir := "ai-tools"
	if kind == types.ReleaseArtifactKindPlugin {
		shelfDir = "system-plugins"
	}
	versionDir := filepath.Join(root, shelfDir, id, version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatalf("mkdir shelf version dir: %v", err)
	}
	archiveName := kind + "-" + id + "_" + version + "_windows-x64.zip"
	archivePath := filepath.Join(versionDir, archiveName)
	writeDemoArchive(t, archivePath)
	size, sha256, err := release.RecordForFile(archivePath)
	if err != nil {
		t.Fatalf("RecordForFile() error = %v", err)
	}
	productPayload := []byte(`{"demo":true}`)
	manifest := types.ReleaseManifest{
		SchemaVersion:  release.ReleaseManifestSchemaVersion,
		Artifact:       types.ReleaseArtifactIdentity{Kind: kind, ID: id},
		Version:        version,
		Platform:       types.ReleasePlatformWindowsX64,
		OfficialSource: "https://github.com/noelle-silva/eucli-box-ai-tools",
		Compatibility:  &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"},
		Source:         types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
		TagName:        "v" + version,
		Archive:        types.ReleaseFileRecord{Name: archiveName, Size: size, SHA256: sha256},
		Files:          []types.ReleaseFileRecord{{Name: "release-product.json", Size: int64(len(productPayload)), SHA256: release.SHA256(productPayload)}},
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal shelf manifest: %v", err)
	}
	manifestPath := filepath.Join(versionDir, strings.TrimSuffix(archiveName, ".zip")+".manifest.json")
	if err := os.WriteFile(manifestPath, payload, 0o644); err != nil {
		t.Fatalf("write shelf manifest: %v", err)
	}
}

func writeDemoArchive(t *testing.T, target string) {
	t.Helper()
	output, err := os.Create(target)
	if err != nil {
		t.Fatalf("create shelf archive: %v", err)
	}
	defer output.Close()
	writer := zip.NewWriter(output)
	entry, err := writer.Create("release-product.json")
	if err != nil {
		t.Fatalf("zip create entry: %v", err)
	}
	if _, err := entry.Write([]byte(`{"demo":true}`)); err != nil {
		t.Fatalf("zip write entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
}
