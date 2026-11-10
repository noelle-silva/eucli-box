//go:build windows

package datastorage

import (
	"errors"
	"syscall"
)

const (
	errorAccessDenied     syscall.Errno = 5
	errorSharingViolation syscall.Errno = 32
)

// isRetryableCommitError 只把 Windows 的瞬时占用判定为可重试：
// 目标文件被其他程序打开时，替换会返回“访问被拒绝”或“共享冲突”。
func isRetryableCommitError(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == errorAccessDenied || errno == errorSharingViolation
}
