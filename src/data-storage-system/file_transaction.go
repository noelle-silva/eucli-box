package datastorage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// fileTransaction 把一组 JSON 写入合并成一次可回滚的提交。
// 所有目标先在暂存区完成编码与旧内容快照，任一暂存失败都不会触碰任何现存文件；
// 提交阶段一旦某个文件替换失败，则按逆序恢复已提交文件的旧内容，避免半吊子事实。
type fileTransaction struct {
	commitFile func(temp string, target string) error
	entries    []*fileTransactionEntry
}

type fileTransactionEntry struct {
	target    string
	temp      string
	previous  []byte
	existed   bool
	committed bool
}

func newFileTransaction() *fileTransaction {
	return &fileTransaction{commitFile: commitJSON}
}

func (tx *fileTransaction) StageJSON(ctx context.Context, target string, value any) error {
	previous, existed, err := snapshotFile(target)
	if err != nil {
		return err
	}
	temp, err := prepareJSON(ctx, target, value)
	if err != nil {
		return err
	}
	tx.entries = append(tx.entries, &fileTransactionEntry{
		target:   target,
		temp:     temp,
		previous: previous,
		existed:  existed,
	})
	return nil
}

func (tx *fileTransaction) Commit() error {
	for _, entry := range tx.entries {
		if err := tx.commitFile(entry.temp, entry.target); err != nil {
			return errors.Join(err, tx.rollbackCommitted())
		}
		entry.temp = ""
		entry.committed = true
	}
	return nil
}

// Discard 清理尚未提交的暂存文件；可重复调用。
func (tx *fileTransaction) Discard() {
	for _, entry := range tx.entries {
		if entry.temp != "" {
			_ = os.Remove(entry.temp)
			entry.temp = ""
		}
	}
}

// rollbackCommitted 按提交逆序恢复旧内容。返回的错误只用于说明回滚失败，
// 原始提交错误由调用方保留。
func (tx *fileTransaction) rollbackCommitted() error {
	var failures []error
	for index := len(tx.entries) - 1; index >= 0; index-- {
		entry := tx.entries[index]
		if !entry.committed {
			continue
		}
		var err error
		if entry.existed {
			err = replaceFileBytes(entry.target, entry.previous)
		} else if removeErr := os.Remove(entry.target); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = removeErr
		}
		if err != nil {
			failures = append(failures, err)
		}
		entry.committed = false
	}
	return errors.Join(failures...)
}

func snapshotFile(target string) ([]byte, bool, error) {
	payload, err := os.ReadFile(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, storageReadFailed("failed to snapshot file for transaction", err)
	}
	return payload, true, nil
}

func replaceFileBytes(target string, payload []byte) error {
	dir := filepath.Dir(target)
	if err := ensureDirs(dir); err != nil {
		return storageWriteFailed("failed to create parent directory", err)
	}
	temp, err := os.CreateTemp(dir, ".rollback-*.json")
	if err != nil {
		return storageWriteFailed("failed to create rollback file", err)
	}
	tempName := temp.Name()
	if _, err := temp.Write(payload); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return storageWriteFailed("failed to write rollback file", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return storageWriteFailed("failed to close rollback file", err)
	}
	if err := commitJSON(tempName, target); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	return nil
}
