//go:build windows

package localrun

import (
	"syscall"
	"unsafe"
)

var (
	localRunKernel32MoveFileEx = syscall.NewLazyDLL("kernel32.dll")
	localRunMoveFileExProc     = localRunKernel32MoveFileEx.NewProc("MoveFileExW")
)

func atomicReplace(source string, target string) error {
	sourceName, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	targetName, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	result, _, callErr := localRunMoveFileExProc.Call(uintptr(unsafe.Pointer(sourceName)), uintptr(unsafe.Pointer(targetName)), 0x1)
	if result == 0 {
		return callErr
	}
	return nil
}
