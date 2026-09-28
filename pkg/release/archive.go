package release

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"eucli-box/pkg/types"
	"eucli-box/pkg/workspace"
)

type ExtractArchiveOptions struct {
	ArchivePath string
	TargetDir   string
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
