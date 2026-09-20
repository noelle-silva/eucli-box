package releaseartifact

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"devtools/common/releaseasset"
	"devtools/common/releaseops"
	"eucli-box/pkg/artifactcatalog"
	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
	"eucli-box/pkg/workspace"
)

type BuildOptions struct {
	Root         string
	Target       string
	WorkRoot     string
	OutputRoot   string
	EvidenceRoot string
	AssetRoot    string
	// VersionOverride 非空表示开发构建：按给定版本号（四段开发版本或三段版本）
	// 制作成品，允许源码未完全记录，同版本再次构建直接覆盖旧成品。
	VersionOverride string
}

type BuildResult struct {
	Manifest     types.ReleaseManifest
	ArchivePath  string
	ManifestPath string
	NotesPath    string
	OutputDir    string
	WorkDir      string
}

func Build(ctx context.Context, options BuildOptions) (BuildResult, error) {
	if ctx == nil {
		return BuildResult{}, fmt.Errorf("制作上下文不能为空")
	}
	root, err := cleanExistingDirectory(options.Root, "仓库根目录")
	if err != nil {
		return BuildResult{}, err
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return BuildResult{}, fmt.Errorf("本期正式成品只能在 Windows x64 环境制作")
	}
	sources, err := releasecatalog.LoadSources()
	if err != nil {
		return BuildResult{}, err
	}
	roster, err := artifactcatalog.Load()
	if err != nil {
		return BuildResult{}, err
	}
	identity, err := roster.ResolveTarget(options.Target)
	if err != nil {
		return BuildResult{}, err
	}
	artifact, err := releaseops.Resolve(root, releasecatalog.Target(identity))
	if err != nil {
		return BuildResult{}, err
	}
	if artifact.Kind == releaseops.KindClient {
		return BuildResult{}, fmt.Errorf("客户端不属于正式成品")
	}
	if artifact.Kind == releaseops.KindBox {
		return BuildResult{}, fmt.Errorf("业务端本体已退出正式成品制作，请使用本体打包")
	}
	devBuild := strings.TrimSpace(options.VersionOverride) != ""
	artifactVersion := strings.TrimSpace(options.VersionOverride)
	if artifactVersion == "" {
		artifactVersion = artifact.Version
	} else if err := release.ValidateVersion(artifactVersion); err != nil {
		return BuildResult{}, fmt.Errorf("开发构建版本无效：%w", err)
	}
	if devBuild {
		if err := validateDevelopmentBaseline(artifact.Version, artifactVersion); err != nil {
			return BuildResult{}, err
		}
		if err := releaseops.CheckDevelopment(artifact); err != nil {
			return BuildResult{}, fmt.Errorf("发布物开发构建检查失败：%w", err)
		}
	} else if err := releaseops.Check(artifact); err != nil {
		return BuildResult{}, fmt.Errorf("发布物完整检查失败：%w", err)
	}
	officialSource, err := sources.SourceFor(identity.Kind)
	if err != nil {
		return BuildResult{}, err
	}
	sourceRepository, err := sources.RecordRepository()
	if err != nil {
		return BuildResult{}, err
	}
	sourceState, err := readSourceState(ctx, root, sourceRepository)
	if err != nil {
		return BuildResult{}, err
	}
	tagName, err := releasecatalog.TagName(identity, artifactVersion)
	if err != nil {
		return BuildResult{}, err
	}
	archiveName, err := releasecatalog.ArchiveName(identity, artifactVersion)
	if err != nil {
		return BuildResult{}, err
	}
	workRoot, outputRoot, err := resolveRoots(options.WorkRoot, options.OutputRoot)
	if err != nil {
		return BuildResult{}, err
	}
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		return BuildResult{}, fmt.Errorf("建立制作工作区失败：%w", err)
	}
	workDir, err := os.MkdirTemp(workRoot, "build-")
	if err != nil {
		return BuildResult{}, fmt.Errorf("建立本次制作目录失败：%w", err)
	}
	result := BuildResult{WorkDir: workDir}
	assetRoot := strings.TrimSpace(options.AssetRoot)
	if assetRoot == "" {
		assetRoot = workspace.AssetRoot(root)
	}
	assetRoots, err := releaseasset.PrepareRequired(ctx, releaseasset.PrepareOptions{
		RepositoryRoot: root,
		Artifact:       identity,
		OutputRoot:     filepath.Join(assetRoot, "prepared"),
		CacheRoot:      filepath.Join(assetRoot, "cache"),
		TempRoot:       filepath.Join(assetRoot, "temp"),
	})
	if err != nil {
		return result, err
	}
	externalAssets := make([]types.ReleaseExternalAsset, 0, len(assetRoots))
	assetNames := make([]string, 0, len(assetRoots))
	for name := range assetRoots {
		assetNames = append(assetNames, name)
	}
	sort.Strings(assetNames)
	for _, name := range assetNames {
		asset, inspectErr := releaseasset.Inspect(ctx, assetRoots[name], name)
		if inspectErr != nil {
			return result, inspectErr
		}
		externalAssets = append(externalAssets, asset)
	}
	assembledDir := filepath.Join(workDir, "assembled")
	if err := os.MkdirAll(assembledDir, 0o755); err != nil {
		return result, err
	}
	if err := assemble(ctx, root, workDir, assembledDir, artifact, identity, assetRoots, sourceState.CommitTime, artifactVersion); err != nil {
		return result, err
	}
	externalAssets, err = releaseasset.BindPackagedAssets(assembledDir, identity, externalAssets)
	if err != nil {
		return result, err
	}
	product := types.ReleaseProductRecord{
		SchemaVersion:  release.ReleaseManifestSchemaVersion,
		Artifact:       identity,
		Version:        artifactVersion,
		Platform:       types.ReleasePlatformWindowsX64,
		OfficialSource: officialSource.Repository,
		Compatibility:  cloneCompatibility(artifact.Compatibility),
		Source:         sourceState.Record,
		DataVersion:    artifact.DataVersion,
		ExternalAssets: externalAssets,
	}
	if err := release.ValidateReleaseProductRecord(product); err != nil {
		return result, err
	}
	if err := copyFile(artifact.READMEPath, filepath.Join(assembledDir, "README.md")); err != nil {
		return result, err
	}
	if err := copyFile(artifact.ChangelogPath, filepath.Join(assembledDir, "CHANGELOG.md")); err != nil {
		return result, err
	}
	if err := writeJSON(filepath.Join(assembledDir, "release-product.json"), product); err != nil {
		return result, err
	}
	if err := validatePackageBoundary(assembledDir, identity); err != nil {
		return result, err
	}
	fileRecords, err := recordsForDirectory(assembledDir)
	if err != nil {
		return result, err
	}
	archivePath := filepath.Join(workDir, archiveName)
	if err := createZip(assembledDir, archivePath); err != nil {
		return result, err
	}
	archiveRecord, err := recordForFile(archivePath, archiveName)
	if err != nil {
		return result, err
	}
	manifest := types.ReleaseManifest{
		SchemaVersion:  release.ReleaseManifestSchemaVersion,
		Artifact:       identity,
		Version:        artifactVersion,
		Platform:       types.ReleasePlatformWindowsX64,
		TagName:        tagName,
		OfficialSource: officialSource.Repository,
		Compatibility:  cloneCompatibility(artifact.Compatibility),
		Source:         sourceState.Record,
		DataVersion:    artifact.DataVersion,
		ExternalAssets: externalAssets,
		Archive:        archiveRecord,
		Files:          fileRecords,
	}
	if err := release.ValidateReleaseManifest(manifest); err != nil {
		return result, err
	}
	manifestName := strings.TrimSuffix(archiveName, ".zip") + ".manifest.json"
	manifestPath := filepath.Join(workDir, manifestName)
	if err := writeJSON(manifestPath, manifest); err != nil {
		return result, err
	}
	notes, err := releaseNotes(artifact.ChangelogPath, artifactVersion, devBuild)
	if err != nil {
		return result, err
	}
	notesPath := filepath.Join(workDir, "release-notes.md")
	if err := os.WriteFile(notesPath, []byte(notes), 0o644); err != nil {
		return result, fmt.Errorf("写入发行说明失败：%w", err)
	}
	verification, err := Verify(ctx, VerifyOptions{ArchivePath: archivePath, ManifestPath: manifestPath, Workspace: filepath.Join(workDir, "verification")})
	if err != nil {
		return result, fmt.Errorf("成品验收失败：%w", err)
	}
	if evidenceRoot := strings.TrimSpace(options.EvidenceRoot); evidenceRoot != "" {
		if err := os.RemoveAll(evidenceRoot); err != nil {
			return result, fmt.Errorf("清理成品验收证据失败：%w", err)
		}
		if err := copyDirectory(verification.Evidence, evidenceRoot); err != nil {
			return result, fmt.Errorf("保存成品验收证据失败：%w", err)
		}
	}
	outputDir := filepath.Join(outputRoot, outputDirectoryName(identity), artifactVersion)
	if err := publishOutputDirectory(outputDir, []string{archivePath, manifestPath, notesPath}, devBuild); err != nil {
		return result, err
	}
	result.Manifest = manifest
	result.OutputDir = outputDir
	result.ArchivePath = filepath.Join(outputDir, filepath.Base(archivePath))
	result.ManifestPath = filepath.Join(outputDir, filepath.Base(manifestPath))
	result.NotesPath = filepath.Join(outputDir, filepath.Base(notesPath))
	return result, nil
}

func assemble(ctx context.Context, root string, workDir string, assembledDir string, artifact releaseops.Artifact, identity types.ReleaseArtifactIdentity, assetRoots map[string]string, sourceTime time.Time, artifactVersion string) error {
	switch identity.Kind {
	case types.ReleaseArtifactKindTool:
		return assembleTool(ctx, root, workDir, assembledDir, artifact, assetRoots, sourceTime, artifactVersion)
	case types.ReleaseArtifactKindPlugin:
		packagePath := "./" + filepath.ToSlash(filepath.Join("system-plugins", identity.ID, "cmd", identity.ID))
		if err := buildGoBinary(ctx, root, workDir, packagePath, filepath.Join(assembledDir, "binary", identity.ID+".exe")); err != nil {
			return err
		}
		for _, name := range []string{"manifest.json", "config.json"} {
			if err := copyFile(filepath.Join(artifact.Directory, name), filepath.Join(assembledDir, name)); err != nil {
				return err
			}
		}
		return stampPluginManifestVersion(assembledDir, artifactVersion)
	default:
		return fmt.Errorf("不支持的发布物类别 %q", identity.Kind)
	}
}

func assembleTool(ctx context.Context, root string, workDir string, assembledDir string, artifact releaseops.Artifact, assetRoots map[string]string, sourceTime time.Time, artifactVersion string) error {
	dataDir := filepath.Join(workDir, "tool-build")
	args := []string{"run", "devtools/eucli-toolpack", "-tool", artifact.ID, "-data-dir", dataDir, "-build-time", sourceTime.UTC().Format(time.RFC3339Nano), "-version", artifactVersion}
	keys := make([]string, 0, len(assetRoots))
	for name := range assetRoots {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		if name == "" || assetRoots[name] == "" {
			return fmt.Errorf("外部附带内容必须使用非空的名称和目录")
		}
		args = append(args, "-asset-root", name+"="+assetRoots[name])
	}
	for _, name := range keys {
		args = append(args, "-require-asset-root", name)
	}
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = root
	cmd.Env = buildEnvironment(os.Environ(), filepath.Join(workDir, "go"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("制作工具本体失败：%w\n%s", err, strings.TrimSpace(string(output)))
	}
	bodyDir := filepath.Join(dataDir, "tool-bodies", artifact.ID)
	if err := copyDirectory(bodyDir, assembledDir); err != nil {
		return fmt.Errorf("组装工具本体失败：%w", err)
	}
	return nil
}

// validateDevelopmentBaseline 要求四段开发版本建立在源码正式基线上；
// 三段显式版本保持原有语义，不加基线约束。
func validateDevelopmentBaseline(sourceVersion string, artifactVersion string) error {
	formality, err := release.Formality(artifactVersion)
	if err != nil {
		return fmt.Errorf("开发构建版本无效：%w", err)
	}
	if formality != release.FormalityDevelopment {
		return nil
	}
	if err := release.ValidateDevelopmentVersion(sourceVersion, artifactVersion); err != nil {
		return fmt.Errorf("开发构建版本必须建立在源码正式基线上：%w", err)
	}
	return nil
}

// stampPluginManifestVersion 把成品版本写入插件身份声明，使包内身份与成品版本一致。
func stampPluginManifestVersion(assembledDir string, version string) error {
	path := filepath.Join(assembledDir, "manifest.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取插件身份声明失败：%w", err)
	}
	var manifest types.SystemPluginManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return fmt.Errorf("插件身份声明无效：%w", err)
	}
	manifest.Version = version
	return writeJSON(path, manifest)
}

func buildGoBinary(ctx context.Context, root string, workDir string, packagePath string, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", target, packagePath)
	cmd.Dir = root
	cmd.Env = buildEnvironment(os.Environ(), filepath.Join(workDir, "go"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("编译 %s 失败：%w\n%s", packagePath, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func buildEnvironment(base []string, root string) []string {
	values := map[string]string{
		"CGO_ENABLED": "0",
		"GOOS":        "windows",
		"GOARCH":      "amd64",
		"GOCACHE":     filepath.Join(root, "cache"),
		"GOTMPDIR":    filepath.Join(root, "temp"),
	}
	for _, path := range []string{values["GOCACHE"], values["GOTMPDIR"]} {
		_ = os.MkdirAll(path, 0o755)
	}
	result := make([]string, 0, len(base)+len(values))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, replaced := values[strings.ToUpper(key)]; replaced {
				continue
			}
		}
		result = append(result, item)
	}
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

// publishOutputDirectory 把本次产出的文件原子地放入成品输出目录；
// allowReplace 为假时同版本目录已存在即拒绝，为真时先清空再放入。
func publishOutputDirectory(outputDir string, sources []string, allowReplace bool) error {
	if info, err := os.Stat(outputDir); err == nil {
		if !allowReplace {
			return fmt.Errorf("本地成品目录已经存在，不能覆盖：%s", outputDir)
		}
		if !info.IsDir() {
			return fmt.Errorf("本地成品路径不是目录：%s", outputDir)
		}
		if err := os.RemoveAll(outputDir); err != nil {
			return fmt.Errorf("覆盖旧成品失败：%w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	stagingOutput := outputDir + ".staging"
	if err := os.RemoveAll(stagingOutput); err != nil {
		return err
	}
	if err := os.MkdirAll(stagingOutput, 0o755); err != nil {
		return fmt.Errorf("建立成品输出目录失败：%w", err)
	}
	for _, source := range sources {
		if err := copyFile(source, filepath.Join(stagingOutput, filepath.Base(source))); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputDir), 0o755); err != nil {
		return err
	}
	if err := os.Rename(stagingOutput, outputDir); err != nil {
		return fmt.Errorf("启用本地成品失败：%w", err)
	}
	return nil
}

func resolveRoots(workRootValue string, outputRootValue string) (string, string, error) {
	workRoot := strings.TrimSpace(workRootValue)
	if workRoot == "" {
		return "", "", fmt.Errorf("制作工作根不能为空：必须显式传入本轮工作现场（工具运行区 work\\build-<轮>）")
	}
	outputRoot := strings.TrimSpace(outputRootValue)
	if outputRoot == "" {
		return "", "", fmt.Errorf("成品输出根不能为空：必须显式传入工具运行区 output")
	}
	var err error
	workRoot, err = filepath.Abs(workRoot)
	if err != nil {
		return "", "", err
	}
	outputRoot, err = filepath.Abs(outputRoot)
	if err != nil {
		return "", "", err
	}
	if samePath(workRoot, outputRoot) || pathWithin(workRoot, outputRoot) || pathWithin(outputRoot, workRoot) {
		return "", "", fmt.Errorf("制作工作区和成品输出区必须彼此分开")
	}
	return workRoot, outputRoot, nil
}

func releaseNotes(path string, version string, allowMissing bool) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取更新说明失败：%w", err)
	}
	lines := strings.Split(strings.ReplaceAll(string(payload), "\r\n", "\n"), "\n")
	start := -1
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "## "+version) {
			start = index
			break
		}
	}
	if start < 0 {
		if allowMissing {
			return fmt.Sprintf("开发构建 %s：无独立发行说明。\n", version), nil
		}
		return "", fmt.Errorf("更新记录缺少版本 %s", version)
	}
	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		if strings.HasPrefix(strings.TrimSpace(lines[index]), "## ") {
			end = index
			break
		}
	}
	notes := strings.TrimSpace(strings.Join(lines[start:end], "\n")) + "\n"
	return notes, nil
}

func writeJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("生成 %s 失败：%w", filepath.Base(path), err)
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败：%w", filepath.Base(path), err)
	}
	return nil
}

func cloneCompatibility(value *types.EucliBoxCompatibility) *types.EucliBoxCompatibility {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func outputDirectoryName(identity types.ReleaseArtifactIdentity) string {
	if identity.Kind == types.ReleaseArtifactKindBox {
		return identity.ID
	}
	return identity.Kind + "-" + identity.ID
}

func cleanExistingDirectory(value string, label string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "."
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("确定%s失败：%w", label, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("读取%s失败：%w", label, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s必须是目录", label)
	}
	return filepath.Clean(absolute), nil
}
