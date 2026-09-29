package aiimage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

type configFileEntry struct {
	Path  string
	Bytes int64
}

// configRoot 返回工具配置区目录（<工具数据目录>/config）。
func configRoot(input types.ToolExecutionInput) (string, error) {
	dataDir := strings.TrimSpace(input.ToolDataDirectory)
	if dataDir == "" {
		return "", errors.New("工具数据目录不可用")
	}
	return filepath.Join(dataDir, types.ToolConfigDirName), nil
}

// resolveConfigPath 解析配置区内的相对文件路径；路径必须落在配置区内。
func resolveConfigPath(root string, relPath string) (string, string, error) {
	cleaned, err := types.CleanToolConfigRelPath(relPath)
	if err != nil {
		return "", "", err
	}
	target := filepath.Join(root, filepath.FromSlash(cleaned))
	if !pathWithin(root, target) {
		return "", "", errors.New("配置文件路径越出配置区")
	}
	return target, cleaned, nil
}

func pathWithin(base string, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// listConfigFiles 递归列出配置区内的全部常规文件；目录不存在视为空。
func listConfigFiles(root string) ([]configFileEntry, error) {
	entries := []configFileEntry{}
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取配置区失败: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("配置区路径不是目录")
	}
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entries = append(entries, configFileEntry{Path: filepath.ToSlash(rel), Bytes: info.Size()})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("遍历配置区失败: %w", walkErr)
	}
	sort.Slice(entries, func(i int, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// readConfigFile 读取配置区内文件原文；限定单文件大小上限。
func readConfigFile(root string, relPath string) (string, string, error) {
	target, cleaned, err := resolveConfigPath(root, relPath)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", cleaned, fmt.Errorf("配置文件不存在: %s", cleaned)
		}
		return "", cleaned, fmt.Errorf("读取配置文件失败: %w", err)
	}
	if info.IsDir() {
		return "", cleaned, fmt.Errorf("配置文件路径是目录: %s", cleaned)
	}
	if info.Size() > types.ToolConfigFileMaxBytes {
		return "", cleaned, fmt.Errorf("配置文件超过大小上限（%d 字节）: %s", types.ToolConfigFileMaxBytes, cleaned)
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		return "", cleaned, fmt.Errorf("读取配置文件失败: %w", err)
	}
	return string(payload), cleaned, nil
}

// writeConfigFile 原子写入配置区内文件；路径与大小先校验。
func writeConfigFile(root string, relPath string, content string) (string, error) {
	target, cleaned, err := resolveConfigPath(root, relPath)
	if err != nil {
		return "", err
	}
	if len(content) > types.ToolConfigFileMaxBytes {
		return "", fmt.Errorf("配置内容超过大小上限（%d 字节）", types.ToolConfigFileMaxBytes)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("创建配置目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".tmp-*.json")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("同步临时文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("关闭临时文件失败: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return "", fmt.Errorf("替换配置文件失败: %w", err)
	}
	return cleaned, nil
}

// deleteConfigFile 删除配置区内文件。
func deleteConfigFile(root string, relPath string) (string, error) {
	target, cleaned, err := resolveConfigPath(root, relPath)
	if err != nil {
		return "", err
	}
	if err := os.Remove(target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cleaned, fmt.Errorf("配置文件不存在: %s", cleaned)
		}
		return cleaned, fmt.Errorf("删除配置文件失败: %w", err)
	}
	return cleaned, nil
}
