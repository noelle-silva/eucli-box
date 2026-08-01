package releaseverify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func directorySnapshot(root string) (string, error) {
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

func gitSnapshot(root string) (string, error) {
	cmd := exec.Command("git", "status", "--porcelain=v1", "--untracked-files=all")
	cmd.Dir = root
	payload, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("读取源码状态失败：%w：%s", err, strings.TrimSpace(string(payload)))
	}
	return strings.ReplaceAll(string(payload), "\r\n", "\n"), nil
}

func compareSnapshots(label string, before string, after string) error {
	if before != after {
		return fmt.Errorf("%s在验证期间发生改变", label)
	}
	return nil
}
