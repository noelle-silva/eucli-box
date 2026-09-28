package release

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
)

// 包内核验只有一条最小标准：身份定义文件与当前平台可执行文件必须存在，
// 内容边界必须安全。说明文档、更新记录、成品身份资料、来源记录一律不参与核验。

// ValidatePackageDirectoryOptions 固定一次包内核验的范围。
type ValidatePackageDirectoryOptions struct {
	Directory string
	Kind      string
}

// ValidatedPackage 是已核验的包目录事实：身份与版本来自包内定义文件。
type ValidatedPackage struct {
	Directory string
	Artifact  types.ReleaseArtifactIdentity
	Version   string
	Files     []types.ReleaseFileRecord
}

// ReadToolPackageDefinition 读取工具包内的定义文件。
// 它只做身份与版本的解析，供包核验与版本目录读取共用；业务深度校验属于工具系统。
func ReadToolPackageDefinition(directory string) (types.ToolDefinition, error) {
	payload, err := os.ReadFile(filepath.Join(directory, "definition.json"))
	if err != nil {
		return types.ToolDefinition{}, fmt.Errorf("工具包缺少工具定义文件：%w", err)
	}
	var definition types.ToolDefinition
	if err := json.Unmarshal(payload, &definition); err != nil {
		return types.ToolDefinition{}, fmt.Errorf("工具定义文件无效：%w", err)
	}
	if strings.TrimSpace(definition.ID) == "" {
		return types.ToolDefinition{}, fmt.Errorf("工具定义缺少 ID")
	}
	if err := ValidateVersion(definition.Version); err != nil {
		return types.ToolDefinition{}, fmt.Errorf("工具定义版本无效：%w", err)
	}
	return definition, nil
}

// ReadPluginPackageDefinition 读取插件包内的身份声明。
// 它只做身份与版本的解析，供包核验与版本目录读取共用；协议与托管的校验属于插件系统。
func ReadPluginPackageDefinition(directory string) (types.SystemPluginManifest, error) {
	payload, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return types.SystemPluginManifest{}, fmt.Errorf("插件包缺少身份声明：%w", err)
	}
	var manifest types.SystemPluginManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return types.SystemPluginManifest{}, fmt.Errorf("插件身份声明无效：%w", err)
	}
	if strings.TrimSpace(manifest.ID) == "" {
		return types.SystemPluginManifest{}, fmt.Errorf("插件身份声明缺少 ID")
	}
	if err := ValidateVersion(manifest.Version); err != nil {
		return types.SystemPluginManifest{}, fmt.Errorf("插件身份声明版本无效：%w", err)
	}
	return manifest, nil
}

// PackageIdentity 从包内定义文件读身份与版本。
func PackageIdentity(directory string, kind string) (types.ReleaseArtifactIdentity, string, error) {
	switch strings.TrimSpace(kind) {
	case types.ReleaseArtifactKindTool:
		definition, err := ReadToolPackageDefinition(directory)
		if err != nil {
			return types.ReleaseArtifactIdentity{}, "", err
		}
		return types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: strings.TrimSpace(definition.ID)}, strings.TrimSpace(definition.Version), nil
	case types.ReleaseArtifactKindPlugin:
		manifest, err := ReadPluginPackageDefinition(directory)
		if err != nil {
			return types.ReleaseArtifactIdentity{}, "", err
		}
		return types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: strings.TrimSpace(manifest.ID)}, strings.TrimSpace(manifest.Version), nil
	default:
		return types.ReleaseArtifactIdentity{}, "", fmt.Errorf("未知发布物类别 %q", kind)
	}
}

// ValidatePackageDirectory 执行包内核验的最小标准：
// 身份定义文件合法、当前平台可执行文件真实存在、内容边界安全。
func ValidatePackageDirectory(options ValidatePackageDirectoryOptions) (ValidatedPackage, error) {
	directory, err := existingDirectory(options.Directory)
	if err != nil {
		return ValidatedPackage{}, err
	}
	if err := validatePackageBoundary(directory); err != nil {
		return ValidatedPackage{}, err
	}
	switch strings.TrimSpace(options.Kind) {
	case types.ReleaseArtifactKindTool:
		definition, err := ReadToolPackageDefinition(directory)
		if err != nil {
			return ValidatedPackage{}, err
		}
		if err := requireWindowsExecutable(directory, toolBinaries(definition.Binaries)); err != nil {
			return ValidatedPackage{}, err
		}
	case types.ReleaseArtifactKindPlugin:
		manifest, err := ReadPluginPackageDefinition(directory)
		if err != nil {
			return ValidatedPackage{}, err
		}
		if err := requirePackageFile(directory, "config.json"); err != nil {
			return ValidatedPackage{}, err
		}
		if err := requireWindowsExecutable(directory, pluginBinaries(manifest.Binaries)); err != nil {
			return ValidatedPackage{}, err
		}
	default:
		return ValidatedPackage{}, fmt.Errorf("未知发布物类别 %q", options.Kind)
	}
	artifact, version, err := PackageIdentity(directory, options.Kind)
	if err != nil {
		return ValidatedPackage{}, err
	}
	files, err := CollectFileRecords(directory)
	if err != nil {
		return ValidatedPackage{}, err
	}
	return ValidatedPackage{Directory: directory, Artifact: artifact, Version: version, Files: files}, nil
}

// packageBinary 是包内平台二进制声明的统一形态；工具与插件的声明结构一致。
type packageBinary struct {
	goos   string
	goarch string
	path   string
}

func toolBinaries(binaries []types.ToolBinary) []packageBinary {
	out := make([]packageBinary, 0, len(binaries))
	for _, binary := range binaries {
		out = append(out, packageBinary{goos: binary.GOOS, goarch: binary.GOARCH, path: binary.Path})
	}
	return out
}

func pluginBinaries(binaries []types.SystemPluginBinary) []packageBinary {
	out := make([]packageBinary, 0, len(binaries))
	for _, binary := range binaries {
		out = append(out, packageBinary{goos: binary.GOOS, goarch: binary.GOARCH, path: binary.Path})
	}
	return out
}

// requireWindowsExecutable 要求至少存在一个 Windows x64 可执行文件声明，
// 且声明的文件路径安全并真实存在。
func requireWindowsExecutable(directory string, binaries []packageBinary) error {
	found := false
	for _, binary := range binaries {
		if strings.TrimSpace(binary.goos) != "windows" || strings.TrimSpace(binary.goarch) != "amd64" {
			continue
		}
		if err := requirePackageFile(directory, binary.path); err != nil {
			return err
		}
		found = true
	}
	if !found {
		return fmt.Errorf("包缺少 Windows x64 可执行文件声明")
	}
	return nil
}

// requirePackageFile 核验包内必需文件：路径安全且是真实存在的普通文件。
func requirePackageFile(directory string, name string) error {
	clean, err := safeArchivePath(name)
	if err != nil {
		return fmt.Errorf("必需文件路径无效：%w", err)
	}
	path := filepath.Join(directory, filepath.FromSlash(clean))
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("缺少必需文件 %s：%w", clean, err)
	}
	if info.IsDir() {
		return fmt.Errorf("必需文件 %s 实际是目录", clean)
	}
	return nil
}
