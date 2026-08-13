package toolkit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirectorySnapshot 返回目录的完整内容快照摘要；目录不存在返回 "absent"。
// 遇到符号链接或重解析点立即失败。
func DirectorySnapshot(root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(absolute); os.IsNotExist(err) {
		return "absent", nil
	} else if err != nil {
		return "", err
	}
	records := make([]string, 0)
	err = filepath.WalkDir(absolute, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("快照目录包含符号链接：%s", path)
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(absolute, path)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		digest := sha256.New()
		size, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		records = append(records, fmt.Sprintf("%s\x00%d\x00%s\n", filepath.ToSlash(relative), size, hex.EncodeToString(digest.Sum(nil))))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(records)
	digest := sha256.Sum256([]byte(strings.Join(records, "")))
	return hex.EncodeToString(digest[:]), nil
}

// CompareSnapshots 比较两份快照，不同则返回错误。
func CompareSnapshots(label string, before string, after string) error {
	if before != after {
		return fmt.Errorf("%s在验证期间发生改变", label)
	}
	return nil
}
