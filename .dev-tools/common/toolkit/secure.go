package toolkit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExistingPlainDirectory 校验目录存在且整条路径都是普通目录（无联接点/符号链接）。
func ExistingPlainDirectory(value string, label string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(value))
	if err != nil || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s无效", label)
	}
	absolute = filepath.Clean(absolute)
	if err := AssertPlainDirectoryChain(absolute, label); err != nil {
		return "", err
	}
	return absolute, nil
}

// EnsurePlainDirectoryPath 只沿真实目录建立路径。
// 每一级都会重新检查，目录联接点和符号链接不能把写入带到边界之外。
func EnsurePlainDirectoryPath(base string, target string, label string) error {
	base, err := ExistingPlainDirectory(base, "运行目录根")
	if err != nil {
		return err
	}
	target, err = filepath.Abs(strings.TrimSpace(target))
	if err != nil || strings.TrimSpace(target) == "" {
		return fmt.Errorf("%s无效", label)
	}
	target = filepath.Clean(target)
	if !PathWithin(base, target) {
		return fmt.Errorf("%s越过运行目录根", label)
	}
	relative, err := filepath.Rel(base, target)
	if err != nil {
		return fmt.Errorf("确定%s路径失败：%w", label, err)
	}
	if relative == "." {
		return nil
	}
	current := base
	for _, name := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, name)
		if err := EnsurePlainDirectory(current, label); err != nil {
			return err
		}
	}
	return nil
}

// EnsurePlainDirectory 确保目录存在且为普通目录。
func EnsurePlainDirectory(path string, label string) error {
	if err := AssertPlainDirectory(path, label); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Mkdir(path, 0o755); err != nil && !os.IsExist(err) {
		return fmt.Errorf("建立%s失败：%w", label, err)
	}
	return AssertPlainDirectory(path, label)
}

// AssertPlainDirectoryChain 检查路径每一级都是普通目录。
func AssertPlainDirectoryChain(path string, label string) error {
	chain := make([]string, 0, 8)
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		chain = append(chain, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	for index := len(chain) - 1; index >= 0; index-- {
		if err := AssertPlainDirectory(chain[index], label); err != nil {
			return err
		}
	}
	return nil
}

// AssertPlainDirectory 检查单个目录是普通目录。
func AssertPlainDirectory(path string, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("读取%s失败：%w", label, err)
	}
	reparsePoint, err := IsReparsePoint(path, info)
	if err != nil {
		return fmt.Errorf("检查%s目录边界失败：%w", label, err)
	}
	if reparsePoint {
		return fmt.Errorf("%s不能经过目录联接点或符号链接：%s", label, path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s必须是目录", label)
	}
	return nil
}

// PathWithin 判断 child 是否位于 base 之内。
func PathWithin(base string, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// SamePath 判断两个路径是否指向同一位置。
func SamePath(left string, right string) bool {
	left, _ = filepath.Abs(left)
	right, _ = filepath.Abs(right)
	return filepath.Clean(left) == filepath.Clean(right)
}
