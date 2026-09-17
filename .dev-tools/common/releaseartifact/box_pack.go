package releaseartifact

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devtools/common/releaseops"
	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

// PackBoxOptions 是业务端本体简版打包的输入事实：只给出仓库根与工作/输出区。
// 本体已退出正式发行体系，这里不接收成品验收、外部附带内容或版本覆盖参数。
type PackBoxOptions struct {
	Root       string
	WorkRoot   string
	OutputRoot string
}

// PackBox 把当前源码的业务端本体构建成一个可直接分发的压缩包：
// 编译可执行程序、随包中文文档，打成 ZIP 即完成；不生成成品身份资料、
// 不生成发行清单，也不做正式成品验收。
// 结果复用统一的成品结果结构，Manifest 只承载打包事实（身份、版本、压缩包记录）。
func PackBox(ctx context.Context, options PackBoxOptions) (BuildResult, error) {
	if ctx == nil {
		return BuildResult{}, fmt.Errorf("打包上下文不能为空")
	}
	root, err := cleanExistingDirectory(options.Root, "仓库根目录")
	if err != nil {
		return BuildResult{}, err
	}
	artifact, err := releaseops.Resolve(root, string(releaseops.KindBox))
	if err != nil {
		return BuildResult{}, err
	}
	if err := releaseops.CheckDevelopment(artifact); err != nil {
		return BuildResult{}, fmt.Errorf("本体打包检查失败：%w", err)
	}
	workRoot, outputRoot, err := resolveRoots(options.WorkRoot, options.OutputRoot)
	if err != nil {
		return BuildResult{}, err
	}
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		return BuildResult{}, fmt.Errorf("建立打包工作区失败：%w", err)
	}
	workDir, err := os.MkdirTemp(workRoot, "pack-")
	if err != nil {
		return BuildResult{}, fmt.Errorf("建立本次打包目录失败：%w", err)
	}
	assembledDir := filepath.Join(workDir, "assembled")
	if err := os.MkdirAll(assembledDir, 0o755); err != nil {
		return BuildResult{}, err
	}
	if err := assembleBox(ctx, root, workDir, assembledDir); err != nil {
		return BuildResult{}, err
	}
	if err := copyFile(artifact.READMEPath, filepath.Join(assembledDir, "README.md")); err != nil {
		return BuildResult{}, err
	}
	if err := copyFile(artifact.ChangelogPath, filepath.Join(assembledDir, "CHANGELOG.md")); err != nil {
		return BuildResult{}, err
	}
	if err := validatePackageFiles(assembledDir, nil); err != nil {
		return BuildResult{}, err
	}
	if err := requireRegularFile(assembledDir, "eucli-box.exe"); err != nil {
		return BuildResult{}, err
	}
	archiveName := boxArchiveName(artifact.Version)
	archivePath := filepath.Join(workDir, archiveName)
	if err := createZip(assembledDir, archivePath); err != nil {
		return BuildResult{}, err
	}
	archiveRecord, err := recordForFile(archivePath, archiveName)
	if err != nil {
		return BuildResult{}, err
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox}
	outputDir := filepath.Join(outputRoot, outputDirectoryName(identity), artifact.Version)
	if err := publishOutputDirectory(outputDir, []string{archivePath}, true); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{
		Manifest: types.ReleaseManifest{
			SchemaVersion: release.ReleaseManifestSchemaVersion,
			Artifact:      identity,
			Version:       artifact.Version,
			Platform:      types.ReleasePlatformWindowsX64,
			Archive:       archiveRecord,
		},
		ArchivePath: filepath.Join(outputDir, archiveName),
		OutputDir:   outputDir,
		WorkDir:     workDir,
	}, nil
}

// boxArchiveName 是业务端本体压缩包的固定命名。
func boxArchiveName(version string) string {
	return fmt.Sprintf("%s_%s_%s.zip", types.ReleaseArtifactKindBox, strings.TrimSpace(version), types.ReleasePlatformWindowsX64)
}

// assembleBox 组装业务端本体的压缩包内容：只编译可执行程序。
func assembleBox(ctx context.Context, root string, workDir string, assembledDir string) error {
	return buildGoBinary(ctx, root, workDir, "./cmd/eucli-box", filepath.Join(assembledDir, "eucli-box.exe"))
}
