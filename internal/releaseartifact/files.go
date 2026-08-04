package releaseartifact

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/internal/releaseasset"
	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
	"eucli-box/pkg/workspace"
)

func copyFile(source string, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("读取文件 %s 失败：%w", source, err)
	}
	if info.IsDir() {
		return fmt.Errorf("文件 %s 实际是目录", source)
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("打开文件 %s 失败：%w", source, err)
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("建立文件目录失败：%w", err)
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("创建文件 %s 失败：%w", target, err)
	}
	if _, copyErr := io.Copy(output, input); copyErr != nil {
		_ = output.Close()
		return fmt.Errorf("复制文件失败：%w", copyErr)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("关闭文件 %s 失败：%w", target, err)
	}
	return nil
}

func copyDirectory(source string, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("读取目录 %s 失败：%w", source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("目录 %s 实际不是目录", source)
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("不允许把符号链接放入正式成品：%s", path)
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(target, 0o755)
		}
		targetPath := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		return copyFile(path, targetPath)
	})
}

func recordsForDirectory(root string) ([]types.ReleaseFileRecord, error) {
	root, err := filepath.Abs(root)
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
		record, err := recordForFile(path, filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return release.SortedFileRecords(records), nil
}

func recordForFile(path string, name string) (types.ReleaseFileRecord, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return types.ReleaseFileRecord{}, fmt.Errorf("读取 %s 失败：%w", path, err)
	}
	digest := sha256.Sum256(payload)
	return types.ReleaseFileRecord{Name: filepath.ToSlash(name), Size: int64(len(payload)), SHA256: hex.EncodeToString(digest[:])}, nil
}

func createZip(sourceDir string, target string) error {
	files, err := recordsForDirectory(sourceDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("创建压缩包失败：%w", err)
	}
	archive := zip.NewWriter(output)
	for _, file := range files {
		name := filepath.ToSlash(file.Name)
		if !safeArchivePath(name) {
			_ = archive.Close()
			_ = output.Close()
			return fmt.Errorf("压缩包文件路径无效：%s", name)
		}
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o644)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			_ = archive.Close()
			_ = output.Close()
			return fmt.Errorf("创建压缩包条目失败：%w", err)
		}
		payload, err := os.ReadFile(filepath.Join(sourceDir, filepath.FromSlash(name)))
		if err != nil {
			_ = archive.Close()
			_ = output.Close()
			return err
		}
		if _, err := writer.Write(payload); err != nil {
			_ = archive.Close()
			_ = output.Close()
			return fmt.Errorf("写入压缩包条目失败：%w", err)
		}
	}
	if err := archive.Close(); err != nil {
		_ = output.Close()
		return fmt.Errorf("关闭压缩包失败：%w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("关闭压缩包文件失败：%w", err)
	}
	return nil
}

func validatePackageBoundary(directory string, identity types.ReleaseArtifactIdentity) error {
	files, err := recordsForDirectory(directory)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("成品目录为空")
	}
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		name := filepath.ToSlash(file.Name)
		if !safeArchivePath(name) {
			return fmt.Errorf("成品包含越界路径：%s", name)
		}
		seen[name] = struct{}{}
	}
	for _, required := range []string{"README.md", "CHANGELOG.md", "release-product.json"} {
		if _, ok := seen[required]; !ok {
			return fmt.Errorf("成品缺少 %s", required)
		}
	}
	payload, err := os.ReadFile(filepath.Join(directory, "release-product.json"))
	if err != nil {
		return err
	}
	product, err := release.DecodeReleaseProductRecord(payload)
	if err != nil {
		return err
	}
	if product.Artifact != identity {
		return fmt.Errorf("成品身份与目录目标不一致")
	}
	externalAssetRoots, err := releaseasset.ValidatePackagedAssets(directory, identity, product.ExternalAssets)
	if err != nil {
		return err
	}
	for _, root := range externalAssetRoots {
		if forbiddenPackagePath(root, nil) {
			return fmt.Errorf("成品外部附带内容位于禁止目录：%s", root)
		}
	}
	for _, file := range files {
		name := filepath.ToSlash(file.Name)
		if forbiddenPackagePath(name, externalAssetRoots) {
			return fmt.Errorf("成品包含禁止内容：%s", name)
		}
	}
	switch identity.Kind {
	case types.ReleaseArtifactKindBox:
		if err := requireRegularFile(directory, "eucli-box.exe"); err != nil {
			return err
		}
	case types.ReleaseArtifactKindTool:
		if err := validateToolPackage(directory, identity.ID); err != nil {
			return err
		}
	case types.ReleaseArtifactKindPlugin:
		if err := validatePluginPackage(directory, identity.ID); err != nil {
			return err
		}
	default:
		return fmt.Errorf("未知成品类别 %q", identity.Kind)
	}
	return nil
}

func validateToolPackage(directory string, id string) error {
	if err := requireRegularFile(directory, "definition.json"); err != nil {
		return err
	}
	var definition types.ToolDefinition
	payload, err := os.ReadFile(filepath.Join(directory, "definition.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(payload, &definition); err != nil {
		return fmt.Errorf("工具 definition.json 无效：%w", err)
	}
	if definition.ID != id || definition.BodyDirectory != "." || definition.DataDirectory != "" || len(definition.UserConfig) != 0 || definition.PromptDescriptionOverride != "" {
		return fmt.Errorf("工具成品混入用户资料或运行期路径")
	}
	if len(definition.Binaries) == 0 {
		return fmt.Errorf("工具成品缺少可执行文件声明")
	}
	for _, binary := range definition.Binaries {
		if binary.GOOS != "windows" || binary.GOARCH != "amd64" || !safeArchivePath(binary.Path) {
			continue
		}
		if err := requireRegularFile(directory, binary.Path); err != nil {
			return err
		}
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
	if !safeArchivePath(filepath.ToSlash(name)) {
		return fmt.Errorf("文件路径无效：%s", name)
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		return fmt.Errorf("缺少必需文件 %s：%w", name, err)
	}
	if info.IsDir() {
		return fmt.Errorf("必需文件 %s 实际是目录", name)
	}
	return nil
}

func forbiddenPackagePath(name string, externalAssetRoots []string) bool {
	parts := strings.Split(strings.ToLower(filepath.ToSlash(name)), "/")
	for _, part := range parts {
		switch part {
		case ".git", workspace.WorkspaceDirectory:
			return true
		}
		if strings.HasPrefix(part, ".env") || strings.HasSuffix(part, ".key") {
			return true
		}
	}
	if pathInsideExternalAsset(name, externalAssetRoots) {
		for _, part := range parts {
			switch part {
			case "cache", "sessions", "settings", "secrets", "credentials":
				return true
			}
		}
		return false
	}
	for _, part := range parts {
		switch part {
		case "data", "cache", "sessions", "settings", "secrets", "credentials":
			return true
		}
	}
	return false
}

func pathInsideExternalAsset(name string, roots []string) bool {
	name = strings.ToLower(filepath.ToSlash(strings.TrimSpace(name)))
	for _, root := range roots {
		root = strings.ToLower(filepath.ToSlash(strings.TrimSpace(root)))
		if root != "" && (name == root || strings.HasPrefix(name, root+"/")) {
			return true
		}
	}
	return false
}

func safeArchivePath(name string) bool {
	name = filepath.ToSlash(strings.TrimSpace(name))
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func pathWithin(base string, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func samePath(left string, right string) bool {
	left, _ = filepath.Abs(left)
	right, _ = filepath.Abs(right)
	return filepath.Clean(left) == filepath.Clean(right)
}

func readZipToDirectory(archivePath string, target string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("打开压缩包失败：%w", err)
	}
	defer archive.Close()
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, file := range archive.File {
		name := filepath.ToSlash(strings.TrimSpace(file.Name))
		if !safeArchivePath(name) {
			return fmt.Errorf("压缩包包含越界路径：%s", file.Name)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("压缩包包含重复路径：%s", name)
		}
		seen[name] = struct{}{}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filepath.Join(target, filepath.FromSlash(name)), 0o755); err != nil {
				return err
			}
			continue
		}
		path := filepath.Join(target, filepath.FromSlash(name))
		if !pathWithin(target, path) {
			return fmt.Errorf("压缩包路径越过验证目录：%s", name)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		_ = input.Close()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
