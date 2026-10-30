package datastorage

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

// recycleRelocation 是「已确认源存在、已备好回收站目标」的待落名动作。
// 目录改名本身是唯一需要放进临界区的一步。
type recycleRelocation struct {
	Kind       types.StorageItemKind
	OriginalID string
	RecycleID  string
	Source     string
	Target     string
}

func (s *system) prepareRecycleRelocation(ctx context.Context, kind types.StorageItemKind, originalID string, source string) (recycleRelocation, error) {
	if err := ctx.Err(); err != nil {
		return recycleRelocation{}, storageDeleteFailed("delete cancelled", err)
	}
	if _, err := os.Stat(source); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return recycleRelocation{}, storageDeleteFailed("item does not exist", err)
		}
		return recycleRelocation{}, storageDeleteFailed("failed to inspect item before delete", err)
	}
	recycleID := fmt.Sprintf("%s-%s", kind, utils.NewID(originalID))
	target := filepath.Join(s.paths.recycleRoot(), recycleID)
	if err := os.MkdirAll(s.paths.recycleRoot(), 0o755); err != nil {
		return recycleRelocation{}, storageDeleteFailed("failed to create recycle directory", err)
	}
	return recycleRelocation{Kind: kind, OriginalID: originalID, RecycleID: recycleID, Source: source, Target: target}, nil
}

func (relocation recycleRelocation) commit() error {
	if err := os.Rename(relocation.Source, relocation.Target); err != nil {
		return storageDeleteFailed("failed to move item to recycle", err)
	}
	return nil
}

func (relocation recycleRelocation) rollback() error {
	if err := os.Rename(relocation.Target, relocation.Source); err != nil {
		return storageDeleteFailed("failed to rollback recycle relocation", err)
	}
	return nil
}

// writeRecycleMetadata 写回收站记录并重建回收站索引。
// recycleMu 是最内层锁：只保护回收站自身的元数据与索引。
func (s *system) writeRecycleMetadata(ctx context.Context, relocation recycleRelocation) error {
	s.recycleMu.Lock()
	defer s.recycleMu.Unlock()
	record := types.RecycleRecord{ID: relocation.RecycleID, OriginalID: relocation.OriginalID, OriginalType: relocation.Kind, DeletedAt: time.Now().UTC()}
	if err := writeJSON(ctx, filepath.Join(relocation.Target, "deleted-at.json"), record); err != nil {
		return err
	}
	// 回收站索引是派生缓存：重建失败只记日志，绝不回滚一次已经安全的删除。
	if err := rebuildRecycleIndex(ctx, s.paths); err != nil {
		log.Printf("data-storage-system: 回收站索引重建失败（条目已安全移入回收站，下次删除或启动会重试）：%v", err)
	}
	return nil
}

func (s *system) moveToRecycle(ctx context.Context, kind types.StorageItemKind, originalID string, source string) error {
	relocation, err := s.prepareRecycleRelocation(ctx, kind, originalID, source)
	if err != nil {
		return err
	}
	if err := relocation.commit(); err != nil {
		return err
	}
	if err := s.writeRecycleMetadata(ctx, relocation); err != nil {
		if rollbackErr := relocation.rollback(); rollbackErr != nil {
			return storageDeleteFailed(
				fmt.Sprintf("delete metadata write failed: %v; rollback also failed: %v", err, rollbackErr),
				fmt.Errorf("delete metadata write failed: %w; rollback also failed: %w", err, rollbackErr),
			)
		}
		return err
	}
	return nil
}
