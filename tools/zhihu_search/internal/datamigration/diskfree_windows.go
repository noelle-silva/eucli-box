//go:build windows

package datamigration

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32DiskFree         = syscall.NewLazyDLL("kernel32.dll")
	kernel32GetDiskFreeSpace = kernel32DiskFree.NewProc("GetDiskFreeSpaceExW")
)

// availableBytes 返回 path 所在卷的可用字节。
func availableBytes(path string) (uint64, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var freeBytesAvailable uint64
	result, _, callErr := kernel32GetDiskFreeSpace.Call(
		uintptr(unsafe.Pointer(name)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(new(uint64))),
		uintptr(unsafe.Pointer(new(uint64))),
	)
	if result == 0 {
		return 0, fmt.Errorf("GetDiskFreeSpaceExW failed: %v", callErr)
	}
	return freeBytesAvailable, nil
}

// checkDiskSpace 检查 path 所在卷可用字节不少于 required。
func checkDiskSpace(path string, required uint64) error {
	available, err := availableBytes(path)
	if err != nil {
		return err
	}
	if available < required {
		return fmt.Errorf("insufficient free space: %d bytes available, %d bytes required", available, required)
	}
	return nil
}
