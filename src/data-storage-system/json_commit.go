package datastorage

import (
	"errors"
	"log"
	"math/rand/v2"
	"os"
	"sync/atomic"
	"syscall"
	"time"
)

// commitRetryDelays 是每次重试前的等待时间，总预算上限为 260ms。
// 数组长度就是重试次数；等待只发生在已判定为瞬时占用之后，
// 正常写入路径没有额外开销。
var commitRetryDelays = [...]time.Duration{
	10 * time.Millisecond,
	50 * time.Millisecond,
	200 * time.Millisecond,
}

type storageWriteMetrics struct {
	commitRetries        atomic.Uint64
	commitRetryExhausted atomic.Uint64
}

var writeMetrics storageWriteMetrics

// StorageWriteMetrics 是通用文档写入的只读计数快照。
type StorageWriteMetrics struct {
	CommitRetries        uint64 `json:"commitRetries"`
	CommitRetryExhausted uint64 `json:"commitRetryExhausted"`
}

// CurrentStorageWriteMetrics 返回当前通用文档写入计数快照。
func CurrentStorageWriteMetrics() StorageWriteMetrics {
	return StorageWriteMetrics{
		CommitRetries:        writeMetrics.commitRetries.Load(),
		CommitRetryExhausted: writeMetrics.commitRetryExhausted.Load(),
	}
}

type commitFailureDetails struct {
	Operation       string `json:"operation"`
	Target          string `json:"target"`
	SystemErrorCode int    `json:"systemErrorCode,omitempty"`
}

// commitJSONWithRetry 把临时文件替换成正式文件。
// 只有平台判定为瞬时占用的错误才进入重试；其他错误立即失败，
// 重试预算耗尽后仍按 storage.write_failed 如实返回，绝不吞错。
// 重试复用同一个临时文件，替换动作本身重复执行结果一致。
func commitJSONWithRetry(temp string, target string) error {
	err := os.Rename(temp, target)
	if err == nil {
		return nil
	}
	if !isRetryableCommitError(err) {
		return commitWriteFailed(target, err)
	}
	for attempt, delay := range commitRetryDelays {
		writeMetrics.commitRetries.Add(1)
		time.Sleep(jitteredRetryDelay(delay))
		err = os.Rename(temp, target)
		if err == nil {
			log.Printf("data-storage-system: 文件替换遇到瞬时占用，第 %d 次重试后成功：%s", attempt+1, target)
			return nil
		}
		if !isRetryableCommitError(err) {
			return commitWriteFailed(target, err)
		}
	}
	writeMetrics.commitRetryExhausted.Add(1)
	failure := commitWriteFailed(target, err)
	log.Printf("data-storage-system: 文件替换重试耗尽：target=%s err=%v", target, failure)
	return failure
}

func commitWriteFailed(target string, err error) error {
	return storageWriteFailedWithDetails("failed to replace json file", err, commitFailureDetails{
		Operation:       "rename",
		Target:          target,
		SystemErrorCode: systemErrorCode(err),
	})
}

func systemErrorCode(err error) int {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return int(errno)
	}
	return 0
}

// jitteredRetryDelay 只在基础间隔上向下抖动，保证总等待时间不超过预算。
func jitteredRetryDelay(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	span := int64(base / 5)
	if span <= 0 {
		return base
	}
	return base - time.Duration(rand.Int64N(span+1))
}
