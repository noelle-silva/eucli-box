//go:build !windows

package release

import "os"

// isReparsePoint 在非 Windows 平台只依赖符号链接位；目录联接是 Windows 专属概念。
func isReparsePoint(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSymlink != 0, nil
}
