package releasecheck

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

const staticIndexDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// staticIndexPayload 构造一份版本 2 静态清单：普通文件服务器只托管清单与压缩包。
func staticIndexPayload(downloadBase string, packageURL string, schemaVersion int, releaseTag string, sourceRevision string) string {
	urlField := ""
	if packageURL != "" {
		urlField = fmt.Sprintf(`,"url":%q`, packageURL)
	}
	tagField := ""
	if releaseTag != "" {
		tagField = fmt.Sprintf(`,"releaseTag":%q`, releaseTag)
	}
	revisionField := ""
	if sourceRevision != "" {
		revisionField = fmt.Sprintf(`,"sourceRevision":%q`, sourceRevision)
	}
	return fmt.Sprintf(`{
		"schemaVersion": %d,
		"updatedAt": "2026-01-01T00:00:00Z",
		"downloadBase": %q,
		"artifacts": [{
			"kind": "tool",
			"id": "demo",
			"versions": [{
				"version": "0.1.0",
				"publishedAt": "2026-01-01T00:00:00Z"%s,
				"compatibility": {"minimumVersion": "0.1.0", "maximumVersionExclusive": "0.2.0"},
				"packages": [{
					"platform": "windows-x64"%s,
					"fileName": "tool-demo_0.1.0_windows-x64.zip",
					"sizeBytes": 12,
					"sha256": %q%s
				}]
			}]
		}]
	}`, schemaVersion, downloadBase, revisionField, tagField, staticIndexDigest, urlField)
}

func TestStaticSourceReadsIndexAndBuildsCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(staticIndexPayload(server0(r)+"/store", "", 2, "", "")))
	}))
	defer server.Close()
	reader, err := NewSourceReader(IndexSource{
		Name:     "社区商店",
		Kind:     types.ReleaseSourceKindRemote,
		IndexURL: server.URL + "/store/index.json",
	}, Config{Client: server.Client()})
	if err != nil {
		t.Fatalf("NewSourceReader() error = %v", err)
	}
	candidate, err := reader.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"})
	if err != nil {
		t.Fatalf("LatestCandidate() error = %v", err)
	}
	expected := server.URL + "/store/tool-demo_0.1.0_windows-x64.zip"
	if candidate.ArchiveURL != expected {
		t.Fatalf("ArchiveURL = %q, want %q", candidate.ArchiveURL, expected)
	}
	if candidate.Source.Name != "社区商店" || candidate.Source.Kind != types.ReleaseSourceKindRemote {
		t.Fatalf("Source = %#v", candidate.Source)
	}
	if candidate.ReleaseURL != "" {
		t.Fatalf("ReleaseURL = %q, want empty for static source", candidate.ReleaseURL)
	}
	if candidate.SourceRepository != "" || candidate.SourceRevision != "" {
		t.Fatalf("source record = %q/%q, want empty", candidate.SourceRepository, candidate.SourceRevision)
	}
	if _, err := candidate.PackageSource(); err != nil {
		t.Fatalf("PackageSource() error = %v", err)
	}
}

func TestStaticSourcePrefersPackageURL(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(staticIndexPayload(server.URL+"/base", server.URL+"/files/demo.zip", 2, "", "")))
	}))
	defer server.Close()
	reader, err := NewSourceReader(IndexSource{
		Name:     "社区商店",
		Kind:     types.ReleaseSourceKindRemote,
		IndexURL: server.URL + "/store/index.json",
	}, Config{Client: server.Client()})
	if err != nil {
		t.Fatalf("NewSourceReader() error = %v", err)
	}
	candidate, err := reader.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"})
	if err != nil {
		t.Fatalf("LatestCandidate() error = %v", err)
	}
	if candidate.ArchiveURL != server.URL+"/files/demo.zip" {
		t.Fatalf("ArchiveURL = %q", candidate.ArchiveURL)
	}
}

func TestStaticSourceRejectsPackageOnAnotherHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(staticIndexPayload("", "https://cdn.example.com/demo.zip", 2, "", "")))
	}))
	defer server.Close()
	reader, err := NewSourceReader(IndexSource{
		Name:     "社区商店",
		Kind:     types.ReleaseSourceKindRemote,
		IndexURL: server.URL + "/store/index.json",
	}, Config{Client: server.Client()})
	if err != nil {
		t.Fatalf("NewSourceReader() error = %v", err)
	}
	_, err = reader.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"})
	if err == nil || !strings.Contains(err.Error(), "同一主机") {
		t.Fatalf("LatestCandidate() error = %v, want host mismatch", err)
	}
}

func TestIndexVersionOneRejectsStaticAddresses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(staticIndexPayload(server0(r), "", 1, "", "")))
	}))
	defer server.Close()
	_, err := NewSourceReader(IndexSource{
		Name:     "旧商店",
		Kind:     types.ReleaseSourceKindRemote,
		IndexURL: server.URL + "/store/index.json",
	}, Config{Client: server.Client()})
	if err != nil {
		t.Fatalf("NewSourceReader() error = %v", err)
	}
	reader, _ := NewSourceReader(IndexSource{Name: "旧商店", Kind: types.ReleaseSourceKindRemote, IndexURL: server.URL + "/store/index.json"}, Config{Client: server.Client()})
	if _, err := reader.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"}); err == nil || !strings.Contains(err.Error(), "不能声明下载基址") {
		t.Fatalf("LatestCandidate() error = %v, want version rule rejection", err)
	}
}

func TestIndexVersionTwoAllowsReleaseTagAndRevision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(staticIndexPayload(server0(r), "", 2, "demo/v0.1.0", "0123456789abcdef0123456789abcdef01234567")))
	}))
	defer server.Close()
	reader, err := NewSourceReader(IndexSource{Name: "社区商店", Kind: types.ReleaseSourceKindRemote, IndexURL: server.URL + "/store/index.json"}, Config{Client: server.Client()})
	if err != nil {
		t.Fatalf("NewSourceReader() error = %v", err)
	}
	candidate, err := reader.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"})
	if err != nil {
		t.Fatalf("LatestCandidate() error = %v", err)
	}
	if candidate.SourceRevision != "" {
		t.Fatalf("SourceRevision = %q, want empty without record repository", candidate.SourceRevision)
	}
	if _, err := candidate.PackageSource(); err != nil {
		t.Fatalf("PackageSource() error = %v", err)
	}
}

func TestSourceReaderValidatesShape(t *testing.T) {
	if _, err := NewSourceReader(IndexSource{Name: "甲", Kind: types.ReleaseSourceKindRemote, Repository: "https://github.com/a/b", IndexURL: "https://a.example.com/index.json"}, Config{}); err == nil {
		t.Fatal("remote with repository error = nil")
	}
	if _, err := NewSourceReader(IndexSource{Name: "甲", Kind: types.ReleaseSourceKindGithub, Repository: "https://github.com/a/b"}, Config{}); err == nil {
		t.Fatal("github without ref error = nil")
	}
	if _, err := NewSourceReader(IndexSource{Name: "", Kind: types.ReleaseSourceKindRemote, IndexURL: "https://a.example.com/index.json"}, Config{}); err == nil {
		t.Fatal("empty name error = nil")
	}
}

func TestReleaseCandidateStaticPackageSource(t *testing.T) {
	compatibility := types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}
	candidate := ReleaseCandidate{
		Artifact:      types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"},
		Version:       "0.1.0",
		PublishedAt:   time.Now(),
		Compatibility: &compatibility,
		Source:        types.ReleaseSourceRef{Name: "社区商店", Kind: types.ReleaseSourceKindRemote, IndexURL: "https://store.example.com/index.json"},
		ArchiveURL:    "https://store.example.com/tool-demo_0.1.0_windows-x64.zip",
		SizeBytes:     12,
		SHA256:        release.SHA256([]byte("demo")),
	}
	source, err := candidate.PackageSource()
	if err != nil {
		t.Fatalf("PackageSource() error = %v", err)
	}
	if source.Product.OfficialSource != "https://store.example.com/index.json" {
		t.Fatalf("OfficialSource = %q", source.Product.OfficialSource)
	}
	if source.Product.Source.Recorded {
		t.Fatal("Recorded = true, want false for static source")
	}
}

// server0 从请求推导服务地址，供处理器内构造自引用地址。
func server0(r *http.Request) string {
	return "http://" + r.Host
}
