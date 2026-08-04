//go:build !windows

package release

import "os"

// ReplaceFileAtomic 把 source 原子替换为目标文件，供下载落盘、本地复制等基础动作复用。
func ReplaceFileAtomic(source string, target string) error {
	return os.Rename(source, target)
}
