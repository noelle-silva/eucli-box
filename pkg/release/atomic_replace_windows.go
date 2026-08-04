//go:build windows

package release

import (
	"syscall"
	"unsafe"
)

var (
	kernel32MoveFileEx = syscall.NewLazyDLL("kernel32.dll")
	moveFileExProc     = kernel32MoveFileEx.NewProc("MoveFileExW")
)

const moveFileReplaceExisting = 0x1

// ReplaceFileAtomic 把 source 原子替换为目标文件，供下载落盘、本地复制等基础动作复用。
func ReplaceFileAtomic(source string, target string) error {
	sourceName, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	targetName, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	result, _, callErr := moveFileExProc.Call(
		uintptr(unsafe.Pointer(sourceName)),
		uintptr(unsafe.Pointer(targetName)),
		moveFileReplaceExisting,
	)
	if result == 0 {
		return callErr
	}
	return nil
}
