//go:build windows

package localrun

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	localRunLockKernel32   = syscall.NewLazyDLL("kernel32.dll")
	localRunLockCreateFile = localRunLockKernel32.NewProc("CreateFileW")
	localRunLockClose      = localRunLockKernel32.NewProc("CloseHandle")
)

const (
	localRunLockGenericRead  = 0x80000000
	localRunLockGenericWrite = 0x40000000
	localRunLockOpenAlways   = 4
	localRunLockNormal       = 0x80
	localRunInvalidHandle    = ^uintptr(0)
)

func acquireDataLock(path string) (*DataLock, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, _, callErr := localRunLockCreateFile.Call(
		uintptr(unsafe.Pointer(name)),
		localRunLockGenericRead|localRunLockGenericWrite,
		0,
		0,
		localRunLockOpenAlways,
		localRunLockNormal,
		0,
	)
	if handle == localRunInvalidHandle {
		return nil, fmt.Errorf("LOCAL_BOX_DATA_IN_USE：%w", callErr)
	}
	return &DataLock{
		path: path,
		release: func() error {
			result, _, closeErr := localRunLockClose.Call(handle)
			if result == 0 {
				return fmt.Errorf("释放数据独占证明失败：%w", closeErr)
			}
			return nil
		},
	}, nil
}
