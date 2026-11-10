//go:build !windows

package datastorage

// isRetryableCommitError 在非 Windows 平台不重试：
// 这些平台的文件替换不会被读取句柄阻塞。
func isRetryableCommitError(err error) bool {
	return false
}
