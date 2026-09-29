package datastorage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
)

// toolConfigDir 返回单个工具配置区目录（<工具数据目录>/config）。
func (s *system) toolConfigDir(toolID string) (string, error) {
	dataDir, err := s.paths.toolDataDir(toolID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, types.ToolConfigDirName), nil
}

// LoadToolConfigFiles 读取工具配置区全部文件：路径 + 原文。
// 宿主只做存取展示，不解释配置语义；目录不存在视为空配置区。
func (s *system) LoadToolConfigFiles(ctx context.Context, toolID string) ([]types.ToolConfigFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, storageReadFailed("read cancelled", err)
	}
	root, err := s.toolConfigDir(toolID)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return []types.ToolConfigFile{}, nil
	}
	if err != nil {
		return nil, storageReadFailed("failed to inspect tool config directory", err)
	}
	if !info.IsDir() {
		return nil, storageInvalid("tool config path is not a directory", nil)
	}
	files := []types.ToolConfigFile{}
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if !entryInfo.Mode().IsRegular() {
			return nil
		}
		if entryInfo.Size() > types.ToolConfigFileMaxBytes {
			return errors.New("tool config file exceeds the size limit: " + entry.Name())
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, types.ToolConfigFile{Path: filepath.ToSlash(rel), Content: string(payload)})
		return nil
	})
	if walkErr != nil {
		return nil, storageReadFailed("failed to read tool config files", walkErr)
	}
	sortToolConfigFiles(files)
	return files, nil
}

// applyToolConfigFileWrites 应用配置区文件写入意图：先整体校验路径与大小，
// 再逐个原子写入或删除；删除不存在的文件视为已达目标状态。
func (s *system) applyToolConfigFileWrites(ctx context.Context, toolID string, writes []types.ToolConfigFileWrite) error {
	if len(writes) == 0 {
		return nil
	}
	root, err := s.toolConfigDir(toolID)
	if err != nil {
		return err
	}
	type preparedWrite struct {
		target  string
		relPath string
		content string
		deleted bool
	}
	prepared := make([]preparedWrite, 0, len(writes))
	for _, write := range writes {
		if err := ctx.Err(); err != nil {
			return storageWriteFailed("write cancelled", err)
		}
		relPath, err := types.CleanToolConfigRelPath(write.Path)
		if err != nil {
			return storageInvalid("tool config file path is invalid", err)
		}
		target := filepath.Join(root, filepath.FromSlash(relPath))
		if !isWithin(root, target) {
			return storageInvalid("tool config file path escapes the config directory", nil)
		}
		if write.Deleted {
			prepared = append(prepared, preparedWrite{target: target, relPath: relPath, deleted: true})
			continue
		}
		if len(write.Content) > types.ToolConfigFileMaxBytes {
			return storageInvalid("tool config file content exceeds the size limit: "+relPath, nil)
		}
		prepared = append(prepared, preparedWrite{target: target, relPath: relPath, content: write.Content})
	}
	for _, item := range prepared {
		if item.deleted {
			if err := os.Remove(item.target); err != nil && !errors.Is(err, os.ErrNotExist) {
				return storageWriteFailed("failed to delete tool config file: "+item.relPath, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(item.target), 0o755); err != nil {
			return storageWriteFailed("failed to create tool config directory", err)
		}
		if err := writeFileAtomic(item.target, []byte(item.content)); err != nil {
			return storageWriteFailed("failed to write tool config file: "+item.relPath, err)
		}
	}
	return nil
}

// writeFileAtomic 以临时文件加原子替换写入文件。
func writeFileAtomic(target string, payload []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".tmp-*.config")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

// sortToolConfigFiles 按路径排序配置区文件，保证读取顺序稳定。
func sortToolConfigFiles(files []types.ToolConfigFile) {
	for i := 1; i < len(files); i++ {
		value := files[i]
		j := i - 1
		for j >= 0 && strings.Compare(files[j].Path, value.Path) > 0 {
			files[j+1] = files[j]
			j--
		}
		files[j+1] = value
	}
}
