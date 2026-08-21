package releasecheck

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

// writePluginDevelopmentPackage 生成一个满足全校验的开发版插件成品。
func writePluginDevelopmentPackage(t *testing.T, root string, id string, version string, commit string) {
	t.Helper()
	dir := filepath.Join(root, "plugin-"+id, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	file, err := writer.Create("manifest.json")
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := file.Write([]byte(`{"id":"` + id + `"}`)); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	payload := buf.Bytes()
	archiveName := id + "-" + version + ".zip"
	manifest := types.ReleaseManifest{
		SchemaVersion:    release.ReleaseManifestSchemaVersion,
		Artifact:         types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: id},
		Version:          version,
		Platform:         types.ReleasePlatformWindowsX64,
		TagName:          "plugin-" + id + "-" + version,
		OfficialSource:   "https://github.com/noelle-silva/eucli-box-system-plugins",
		Compatibility:    &types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.9.0"},
		Source:           types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: commit, Recorded: true},
		VerificationOnly: true,
		Archive:          types.ReleaseFileRecord{Name: archiveName, Size: int64(len(payload)), SHA256: release.SHA256(payload)},
		Files: []types.ReleaseFileRecord{
			{Name: "manifest.json", Size: int64(len(`{"id":"` + id + `"}`)), SHA256: release.SHA256([]byte(`{"id":"` + id + `"}`))},
			{Name: "release-product.json", Size: 1, SHA256: release.SHA256([]byte{})}},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+"-"+version+".manifest.json"), manifestBytes, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, archiveName), payload, 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
}

var testCommit = "0123456789abcdef0123456789abcdef01234567"

func TestDevelopmentSourceReaderServesPluginIdentity(t *testing.T) {
	root := t.TempDir()
	writePluginDevelopmentPackage(t, root, "weather", "0.1.0", testCommit)
	reader, err := NewDevelopmentSourceReader("1", root)
	if err != nil {
		t.Fatalf("NewDevelopmentSourceReader() error = %v", err)
	}
	candidate, err := reader.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "weather"})
	if err != nil {
		t.Fatalf("LatestCandidate() error = %v", err)
	}
	if candidate.Version != "0.1.0" || !candidate.Development || candidate.ArchiveURL == "" || candidate.SHA256 == "" {
		t.Fatalf("candidate = %#v", candidate)
	}
	source, err := DevelopmentSource(candidate)
	if err != nil {
		t.Fatalf("DevelopmentSource() error = %v", err)
	}
	if !source.Development {
		t.Fatalf("source = %#v", source)
	}
}

func TestDevelopmentSourceReaderIgnoresMissingPluginPackage(t *testing.T) {
	root := t.TempDir()
	reader, err := NewDevelopmentSourceReader("1", root)
	if err != nil {
		t.Fatalf("NewDevelopmentSourceReader() error = %v", err)
	}
	_, err = reader.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "weather"})
	if err == nil {
		t.Fatal("LatestCandidate() error = nil, want missing package error")
	}
}

// TestDevelopmentSourceReaderUsesRealBuiltPackage 用当前源码构建的开发版工具
// 成品验证本地候选；成品目录通过环境变量传入，缺失时跳过。
func TestDevelopmentSourceReaderUsesRealBuiltPackage(t *testing.T) {
	packageRoot := strings.TrimSpace(os.Getenv("EUCLI_DEV_TOOL_TEST_PACKAGE_ROOT"))
	if packageRoot == "" {
		t.Skip("未设置 EUCLI_DEV_TOOL_TEST_PACKAGE_ROOT，跳过开发成品集成测试")
	}
	if _, err := os.Stat(packageRoot); err != nil {
		t.Skipf("开发成品目录不存在，跳过：%s", err)
	}
	reader, err := NewDevelopmentSourceReader("1", packageRoot)
	if err != nil {
		t.Fatalf("NewDevelopmentSourceReader() error = %v", err)
	}
	candidate, err := reader.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "shell_command"})
	if err != nil {
		t.Fatalf("LatestCandidate() error = %v", err)
	}
	if candidate.Version == "" || candidate.ArchiveURL == "" || candidate.SHA256 == "" {
		t.Fatalf("candidate = %#v", candidate)
	}
	source, err := DevelopmentSource(candidate)
	if err != nil {
		t.Fatalf("DevelopmentSource() error = %v", err)
	}
	if !source.Development || source.ArchiveURL != candidate.ArchiveURL {
		t.Fatalf("development source = %#v", source)
	}
}

func TestDevelopmentSourceInactiveByDefault(t *testing.T) {
	reader, err := NewDevelopmentSourceReader("", "")
	if err != nil {
		t.Fatalf("NewDevelopmentSourceReader() error = nil expected %v", err)
	}
	if reader != nil {
		t.Fatalf("reader should be nil when source flag absent")
	}
}

func TestDevelopmentSourceRejectsWrongIdentity(t *testing.T) {
	_, err := NewDevelopmentSourceReader("1", filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("NewDevelopmentSourceReader() error = nil, want missing root error")
	}
}
