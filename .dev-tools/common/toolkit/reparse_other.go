//go:build !windows

package toolkit

import "os"

// IsReparsePoint 判断路径是否经过符号链接。
func IsReparsePoint(_ string, info os.FileInfo) (bool, error) {
	return info.Mode()&os.ModeSymlink != 0, nil
}
