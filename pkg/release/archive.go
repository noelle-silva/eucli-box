package release

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"eucli-box/pkg/types"
	"eucli-box/pkg/workspace"
)

type ExtractArchiveOptions struct {
	ArchivePath string
	TargetDir   string
}

type ValidateExtractedPackageOptions struct {
	Directory string
	Product   types.ReleaseProductRecord
}

type ValidatedPackage struct {
	Directory string
	Product   types.ReleaseProductRecord
	Files     []types.ReleaseFileRecord
}

func ExtractArchive(options ExtractArchiveOptions) error {
	archivePath, err := absoluteRegularFile(options.ArchivePath, "压缩包")
	if err != nil {
		return err
	}
	if strings.TrimSpace(options.TargetDir) == "" {
		return fmt.Errorf("解包目标目录不能为空")
	}
	target, err := filepath.Abs(options.TargetDir)
	if err != nil {
		return fmt.Errorf("解包目标目录无效：%w", err)
	}
	if err := EnsureEmptyDirectory(target); err != nil {
		return err
	}

	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("打开压缩包失败：%w", err)
	}
	defer archive.Close()
	seen := map[string]struct{}{}
	for _, entry := range archive.File {
		name, err := safeArchivePath(entry.Name)
		if err != nil {
			return err
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("压缩包包含重复路径：%s", name)
		}
		seen[key] = struct{}{}
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("压缩包不能包含符号链接：%s", name)
		}
		destination := filepath.Join(target, filepath.FromSlash(name))
		if !pathWithin(target, destination) {
			return fmt.Errorf("压缩包路径越过解包目录：%s", name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return fmt.Errorf("建立解包目录 %s 失败：%w", name, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("建立解包文件目录失败：%w", err)
		}
		input, err := entry.Open()
		if err != nil {
			return fmt.Errorf("打开压缩包文件 %s 失败：%w", name, err)
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			_ = input.Close()
			return fmt.Errorf("创建解包文件 %s 失败：%w", name, err)
		}
		_, copyErr := io.Copy(output, input)
		inputCloseErr := input.Close()
		outputCloseErr := output.Close()
		if copyErr != nil {
			return fmt.Errorf("解包文件 %s 失败：%w", name, copyErr)
		}
		if inputCloseErr != nil || outputCloseErr != nil {
			return fmt.Errorf("关闭解包文件 %s 失败", name)
		}
	}
	return nil
}

func CompareFileRecords(root string, expected []types.ReleaseFileRecord) ([]types.ReleaseFileRecord, error) {
	actual, err := CollectFileRecords(root)
	if err != nil {
		return nil, err
	}
	expected = SortedFileRecords(expected)
	if len(actual) != len(expected) {
		return actual, fmt.Errorf("包内文件数量与发行清单不一致")
	}
	for index := range actual {
		if actual[index] != expected[index] {
			return actual, fmt.Errorf("文件完整性与发行清单不一致：%s", expected[index].Name)
		}
	}
	return actual, nil
}

func ValidateExtractedPackage(options ValidateExtractedPackageOptions) (ValidatedPackage, error) {
	directory, err := existingDirectory(options.Directory)
	if err != nil {
		return ValidatedPackage{}, err
	}
	if err := ValidateReleaseProductRecord(options.Product); err != nil {
		return ValidatedPackage{}, err
	}
	if err := validatePackageBoundary(directory); err != nil {
		return ValidatedPackage{}, err
	}
	productPayload, err := os.ReadFile(filepath.Join(directory, "release-product.json"))
	if err != nil {
		return ValidatedPackage{}, fmt.Errorf("读取成品身份资料失败：%w", err)
	}
	product, err := DecodeReleaseProductRecord(productPayload)
	if err != nil {
		return ValidatedPackage{}, err
	}
	if err := validateExpectedProduct(options.Product, product); err != nil {
		return ValidatedPackage{}, err
	}
	if err := validateExtractedExternalAssets(directory, product.ExternalAssets); err != nil {
		return ValidatedPackage{}, err
	}
	if err := validateRequiredPackageFiles(directory, options.Product.Artifact); err != nil {
		return ValidatedPackage{}, err
	}
	files, err := CollectFileRecords(directory)
	if err != nil {
		return ValidatedPackage{}, err
	}
	return ValidatedPackage{Directory: directory, Product: product, Files: files}, nil
}

// validateExpectedProduct 核对包内身份资料与官方候选期望完全一致；
// 外部附带内容以包内身份资料自身声明为准并单独自洽核对。
func validateExpectedProduct(expected types.ReleaseProductRecord, actual types.ReleaseProductRecord) error {
	if actual.Artifact != expected.Artifact {
		return fmt.Errorf("包内身份资料与官方候选不一致")
	}
	if actual.Version != expected.Version || actual.Platform != expected.Platform || actual.OfficialSource != expected.OfficialSource {
		return fmt.Errorf("包内身份资料与官方候选不一致")
	}
	if !reflect.DeepEqual(actual.Compatibility, expected.Compatibility) {
		return fmt.Errorf("包内适用范围与官方候选不一致")
	}
	if actual.DataVersion != expected.DataVersion {
		return fmt.Errorf("包内数据版本与官方候选不一致")
	}
	if actual.Source.Commit != expected.Source.Commit || actual.Source.Repository != expected.Source.Repository {
		return fmt.Errorf("包内来源记录与官方候选不一致")
	}
	if !expected.VerificationOnly && !actual.Source.Recorded {
		return fmt.Errorf("正式成品来源必须已经完整记录")
	}
	if actual.VerificationOnly != expected.VerificationOnly {
		return fmt.Errorf("包内验证标记与官方候选不一致")
	}
	return nil
}

func CollectFileRecords(root string) ([]types.ReleaseFileRecord, error) {
	return collectFileRecords(root, "")
}

func collectFileRecords(root string, excluded string) ([]types.ReleaseFileRecord, error) {
	root, err := existingDirectory(root)
	if err != nil {
		return nil, err
	}
	records := make([]types.ReleaseFileRecord, 0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("成品目录不能包含符号链接：%s", path)
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if name == excluded {
			return nil
		}
		record, err := fileRecord(path, name)
		if err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return SortedFileRecords(records), nil
}

func validatePackageBoundary(directory string) error {
	files, err := CollectFileRecords(directory)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("成品目录为空")
	}
	externalRoots, err := discoverExternalRoots(directory)
	if err != nil {
		return err
	}
	for _, file := range files {
		name := filepath.ToSlash(file.Name)
		if _, err := safeArchivePath(name); err != nil {
			return fmt.Errorf("成品包含越界路径：%s", name)
		}
		if forbiddenPackagePath(name, externalRoots) {
			return fmt.Errorf("成品包含禁止内容：%s", name)
		}
	}
	return nil
}

func validateRequiredPackageFiles(directory string, identity types.ReleaseArtifactIdentity) error {
	for _, name := range []string{"README.md", "CHANGELOG.md", "release-product.json"} {
		if err := requireRegularFile(directory, name); err != nil {
			return err
		}
	}
	switch identity.Kind {
	case types.ReleaseArtifactKindBox:
		return requireRegularFile(directory, "eucli-box.exe")
	case types.ReleaseArtifactKindTool:
		return validateToolPackage(directory, identity.ID)
	case types.ReleaseArtifactKindPlugin:
		return validatePluginPackage(directory, identity.ID)
	default:
		return fmt.Errorf("未知成品类别 %q", identity.Kind)
	}
}

func validateToolPackage(directory string, id string) error {
	if err := requireRegularFile(directory, "definition.json"); err != nil {
		return err
	}
	payload, err := os.ReadFile(filepath.Join(directory, "definition.json"))
	if err != nil {
		return err
	}
	var definition types.ToolDefinition
	if err := json.Unmarshal(payload, &definition); err != nil {
		return fmt.Errorf("工具 definition.json 无效：%w", err)
	}
	if definition.ID != id || definition.BodyDirectory != "." || definition.DataDirectory != "" || len(definition.UserConfig) != 0 || definition.PromptDescriptionOverride != "" {
		return fmt.Errorf("工具成品混入用户资料或运行期路径")
	}
	if len(definition.Binaries) == 0 {
		return fmt.Errorf("工具成品缺少可执行文件声明")
	}
	foundWindowsBinary := false
	for _, binary := range definition.Binaries {
		if binary.GOOS != "windows" || binary.GOARCH != "amd64" {
			continue
		}
		foundWindowsBinary = true
		if err := requireRegularFile(directory, binary.Path); err != nil {
			return err
		}
	}
	if !foundWindowsBinary {
		return fmt.Errorf("工具成品缺少 Windows x64 可执行文件")
	}
	return nil
}

func validatePluginPackage(directory string, id string) error {
	for _, name := range []string{"manifest.json", "config.json", filepath.ToSlash(filepath.Join("binary", id+".exe"))} {
		if err := requireRegularFile(directory, name); err != nil {
			return err
		}
	}
	payload, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest types.SystemPluginManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return fmt.Errorf("插件 manifest.json 无效：%w", err)
	}
	if manifest.ID != id {
		return fmt.Errorf("插件身份与目录目标不一致")
	}
	return nil
}

func requireRegularFile(root string, name string) error {
	if _, err := safeArchivePath(name); err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(name))
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("缺少必需文件 %s：%w", name, err)
	}
	if info.IsDir() {
		return fmt.Errorf("必需文件 %s 实际是目录", name)
	}
	return nil
}

type externalManifest struct {
	SchemaVersion int                       `json:"schemaVersion"`
	Name          string                    `json:"name"`
	Version       string                    `json:"version"`
	Platform      string                    `json:"platform"`
	Source        string                    `json:"source"`
	Sources       []string                  `json:"sources"`
	Inputs        []map[string]any          `json:"inputs,omitempty"`
	Files         []types.ReleaseFileRecord `json:"files"`
	TreeSHA256    string                    `json:"treeSha256"`
}

func validateExtractedExternalAssets(directory string, declared []types.ReleaseExternalAsset) error {
	declaredPaths := map[string]types.ReleaseExternalAsset{}
	for _, asset := range declared {
		if err := validateExtractedPackagePath(asset.PackagePath); err != nil {
			return fmt.Errorf("外部附带内容 %s 的位置无效：%w", asset.Name, err)
		}
		if _, exists := declaredPaths[asset.PackagePath]; exists {
			return fmt.Errorf("外部附带内容位置重复：%s", asset.PackagePath)
		}
		declaredPaths[asset.PackagePath] = asset
		root := filepath.Join(directory, filepath.FromSlash(asset.PackagePath))
		manifestPath := filepath.Join(root, "vendor-manifest.json")
		payload, err := os.ReadFile(manifestPath)
		if err != nil {
			return fmt.Errorf("外部附带内容 %s 缺少身份资料：%w", asset.Name, err)
		}
		var manifest externalManifest
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&manifest); err != nil {
			return fmt.Errorf("外部附带内容 %s 身份资料无效：%w", asset.Name, err)
		}
		if manifest.SchemaVersion != 1 || manifest.Name != asset.Name || manifest.Version != asset.Version || manifest.Platform != types.ReleasePlatformWindowsX64 || manifest.Source != asset.Source || len(manifest.Files) != asset.FileCount || !strings.EqualFold(manifest.TreeSHA256, asset.TreeSHA256) {
			return fmt.Errorf("外部附带内容 %s 身份资料不一致", asset.Name)
		}
		actual, err := collectFileRecords(root, "vendor-manifest.json")
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(actual, SortedFileRecords(manifest.Files)) || treeSHA256(actual) != strings.ToLower(manifest.TreeSHA256) {
			return fmt.Errorf("外部附带内容 %s 逐文件摘要不一致", asset.Name)
		}
	}
	discovered, err := discoverExternalRoots(directory)
	if err != nil {
		return err
	}
	if len(discovered) != len(declaredPaths) {
		return fmt.Errorf("成品外部附带内容数量与身份资料不一致")
	}
	for _, root := range discovered {
		if _, ok := declaredPaths[root]; !ok {
			return fmt.Errorf("成品包含未声明的外部附带内容：%s", root)
		}
	}
	return nil
}

func discoverExternalRoots(directory string) ([]string, error) {
	roots := make([]string, 0)
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("成品外部附带内容不能包含符号链接：%s", path)
		}
		if entry.IsDir() || entry.Name() != "vendor-manifest.json" {
			return nil
		}
		parent := filepath.Dir(path)
		relative, err := filepath.Rel(directory, parent)
		if err != nil {
			return err
		}
		name, err := safeArchivePath(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		roots = append(roots, name)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(roots)
	return roots, nil
}

func forbiddenPackagePath(name string, externalRoots []string) bool {
	parts := strings.Split(strings.ToLower(filepath.ToSlash(name)), "/")
	for _, part := range parts {
		switch {
		case part == ".git", part == workspace.WorkspaceDirectory:
			return true
		case strings.HasPrefix(part, ".env"), strings.HasSuffix(part, ".key"):
			return true
		}
	}
	insideExternal := false
	for _, root := range externalRoots {
		root = strings.ToLower(filepath.ToSlash(root))
		if strings.HasPrefix(strings.ToLower(filepath.ToSlash(name)), root+"/") {
			insideExternal = true
			break
		}
	}
	for _, part := range parts {
		switch part {
		case "data", "cache", "sessions", "settings", "secrets", "credentials":
			if insideExternal {
				if part == "data" {
					continue
				}
				return true
			}
			return true
		}
	}
	return false
}

func treeSHA256(files []types.ReleaseFileRecord) string {
	digest := sha256.New()
	for _, file := range SortedFileRecords(files) {
		_, _ = fmt.Fprintf(digest, "%s\x00%d\x00%s\n", file.Name, file.Size, strings.ToLower(file.SHA256))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func fileRecord(path string, name string) (types.ReleaseFileRecord, error) {
	input, err := os.Open(path)
	if err != nil {
		return types.ReleaseFileRecord{}, err
	}
	defer input.Close()
	digest := sha256.New()
	size, err := io.Copy(digest, input)
	if err != nil {
		return types.ReleaseFileRecord{}, err
	}
	return types.ReleaseFileRecord{Name: filepath.ToSlash(name), Size: size, SHA256: hex.EncodeToString(digest.Sum(nil))}, nil
}

// RecordForFile 返回单个文件的名称、大小和 SHA-256。
func RecordForFile(path string) (int64, string, error) {
	path, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil || strings.TrimSpace(path) == "" {
		return 0, "", fmt.Errorf("文件路径无效")
	}
	record, err := fileRecord(path, filepath.Base(path))
	if err != nil {
		return 0, "", err
	}
	return record.Size, record.SHA256, nil
}

func safeArchivePath(value string) (string, error) {
	name := filepath.ToSlash(strings.TrimSpace(value))
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || filepath.VolumeName(name) != "" {
		return "", fmt.Errorf("压缩包路径无效：%s", value)
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("压缩包路径无效：%s", value)
		}
	}
	return name, nil
}

func validateExtractedPackagePath(value string) error {
	_, err := safeArchivePath(value)
	return err
}

func existingDirectory(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("目录不能为空")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	if err := EnsurePlainDirectory(absolute); err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("路径必须是目录")
	}
	return filepath.Clean(absolute), nil
}

// EnsureEmptyDirectory 建立（若不存在）或核对一个空的普通目录；路径链不得包含重解析点。
func EnsureEmptyDirectory(path string) error {
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("目录路径实际是普通文件或未知类型：%s", path)
		}
	case os.IsNotExist(err):
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("建立目录失败：%w", err)
		}
	default:
		return fmt.Errorf("读取目录失败：%w", err)
	}
	if err := EnsurePlainDirectory(path); err != nil {
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("读取目录内容失败：%w", err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("目录必须为空：%s", path)
	}
	return nil
}

// EnsurePlainDirectory 核对已存在目录的整条路径链不包含符号链接或重解析点。
func EnsurePlainDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("路径不是普通目录：%s", path)
	}
	hasReparse, err := pathChainHasReparsePoint(path)
	if err != nil {
		return err
	}
	if hasReparse {
		return fmt.Errorf("目录路径包含符号链接或重解析点：%s", path)
	}
	return nil
}

// pathChainHasReparsePoint 检查从已存在路径向上到根之间是否存在重解析点。
func pathChainHasReparsePoint(path string) (bool, error) {
	current := filepath.Clean(path)
	for {
		reparse, err := isReparsePoint(current)
		if err != nil {
			if os.IsNotExist(err) {
				parent := filepath.Dir(current)
				if parent == current {
					return false, nil
				}
				current = parent
				continue
			}
			return false, err
		}
		if reparse {
			return true, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false, nil
		}
		current = parent
	}
}

func absoluteRegularFile(value string, label string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s路径不能为空", label)
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("读取%s失败：%w", label, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s不能是目录", label)
	}
	reparse, err := isReparsePoint(absolute)
	if err != nil {
		return "", fmt.Errorf("读取%s失败：%w", label, err)
	}
	if reparse {
		return "", fmt.Errorf("%s不能是符号链接或重解析点", label)
	}
	return filepath.Clean(absolute), nil
}

func pathWithin(base string, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
