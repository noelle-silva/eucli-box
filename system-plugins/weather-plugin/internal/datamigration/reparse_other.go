//go:build !windows

package datamigration

import "os"

// isReparsePoint 检测路径是否经过符号链接。
func isReparsePoint(_ string, info os.FileInfo) (bool, error) {
	return info.Mode()&os.ModeSymlink != 0, nil
}
