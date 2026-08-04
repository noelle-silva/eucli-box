package localrun

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type DataLock struct {
	path    string
	release func() error
	once    sync.Once
	err     error
}

func AcquireDataLock(dataDir string) (*DataLock, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, fmt.Errorf("数据目录不能为空")
	}
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("数据目录无效：%w", err)
	}
	absolute = filepath.Clean(absolute)
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("建立数据目录失败：%w", err)
	}
	path, err := DataLockPath(absolute)
	if err != nil {
		return nil, err
	}
	return acquireDataLock(path)
}

func (lock *DataLock) Path() string {
	if lock == nil {
		return ""
	}
	return lock.path
}

func (lock *DataLock) Release() error {
	if lock == nil {
		return nil
	}
	lock.once.Do(func() { lock.err = lock.release() })
	return lock.err
}
