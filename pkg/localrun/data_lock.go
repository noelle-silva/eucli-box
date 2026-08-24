package localrun

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DataLockPath 计算数据目录锁文件路径：锁文件放数据目录的上一级，避免锁文件进入数据目录本身。
func DataLockPath(dataDir string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(dataDir))
	if err != nil || strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("数据目录无效")
	}
	return filepath.Join(filepath.Dir(filepath.Clean(absolute)), "data.lock"), nil
}

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
