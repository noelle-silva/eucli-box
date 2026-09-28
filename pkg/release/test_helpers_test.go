package release

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"eucli-box/pkg/types"
)

const testOfficialSource = "https://github.com/noelle-silva/eucli-box-ai-tools"

var testCompatibility = types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}

// buildTestToolContents 生成一个内容合法、身份完整的工具成品目录，返回目录和逐文件资料。
func buildTestToolContents(t *testing.T, id string, version string) (string, []types.ReleaseFileRecord) {
	t.Helper()
	root := t.TempDir()
	binaryName := id + ".exe"
	binaryPath := filepath.ToSlash(filepath.Join("binary", "windows-amd64", binaryName))
	definition := types.ToolDefinition{
		ID:                    id,
		Name:                  "Demo " + id,
		Description:           "demo tool",
		Version:               version,
		EucliBoxCompatibility: testCompatibility,
		DefaultInvocationMode: "sync",
		Type:                  "local",
		BodyDirectory:         ".",
		Binaries:              []types.ToolBinary{{GOOS: "windows", GOARCH: "amd64", Path: binaryPath}},
	}
	definitionPayload, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	contents := map[string][]byte{
		"definition.json": definitionPayload,
		binaryPath:        []byte("tool-binary"),
		"README.md":       []byte("# " + id + "\n"),
		"CHANGELOG.md":    []byte("## " + version + "\n"),
	}
	for name, payload := range contents {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	product := productForTestTool(id, version)
	productPayload, err := json.MarshalIndent(product, "", "  ")
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "release-product.json"), productPayload, 0o644); err != nil {
		t.Fatalf("write product: %v", err)
	}
	files, err := CollectFileRecords(root)
	if err != nil {
		t.Fatalf("collect files: %v", err)
	}
	return root, files
}

// buildMinimalToolContents 生成一个纯本地工具包目录：
// 只有工具定义文件与可执行文件，没有任何说明文档、更新记录或发行资料。
func buildMinimalToolContents(t *testing.T, id string, version string) string {
	t.Helper()
	root := t.TempDir()
	binaryPath := filepath.ToSlash(filepath.Join("binary", "windows-amd64", id+".exe"))
	definition := types.ToolDefinition{
		ID:                    id,
		Name:                  "Demo " + id,
		Description:           "demo tool",
		Version:               version,
		EucliBoxCompatibility: testCompatibility,
		DefaultInvocationMode: "sync",
		Type:                  "local",
		BodyDirectory:         ".",
		Binaries:              []types.ToolBinary{{GOOS: "windows", GOARCH: "amd64", Path: binaryPath}},
	}
	definitionPayload, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	contents := map[string][]byte{
		"definition.json": definitionPayload,
		binaryPath:        []byte("tool-binary"),
	}
	for name, payload := range contents {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return root
}

// rewriteToolDefinitionVersion 改写包内工具定义的版本字段。
func rewriteToolDefinitionVersion(t *testing.T, root string, version string) {
	t.Helper()
	definitionPath := filepath.Join(root, "definition.json")
	payload, err := os.ReadFile(definitionPath)
	if err != nil {
		t.Fatalf("read definition: %v", err)
	}
	var definition types.ToolDefinition
	if err := json.Unmarshal(payload, &definition); err != nil {
		t.Fatalf("decode definition: %v", err)
	}
	definition.Version = version
	payload, err = json.MarshalIndent(definition, "", "  ")
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	if err := os.WriteFile(definitionPath, payload, 0o644); err != nil {
		t.Fatalf("write definition: %v", err)
	}
}

// productForTestTool 构造与 buildTestToolContents 对应的工具成品身份记录。
func productForTestTool(id string, version string) types.ReleaseProductRecord {
	return types.ReleaseProductRecord{
		SchemaVersion:  ReleaseManifestSchemaVersion,
		Artifact:       types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: id},
		Version:        version,
		Platform:       types.ReleasePlatformWindowsX64,
		OfficialSource: testOfficialSource,
		Compatibility:  &testCompatibility,
		Source:         types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
	}
}

// zipDirectory 把目录打包成 zip 文件。
func zipDirectory(t *testing.T, root string, target string) {
	t.Helper()
	output, err := os.Create(target)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(output)
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := writer.Create(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		_, err = file.Write(payload)
		return err
	})
	closeErr := writer.Close()
	outputCloseErr := output.Close()
	if walkErr != nil {
		t.Fatalf("zip walk: %v", walkErr)
	}
	if closeErr != nil {
		t.Fatalf("close zip: %v", closeErr)
	}
	if outputCloseErr != nil {
		t.Fatalf("close zip file: %v", outputCloseErr)
	}
}

// manifestForArchive 构造与 buildTestToolContents 内容一致的合法工具清单。
func manifestForArchive(t *testing.T, id string, version string, archivePath string, files []types.ReleaseFileRecord) types.ReleaseManifest {
	t.Helper()
	payload, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("stat archive: %v", err)
	}
	return types.ReleaseManifest{
		SchemaVersion:  ReleaseManifestSchemaVersion,
		Artifact:       types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: id},
		Version:        version,
		Platform:       types.ReleasePlatformWindowsX64,
		TagName:        id + "/v" + version,
		OfficialSource: testOfficialSource,
		Compatibility:  &testCompatibility,
		Source:         types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
		Archive:        types.ReleaseFileRecord{Name: filepath.Base(archivePath), Size: info.Size(), SHA256: SHA256(payload)},
		Files:          files,
	}
}

// makeTestToolArchive 一次生成工具成品目录、压缩包和对应清单。
func makeTestToolArchive(t *testing.T, id string, version string) (string, types.ReleaseManifest) {
	t.Helper()
	root, files := buildTestToolContents(t, id, version)
	archivePath := filepath.Join(t.TempDir(), id+"-"+version+".zip")
	zipDirectory(t, root, archivePath)
	return archivePath, manifestForArchive(t, id, version, archivePath, files)
}

// productFromManifest 从测试清单构造对应的包内期望产品记录。
func productFromManifest(manifest types.ReleaseManifest) types.ReleaseProductRecord {
	return types.ReleaseProductRecord{
		SchemaVersion:  manifest.SchemaVersion,
		Artifact:       manifest.Artifact,
		Version:        manifest.Version,
		Platform:       manifest.Platform,
		OfficialSource: manifest.OfficialSource,
		Compatibility:  manifest.Compatibility,
		Source:         manifest.Source,
		DataVersion:    manifest.DataVersion,
		ExternalAssets: manifest.ExternalAssets,
	}
}
