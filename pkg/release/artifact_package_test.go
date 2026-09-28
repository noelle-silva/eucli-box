package release

import (
	"archive/zip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

type packageFixture struct {
	t              *testing.T
	server         *httptest.Server
	product        types.ReleaseProductRecord
	archivePayload []byte
	sizeBytes      int64
	sha256         string
}

func newPackageFixture(t *testing.T, id string, version string) *packageFixture {
	t.Helper()
	archivePath, manifest := makeTestToolArchive(t, id, version)
	payload, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	fixture := &packageFixture{
		t:              t,
		product:        productFromManifest(manifest),
		archivePayload: payload,
		sizeBytes:      int64(len(payload)),
		sha256:         SHA256(payload),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *packageFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/archive":
		_, _ = w.Write(f.archivePayload)
	default:
		http.NotFound(w, r)
	}
}

func (f *packageFixture) source() ArtifactPackageSource {
	return ArtifactPackageSource{
		Artifact:   f.product.Artifact,
		Product:    f.product,
		ArchiveURL: f.server.URL + "/archive",
		SizeBytes:  f.sizeBytes,
		SHA256:     f.sha256,
	}
}

func (f *packageFixture) acquire(t *testing.T, source ArtifactPackageSource) (ValidatedPackage, error) {
	t.Helper()
	return AcquireAndValidatePackage(context.Background(), AcquirePackageOptions{
		Source:       source,
		DownloadDir:  filepath.Join(t.TempDir(), "download"),
		ExtractedDir: filepath.Join(t.TempDir(), "extracted"),
		Client:       f.server.Client(),
	})
}

func TestAcquireAndValidatePackageReturnsValidatedPackage(t *testing.T) {
	fixture := newPackageFixture(t, "demo", "0.1.0")
	validated, err := fixture.acquire(t, fixture.source())
	if err != nil {
		t.Fatalf("AcquireAndValidatePackage() error = %v", err)
	}
	if validated.Artifact != fixture.product.Artifact || validated.Version != "0.1.0" {
		t.Fatalf("validated package = %#v", validated)
	}
	if _, err := os.Stat(filepath.Join(validated.Directory, "definition.json")); err != nil {
		t.Fatalf("missing definition.json: %v", err)
	}
}

func TestAcquireAndValidatePackageAcceptsDevelopmentVersion(t *testing.T) {
	fixture := newPackageFixture(t, "demo", "0.1.0.2")
	validated, err := fixture.acquire(t, fixture.source())
	if err != nil {
		t.Fatalf("AcquireAndValidatePackage() error = %v", err)
	}
	if validated.Version != "0.1.0.2" {
		t.Fatalf("validated version = %s", validated.Version)
	}
}

// TestValidatePackageDirectoryAcceptsMinimalToolPackage 最小标准：
// 没有说明文档、更新记录、成品身份资料的工具包同样合法。
func TestValidatePackageDirectoryAcceptsMinimalToolPackage(t *testing.T) {
	root := buildMinimalToolContents(t, "demo", "0.1.0")
	validated, err := ValidatePackageDirectory(ValidatePackageDirectoryOptions{Directory: root, Kind: types.ReleaseArtifactKindTool})
	if err != nil {
		t.Fatalf("ValidatePackageDirectory() error = %v", err)
	}
	if validated.Artifact.ID != "demo" || validated.Version != "0.1.0" {
		t.Fatalf("validated = %#v", validated)
	}
	if len(validated.Files) != 2 {
		t.Fatalf("files = %#v", validated.Files)
	}
}

func TestValidatePackageDirectoryRejectsMissingDefinition(t *testing.T) {
	root := buildMinimalToolContents(t, "demo", "0.1.0")
	if err := os.Remove(filepath.Join(root, "definition.json")); err != nil {
		t.Fatalf("remove definition: %v", err)
	}
	_, err := ValidatePackageDirectory(ValidatePackageDirectoryOptions{Directory: root, Kind: types.ReleaseArtifactKindTool})
	if err == nil || !strings.Contains(err.Error(), "工具定义文件") {
		t.Fatalf("ValidatePackageDirectory() error = %v", err)
	}
}

func TestValidatePackageDirectoryRejectsMissingBinary(t *testing.T) {
	root := buildMinimalToolContents(t, "demo", "0.1.0")
	if err := os.Remove(filepath.Join(root, "binary", "windows-amd64", "demo.exe")); err != nil {
		t.Fatalf("remove binary: %v", err)
	}
	_, err := ValidatePackageDirectory(ValidatePackageDirectoryOptions{Directory: root, Kind: types.ReleaseArtifactKindTool})
	if err == nil || !strings.Contains(err.Error(), "缺少必需文件") {
		t.Fatalf("ValidatePackageDirectory() error = %v", err)
	}
}

func TestValidatePackageDirectoryRejectsInvalidVersion(t *testing.T) {
	root := buildMinimalToolContents(t, "demo", "0.1.0")
	rewriteToolDefinitionVersion(t, root, "not-a-version")
	_, err := ValidatePackageDirectory(ValidatePackageDirectoryOptions{Directory: root, Kind: types.ReleaseArtifactKindTool})
	if err == nil || !strings.Contains(err.Error(), "版本无效") {
		t.Fatalf("ValidatePackageDirectory() error = %v", err)
	}
}

// TestValidatePackageDirectoryAcceptsMinimalPluginPackage 插件最小标准：
// 身份声明、配置默认值与可执行文件即可，不要求任何说明文档或发行资料。
func TestValidatePackageDirectoryAcceptsMinimalPluginPackage(t *testing.T) {
	root := t.TempDir()
	manifest := types.SystemPluginManifest{
		ID:      "demo",
		Name:    "Demo",
		Version: "0.1.0",
		Binaries: []types.SystemPluginBinary{
			{GOOS: "windows", GOARCH: "amd64", Path: filepath.ToSlash(filepath.Join("binary", "demo.exe"))},
		},
	}
	manifestPayload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	files := map[string][]byte{
		"manifest.json": manifestPayload,
		"config.json":   []byte("{}"),
		filepath.ToSlash(filepath.Join("binary", "demo.exe")): []byte("plugin-binary"),
	}
	for name, payload := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	validated, err := ValidatePackageDirectory(ValidatePackageDirectoryOptions{Directory: root, Kind: types.ReleaseArtifactKindPlugin})
	if err != nil {
		t.Fatalf("ValidatePackageDirectory() error = %v", err)
	}
	if validated.Artifact.ID != "demo" || validated.Version != "0.1.0" {
		t.Fatalf("validated = %#v", validated)
	}
}

func TestValidatePackageDirectoryRejectsPluginWithoutConfig(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"manifest.json": `{"id":"demo","version":"0.1.0","binaries":[{"goos":"windows","goarch":"amd64","path":"binary/demo.exe"}]}`,
		filepath.ToSlash(filepath.Join("binary", "demo.exe")): "plugin-binary",
	}
	for name, payload := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	_, err := ValidatePackageDirectory(ValidatePackageDirectoryOptions{Directory: root, Kind: types.ReleaseArtifactKindPlugin})
	if err == nil || !strings.Contains(err.Error(), "config.json") {
		t.Fatalf("ValidatePackageDirectory() error = %v", err)
	}
}

func TestAcquireAndValidatePackageReportsProgress(t *testing.T) {
	fixture := newPackageFixture(t, "demo", "0.1.0")
	var progress DownloadProgress
	_, err := AcquireAndValidatePackage(context.Background(), AcquirePackageOptions{
		Source:       fixture.source(),
		DownloadDir:  filepath.Join(t.TempDir(), "download"),
		ExtractedDir: filepath.Join(t.TempDir(), "extracted"),
		Client:       fixture.server.Client(),
		OnProgress: func(value DownloadProgress) {
			progress = value
		},
	})
	if err != nil {
		t.Fatalf("AcquireAndValidatePackage() error = %v", err)
	}
	if progress.ReceivedBytes <= 0 || progress.TotalBytes != int64(len(fixture.archivePayload)) {
		t.Fatalf("progress = %#v", progress)
	}
}

func TestAcquireAndValidatePackageRejectsCorruptedArchiveDigest(t *testing.T) {
	fixture := newPackageFixture(t, "demo", "0.1.0")
	fixture.archivePayload = append([]byte("tampered"), fixture.archivePayload...)
	_, err := fixture.acquire(t, fixture.source())
	if err == nil || !strings.Contains(err.Error(), "下载压缩包失败") {
		t.Fatalf("AcquireAndValidatePackage() error = %v", err)
	}
}

func TestAcquireAndValidatePackageRejectsArchivePathEscape(t *testing.T) {
	fixture := newPackageFixture(t, "demo", "0.1.0")
	evil := filepath.Join(t.TempDir(), "evil.zip")
	zipWithRawEntry(t, evil, "../evil.txt", []byte("escaped"))
	payload, err := os.ReadFile(evil)
	if err != nil {
		t.Fatalf("read evil archive: %v", err)
	}
	fixture.archivePayload = payload
	fixture.sizeBytes = int64(len(payload))
	fixture.sha256 = SHA256(payload)
	_, err = fixture.acquire(t, fixture.source())
	if err == nil || !strings.Contains(err.Error(), "解开压缩包失败") {
		t.Fatalf("AcquireAndValidatePackage() error = %v", err)
	}
}

func TestAcquireAndValidatePackageRejectsInvalidSource(t *testing.T) {
	fixture := newPackageFixture(t, "demo", "0.1.0")
	source := fixture.source()
	source.Artifact = types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "other"}
	_, err := fixture.acquire(t, source)
	if err == nil {
		t.Fatal("AcquireAndValidatePackage() error = nil")
	}
}

func TestAcquireAndValidatePackageRejectsProductMismatch(t *testing.T) {
	fixture := newPackageFixture(t, "demo", "0.1.0")
	source := fixture.source()
	source.Product.Version = "9.9.9"
	_, err := fixture.acquire(t, source)
	if err == nil || !strings.Contains(err.Error(), "包内版本与候选版本不一致") {
		t.Fatalf("AcquireAndValidatePackage() error = %v", err)
	}
}

func TestValidateArtifactPackageSourceRejectsMissingFacts(t *testing.T) {
	source := ArtifactPackageSource{}
	if err := ValidateArtifactPackageSource(source); err == nil {
		t.Fatal("ValidateArtifactPackageSource() error = nil")
	}
}

func zipWithRawEntry(t *testing.T, target string, name string, payload []byte) {
	t.Helper()
	output, err := os.Create(target)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer output.Close()
	writer := zip.NewWriter(output)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if _, err := entry.Write(payload); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}
