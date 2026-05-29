package datastorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func moveToRecycle(ctx context.Context, paths paths, kind types.StorageItemKind, originalID string, source string) error {
	if err := ctx.Err(); err != nil {
		return storageDeleteFailed("delete cancelled", err)
	}
	if _, err := os.Stat(source); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return storageDeleteFailed("item does not exist", err)
		}
		return storageDeleteFailed("failed to inspect item before delete", err)
	}
	recycleID := fmt.Sprintf("%s-%s", kind, utils.NewID(originalID))
	target := filepath.Join(paths.recycleRoot(), recycleID)
	if err := os.MkdirAll(paths.recycleRoot(), 0o755); err != nil {
		return storageDeleteFailed("failed to create recycle directory", err)
	}
	if err := os.Rename(source, target); err != nil {
		return storageDeleteFailed("failed to move item to recycle", err)
	}
	record := types.RecycleRecord{ID: recycleID, OriginalID: originalID, OriginalType: kind, DeletedAt: time.Now().UTC()}
	if err := writeJSON(ctx, filepath.Join(target, "deleted-at.json"), record); err != nil {
		if rollbackErr := os.Rename(target, source); rollbackErr != nil {
			return storageDeleteFailed("delete metadata write failed and rollback also failed", rollbackErr)
		}
		return err
	}
	return rebuildRecycleIndex(ctx, paths)
}
