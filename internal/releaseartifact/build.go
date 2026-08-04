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

	"eucli-box/internal/releaseasset"
	"eucli-box/internal/releaseops"
	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
	"eucli-box/pkg/workspace"
)

type BuildOptions struct {
	Root             string
	Target           string
	WorkRoot         string
	OutputRoot       string
	EvidenceRoot     string
	VerificationOnly bool
	AssetRoot        string
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
	catalog, err := releasecatalog.Load()
	if err != nil {
		return BuildResult{}, err
	}
	identity, err := catalog.ResolveTarget(options.Target)
	if err != nil {
		return BuildResult{}, err
	}
	artifact, err := releaseops.Resolve(root, releasecatalog.Target(identity))
	if err != nil {
		return BuildResult{}, err
	}
	if artifact.Kind == releaseops.KindClient {
		return BuildResult{}, fmt.Errorf("客户端不属于任务 026 的正式成品")
	}
	if err := releaseops.Check(artifact); err != nil {
		return BuildResult{}, fmt.Errorf("发布物完整检查失败：%w", err)
	}
	officialSource, err := catalog.SourceFor(identity.Kind)
	if err != nil {
		return BuildResult{}, err
	}
	sourceRepository, err := catalog.SourceFor(types.ReleaseArtifactKindBox)
	if err != nil {
		return BuildResult{}, err
	}
	sourceState, err := readSourceState(ctx, root, sourceRepository.Repository, options.VerificationOnly)
	if err != nil {
		return BuildResult{}, err
	}
	tagName, err := releasecatalog.TagName(identity, artifact.Version)
	if err != nil {
		return BuildResult{}, err
	}
	archiveName, err := releasecatalog.ArchiveName(identity, artifact.Version)
	if err != nil {
		return BuildResult{}, err
	}
	workRoot, outputRoot, err := resolveRoots(root, options)
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
	if err := assemble(ctx, root, workDir, assembledDir, artifact, identity, assetRoots, sourceState.CommitTime); err != nil {
		return result, err
	}
	externalAssets, err = releaseasset.BindPackagedAssets(assembledDir, identity, externalAssets)
	if err != nil {
		return result, err
	}
	product := types.ReleaseProductRecord{
		SchemaVersion:    release.ReleaseManifestSchemaVersion,
		Artifact:         identity,
		Version:          artifact.Version,
		Platform:         types.ReleasePlatformWindowsX64,
		OfficialSource:   officialSource.Repository,
		Compatibility:    cloneCompatibility(artifact.Compatibility),
		Source:           sourceState.Record,
		DataVersion:      artifact.DataVersion,
		ExternalAssets:   externalAssets,
		VerificationOnly: options.VerificationOnly,
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
		SchemaVersion:    release.ReleaseManifestSchemaVersion,
		Artifact:         identity,
		Version:          artifact.Version,
		Platform:         types.ReleasePlatformWindowsX64,
		TagName:          tagName,
		OfficialSource:   officialSource.Repository,
		Compatibility:    cloneCompatibility(artifact.Compatibility),
		Source:           sourceState.Record,
		DataVersion:      artifact.DataVersion,
		ExternalAssets:   externalAssets,
		VerificationOnly: options.VerificationOnly,
		Archive:          archiveRecord,
		Files:            fileRecords,
	}
	if err := release.ValidateReleaseManifest(manifest); err != nil {
		return result, err
	}
	manifestName := strings.TrimSuffix(archiveName, ".zip") + ".manifest.json"
	manifestPath := filepath.Join(workDir, manifestName)
	if err := writeJSON(manifestPath, manifest); err != nil {
		return result, err
	}
	notes, err := releaseNotes(artifact.ChangelogPath, artifact.Version)
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
	outputDir := filepath.Join(outputRoot, outputDirectoryName(identity), artifact.Version)
	if _, err := os.Stat(outputDir); err == nil {
		return result, fmt.Errorf("本地成品目录已经存在，不能覆盖：%s", outputDir)
	} else if !os.IsNotExist(err) {
		return result, err
	}
	stagingOutput := outputDir + ".staging"
	if err := os.RemoveAll(stagingOutput); err != nil {
		return result, err
	}
	if err := os.MkdirAll(stagingOutput, 0o755); err != nil {
		return result, fmt.Errorf("建立成品输出目录失败：%w", err)
	}
	for _, source := range []string{archivePath, manifestPath, notesPath} {
		if err := copyFile(source, filepath.Join(stagingOutput, filepath.Base(source))); err != nil {
			return result, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputDir), 0o755); err != nil {
		return result, err
	}
	if err := os.Rename(stagingOutput, outputDir); err != nil {
		return result, fmt.Errorf("启用本地正式成品失败：%w", err)
	}
	result.Manifest = manifest
	result.OutputDir = outputDir
	result.ArchivePath = filepath.Join(outputDir, filepath.Base(archivePath))
	result.ManifestPath = filepath.Join(outputDir, filepath.Base(manifestPath))
	result.NotesPath = filepath.Join(outputDir, filepath.Base(notesPath))
	return result, nil
}

func assemble(ctx context.Context, root string, workDir string, assembledDir string, artifact releaseops.Artifact, identity types.ReleaseArtifactIdentity, assetRoots map[string]string, sourceTime time.Time) error {
	switch identity.Kind {
	case types.ReleaseArtifactKindBox:
		return buildGoBinary(ctx, root, workDir, "./cmd/eucli-box", filepath.Join(assembledDir, "eucli-box.exe"))
	case types.ReleaseArtifactKindTool:
		return assembleTool(ctx, root, workDir, assembledDir, artifact, assetRoots, sourceTime)
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
		return nil
	default:
		return fmt.Errorf("不支持的发布物类别 %q", identity.Kind)
	}
}

func assembleTool(ctx context.Context, root string, workDir string, assembledDir string, artifact releaseops.Artifact, assetRoots map[string]string, sourceTime time.Time) error {
	dataDir := filepath.Join(workDir, "tool-build")
	args := []string{"run", "./cmd/eucli-toolpack", "-tool", artifact.ID, "-data-dir", dataDir, "-build-time", sourceTime.UTC().Format(time.RFC3339Nano)}
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

func resolveRoots(root string, options BuildOptions) (string, string, error) {
	workRoot := strings.TrimSpace(options.WorkRoot)
	if workRoot == "" {
		workRoot = workspace.WorkRoot(root)
	}
	outputRoot := strings.TrimSpace(options.OutputRoot)
	if outputRoot == "" {
		outputRoot = workspace.OutputRoot(root)
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

func releaseNotes(path string, version string) (string, error) {
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
