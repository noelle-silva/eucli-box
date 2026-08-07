//go:build windows

package localrun

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	localRunKernel32             = syscall.NewLazyDLL("kernel32.dll")
	localRunAdvapi32             = syscall.NewLazyDLL("advapi32.dll")
	localRunGetCurrentProcess    = localRunKernel32.NewProc("GetCurrentProcess")
	localRunCloseHandle          = localRunKernel32.NewProc("CloseHandle")
	localRunOpenProcess          = localRunKernel32.NewProc("OpenProcess")
	localRunGetProcessTimes      = localRunKernel32.NewProc("GetProcessTimes")
	localRunOpenProcessToken     = localRunAdvapi32.NewProc("OpenProcessToken")
	localRunGetTokenInformation  = localRunAdvapi32.NewProc("GetTokenInformation")
	localRunGetLengthSid         = localRunAdvapi32.NewProc("GetLengthSid")
	localRunCreateWellKnownSid   = localRunAdvapi32.NewProc("CreateWellKnownSid")
	localRunSetEntriesInAcl      = localRunAdvapi32.NewProc("SetEntriesInAclW")
	localRunGetNamedSecurityInfo = localRunAdvapi32.NewProc("GetNamedSecurityInfoW")
	localRunSetNamedSecurityInfo = localRunAdvapi32.NewProc("SetNamedSecurityInfoW")
	localRunGetAce               = localRunAdvapi32.NewProc("GetAce")
	localRunEqualSid             = localRunAdvapi32.NewProc("EqualSid")
	localRunLocalFree            = localRunKernel32.NewProc("LocalFree")
	localRunCreateFile           = localRunKernel32.NewProc("CreateFileW")
	localRunGetLastError         = localRunKernel32.NewProc("GetLastError")
)

const (
	localRunTokenQuery                     = 0x0008
	localRunTokenUser                      = 1
	localRunProcessQueryLimitedInformation = 0x1000
	localRunFileGenericRead                = 0x80000000
	localRunFileGenericWrite               = 0x40000000
	localRunFileDelete                     = 0x00010000
	localRunOpenAlways                     = 4
	localRunFileAttributeNormal            = 0x80
	localRunInvalidHandleValue             = ^uintptr(0)
	localRunSecurityInformationDACL        = 0x00000004
	localRunSEFileObject                   = 1
	localRunSetAccess                      = 2
	localRunRevokeAccess                   = 4
	localRunTrusteeIsSID                   = 0
	localRunTrusteeIsUser                  = 1
	localRunObjectInherit                  = 0x1
	localRunContainerInherit               = 0x2
	localRunWorldSID                       = 1
	localRunAuthenticatedUserSID           = 17
	localRunBuiltinUsersSID                = 27
	localRunFileReadMask                   = 0x00120089
	localRunFileWriteMask                  = 0x00120116
	localRunProtectedDACL                  = 0x80000000
	localRunErrorInvalidParameter          = syscall.Errno(87)
)

type localRunTrustee struct {
	MultipleTrustee          unsafe.Pointer
	MultipleTrusteeOperation uint32
	TrusteeForm              uint32
	TrusteeType              uint32
	Name                     unsafe.Pointer
}

type localRunExplicitAccess struct {
	Permissions uint32
	AccessMode  uint32
	Inheritance uint32
	Trustee     localRunTrustee
}

func ProtectFileForCurrentUser(path string) error {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	userSID, err := localRunCurrentUserSID()
	if err != nil {
		return err
	}
	worldSID, err := localRunWellKnownSID(localRunWorldSID)
	if err != nil {
		return err
	}
	authSID, err := localRunWellKnownSID(localRunAuthenticatedUserSID)
	if err != nil {
		return err
	}
	usersSID, err := localRunWellKnownSID(localRunBuiltinUsersSID)
	if err != nil {
		return err
	}
	var securityDescriptor unsafe.Pointer
	result, _, callErr := localRunGetNamedSecurityInfo.Call(
		uintptr(unsafe.Pointer(name)), localRunSEFileObject, localRunSecurityInformationDACL,
		0, 0, 0, 0, uintptr(unsafe.Pointer(&securityDescriptor)),
	)
	if result != 0 {
		return fmt.Errorf("读取资料 ACL 失败：%w", syscall.Errno(result))
	}
	defer localRunLocalFree.Call(uintptr(securityDescriptor))
	entries := []localRunExplicitAccess{localRunGrantEntry(userSID)}
	var newDACL unsafe.Pointer
	result, _, callErr = localRunSetEntriesInAcl.Call(uintptr(len(entries)), uintptr(unsafe.Pointer(&entries[0])), 0, uintptr(unsafe.Pointer(&newDACL)))
	if result != 0 {
		return fmt.Errorf("建立资料 ACL 失败：%w", syscall.Errno(result))
	}
	defer localRunLocalFree.Call(uintptr(newDACL))
	result, _, callErr = localRunSetNamedSecurityInfo.Call(uintptr(unsafe.Pointer(name)), localRunSEFileObject, localRunSecurityInformationDACL|localRunProtectedDACL, 0, 0, uintptr(newDACL), 0)
	if result != 0 {
		return fmt.Errorf("写入资料 ACL 失败：%w", syscall.Errno(result))
	}
	if err := localRunVerifyACL(path, userSID, worldSID, authSID, usersSID); err != nil {
		return err
	}
	_ = callErr
	return nil
}

func ProcessStartedAt(pid int) (time.Time, error) {
	if pid <= 0 {
		return time.Time{}, fmt.Errorf("进程编号无效")
	}
	handle, _, err := localRunOpenProcess.Call(localRunProcessQueryLimitedInformation, 0, uintptr(pid))
	if handle == 0 {
		return time.Time{}, fmt.Errorf("打开进程失败：%w", err)
	}
	defer localRunCloseHandle.Call(handle)
	var creation, exit, kernel, user syscall.Filetime
	result, _, callErr := localRunGetProcessTimes.Call(handle, uintptr(unsafe.Pointer(&creation)), uintptr(unsafe.Pointer(&exit)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if result == 0 {
		return time.Time{}, fmt.Errorf("读取进程开始时间失败：%w", callErr)
	}
	return filetimeToUTC(creation), nil
}

func ProcessMatches(pid int, startedAt time.Time) (bool, error) {
	actual, err := ProcessStartedAt(pid)
	if err != nil {
		if errors.Is(err, localRunErrorInvalidParameter) {
			return false, nil
		}
		return false, err
	}
	return actual.Equal(startedAt.UTC()), nil
}

func localRunCurrentUserSID() ([]byte, error) {
	process, _, _ := localRunGetCurrentProcess.Call()
	var token uintptr
	result, _, err := localRunOpenProcessToken.Call(process, localRunTokenQuery, uintptr(unsafe.Pointer(&token)))
	if result == 0 {
		return nil, fmt.Errorf("读取当前 Windows 用户失败：%w", err)
	}
	defer localRunCloseHandle.Call(token)
	var size uint32
	localRunGetTokenInformation.Call(token, localRunTokenUser, 0, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return nil, fmt.Errorf("读取当前 Windows 用户资料失败")
	}
	buffer := make([]byte, size)
	result, _, err = localRunGetTokenInformation.Call(token, localRunTokenUser, uintptr(unsafe.Pointer(&buffer[0])), uintptr(size), uintptr(unsafe.Pointer(&size)))
	if result == 0 {
		return nil, fmt.Errorf("读取当前 Windows 用户资料失败：%w", err)
	}
	sid := *(*unsafe.Pointer)(unsafe.Pointer(&buffer[0]))
	return localRunCopySID(sid)
}

func localRunWellKnownSID(kind uint32) ([]byte, error) {
	var size uint32
	localRunCreateWellKnownSid.Call(uintptr(kind), 0, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return nil, fmt.Errorf("建立 Windows 系统用户组身份失败")
	}
	buffer := make([]byte, size)
	result, _, err := localRunCreateWellKnownSid.Call(uintptr(kind), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)))
	if result == 0 {
		return nil, fmt.Errorf("建立 Windows 系统用户组身份失败：%w", err)
	}
	return buffer[:size], nil
}

func localRunCopySID(pointer unsafe.Pointer) ([]byte, error) {
	if pointer == nil {
		return nil, fmt.Errorf("Windows 用户身份为空")
	}
	length, _, _ := localRunGetLengthSid.Call(uintptr(pointer))
	if length == 0 {
		return nil, fmt.Errorf("Windows 用户身份无效")
	}
	result := make([]byte, length)
	copy(result, unsafe.Slice((*byte)(pointer), length))
	return result, nil
}

func localRunGrantEntry(sid []byte) localRunExplicitAccess {
	return localRunExplicitAccess{Permissions: localRunFileGenericRead | localRunFileGenericWrite | localRunFileDelete, AccessMode: localRunSetAccess, Inheritance: localRunObjectInherit | localRunContainerInherit, Trustee: localRunTrustee{TrusteeForm: localRunTrusteeIsSID, TrusteeType: localRunTrusteeIsUser, Name: unsafe.Pointer(&sid[0])}}
}

func localRunVerifyACL(path string, userSID []byte, worldSID []byte, authSID []byte, usersSID []byte) error {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var dacl unsafe.Pointer
	var descriptor unsafe.Pointer
	result, _, _ := localRunGetNamedSecurityInfo.Call(uintptr(unsafe.Pointer(name)), localRunSEFileObject, localRunSecurityInformationDACL, 0, 0, uintptr(unsafe.Pointer(&dacl)), 0, uintptr(unsafe.Pointer(&descriptor)))
	if result != 0 {
		return fmt.Errorf("核对资料 ACL 失败：%w", syscall.Errno(result))
	}
	defer localRunLocalFree.Call(uintptr(descriptor))
	if dacl == nil {
		return fmt.Errorf("资料 ACL 不存在")
	}
	acl := (*localRunACL)(dacl)
	userReadable := false
	for index := uint32(0); index < uint32(acl.AceCount); index++ {
		var ace unsafe.Pointer
		result, _, _ := localRunGetAce.Call(uintptr(dacl), uintptr(index), uintptr(unsafe.Pointer(&ace)))
		if result == 0 || ace == nil {
			return fmt.Errorf("读取资料 ACL 条目失败")
		}
		header := (*localRunACEHeader)(ace)
		if header.AceType != 0 {
			continue
		}
		allowed := *(*uint32)(unsafe.Add(ace, 4))
		sidStart := unsafe.Add(ace, 8)
		sid, err := localRunCopySID(sidStart)
		if err != nil {
			return err
		}
		userHasGeneric := allowed&(localRunFileGenericRead|localRunFileGenericWrite|localRunFileDelete) == (localRunFileGenericRead | localRunFileGenericWrite | localRunFileDelete)
		userHasMapped := allowed&localRunFileReadMask == localRunFileReadMask && allowed&localRunFileWriteMask == localRunFileWriteMask
		if localRunSIDsEqual(sid, userSID) && (userHasGeneric || userHasMapped) {
			userReadable = true
		}
		publicHasRead := allowed&localRunFileReadMask != 0 || allowed&localRunFileGenericRead != 0
		if (localRunSIDsEqual(sid, worldSID) || localRunSIDsEqual(sid, authSID) || localRunSIDsEqual(sid, usersSID)) && publicHasRead {
			return fmt.Errorf("资料 ACL 向公共用户开放读取权限")
		}
	}
	if !userReadable {
		return fmt.Errorf("当前 Windows 用户没有资料读写权限")
	}
	return nil
}

type localRunACL struct {
	AclRevision byte
	Sbz1        byte
	AclSize     uint16
	AceCount    uint16
	Sbz2        uint16
}

type localRunACEHeader struct {
	AceType  byte
	AceFlags byte
	AceSize  uint16
}

func localRunSIDsEqual(left []byte, right []byte) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	result, _, _ := localRunEqualSid.Call(uintptr(unsafe.Pointer(&left[0])), uintptr(unsafe.Pointer(&right[0])))
	return result != 0
}

func filetimeToUTC(value syscall.Filetime) time.Time {
	const windowsEpochOffset = 116444736000000000
	windowsTicks := (uint64(value.HighDateTime) << 32) | uint64(value.LowDateTime)
	nanoseconds := int64(windowsTicks-windowsEpochOffset) * 100
	return time.Unix(0, nanoseconds).UTC()
}

func localRunHandleError() error {
	value, _, _ := localRunGetLastError.Call()
	return os.NewSyscallError("Windows", syscall.Errno(value))
}
