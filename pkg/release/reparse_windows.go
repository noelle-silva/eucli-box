//go:build windows

package release

import (
	"os"
	"syscall"
	"unsafe"
)

const fileAttributeReparsePoint = 0x400

var (
	kernel32GetFileAttributes = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFileAttributesW")
)

// isReparsePoint 检测路径是否带有重解析点属性（目录联接、符号链接等）。
// 目录必须通过显式参数创建；junction 在 os.Lstat 下表现为普通目录，必须用属性检测。
func isReparsePoint(path string) (bool, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	attributes, _, _ := kernel32GetFileAttributes.Call(uintptr(unsafe.Pointer(name)))
	if attributes == uintptr(^uint32(0)) {
		return false, os.ErrNotExist
	}
	return attributes&fileAttributeReparsePoint != 0, nil
}
