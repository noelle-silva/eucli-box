package aiimage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

// 配置写锁：工具每次调用是独立进程，以锁文件对配置区做跨进程读-改-写串行化，
// 避免并行写入在替换阶段互相覆盖、静默丢失改动。
const (
	configWriteLockFileName = ".config-write.lock"
	configWriteLockWait     = 5 * time.Second
	configWriteLockPoll     = 50 * time.Millisecond
	configWriteLockStale    = 30 * time.Second
)

// configWriteLock 是一次配置写入持有的排他锁。
type configWriteLock struct {
	file *os.File
	path string
}

// acquireConfigWriteLock 获取配置写锁：锁被占用时轮询等待，
// 等待超时返回可重试的冲突提示；崩溃进程遗留的陈旧锁自动回收。
func acquireConfigWriteLock(input types.ToolExecutionInput) (*configWriteLock, error) {
	dataDir := strings.TrimSpace(input.ToolDataDirectory)
	if dataDir == "" {
		return nil, permanentError(errors.New("工具数据目录不可用"), actionFixConfig)
	}
	lockPath := filepath.Join(dataDir, configWriteLockFileName)
	deadline := time.Now().Add(configWriteLockWait)
	for {
		lock, err := createConfigWriteLock(lockPath)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, permanentError(fmt.Errorf("配置写锁不可用: %s", ioErrorReason(err)), actionFixConfig)
		}
		reclaimed, reclaimErr := reclaimStaleConfigWriteLock(lockPath)
		if reclaimErr != nil {
			return nil, permanentError(fmt.Errorf("配置写锁不可用: %s", ioErrorReason(reclaimErr)), actionFixConfig)
		}
		if reclaimed {
			continue
		}
		if time.Now().After(deadline) {
			return nil, retryableError(errors.New("配置正在被另一次写入占用，本次未执行"), actionRetryLater)
		}
		time.Sleep(configWriteLockPoll)
	}
}

func createConfigWriteLock(lockPath string) (*configWriteLock, error) {
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(file, "pid=%d\n", os.Getpid())
	return &configWriteLock{file: file, path: lockPath}, nil
}

// reclaimStaleConfigWriteLock 回收超过陈旧时限的遗留锁；未超时返回 false。
func reclaimStaleConfigWriteLock(lockPath string) (bool, error) {
	info, err := os.Stat(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	if time.Since(info.ModTime()) < configWriteLockStale {
		return false, nil
	}
	if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, nil
}

// release 释放配置写锁。
func (l *configWriteLock) release() {
	if l == nil {
		return
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	_ = os.Remove(l.path)
}
