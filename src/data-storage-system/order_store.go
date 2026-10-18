package datastorage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// orderFile 是集合级排序事实：只记录 ID 顺序，不复制实体内容。
// 实体自身的增删改仍以各自 data.json 为唯一事实源。
type orderFile struct {
	IDs []string `json:"ids"`
}

func readOrderIDs(ctx context.Context, target string) ([]string, error) {
	if !dataFileExists(target) {
		return nil, nil
	}
	order, err := readJSON[orderFile](ctx, target)
	if err != nil {
		return nil, err
	}
	return normalizeOrderIDs(order.IDs), nil
}

func writeOrderIDs(ctx context.Context, target string, ids []string) error {
	return writeJSON(ctx, target, orderFile{IDs: normalizeOrderIDs(ids)})
}

func normalizeOrderIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

// mergeOrderIDs 把持久化顺序与当前实际存在的 ID 合并：
// 已知顺序优先，缺失的 ID 按字典序追加，保证读取结果稳定且不丢实体。
func mergeOrderIDs(order []string, existing []string) []string {
	present := make(map[string]struct{}, len(existing))
	for _, id := range existing {
		present[id] = struct{}{}
	}
	result := make([]string, 0, len(existing))
	seen := make(map[string]struct{}, len(existing))
	for _, id := range normalizeOrderIDs(order) {
		if _, ok := present[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	remaining := make([]string, 0, len(existing)-len(result))
	for id := range present {
		if _, ok := seen[id]; ok {
			continue
		}
		remaining = append(remaining, id)
	}
	sort.Strings(remaining)
	return append(result, remaining...)
}

func listObjectIDs(ctx context.Context, root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, storageReadFailed("failed to scan object directory", err)
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, storageReadFailed("scan cancelled", err)
		}
		if !entry.IsDir() {
			continue
		}
		if !dataFileExists(filepath.Join(root, entry.Name(), "data.json")) {
			continue
		}
		ids = append(ids, entry.Name())
	}
	return ids, nil
}

func orderedObjectIDs(ctx context.Context, root string, orderTarget string) ([]string, error) {
	existing, err := listObjectIDs(ctx, root)
	if err != nil {
		return nil, err
	}
	order, err := readOrderIDs(ctx, orderTarget)
	if err != nil {
		return nil, err
	}
	return mergeOrderIDs(order, existing), nil
}

// syncOrderAfterSave 在实体落盘后维护集合顺序：
// 新实体放到最前（与既有界面行为一致），已有实体保持原位；
// 同时把顺序文件中已不存在的 ID 清理掉。
func (s *system) syncOrderAfterSave(ctx context.Context, root string, orderTarget string, id string, isNew bool) error {
	existing, err := listObjectIDs(ctx, root)
	if err != nil {
		return err
	}
	order, err := readOrderIDs(ctx, orderTarget)
	if err != nil {
		return err
	}
	if isNew {
		order = append([]string{id}, order...)
	}
	return writeOrderIDs(ctx, orderTarget, mergeOrderIDs(order, existing))
}

func (s *system) syncOrderAfterDelete(ctx context.Context, root string, orderTarget string) error {
	existing, err := listObjectIDs(ctx, root)
	if err != nil {
		return err
	}
	order, err := readOrderIDs(ctx, orderTarget)
	if err != nil {
		return err
	}
	return writeOrderIDs(ctx, orderTarget, mergeOrderIDs(order, existing))
}

func (s *system) reorderObjects(ctx context.Context, root string, orderTarget string, ids []string) error {
	existing, err := listObjectIDs(ctx, root)
	if err != nil {
		return err
	}
	return reorderExistingIDs(ctx, orderTarget, ids, existing)
}

// reorderExistingIDs 校验并写入集合顺序；existing 是该集合当前实际存在的 ID。
func reorderExistingIDs(ctx context.Context, orderTarget string, ids []string, existing []string) error {
	requested := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			return storageInvalid("order id is required", nil)
		}
		if _, ok := seen[trimmed]; ok {
			return storageInvalid("order ids contain duplicate: "+trimmed, nil)
		}
		seen[trimmed] = struct{}{}
		requested = append(requested, trimmed)
	}
	if len(requested) == 0 {
		return storageInvalid("order ids are required", nil)
	}
	present := make(map[string]struct{}, len(existing))
	for _, id := range existing {
		present[id] = struct{}{}
	}
	for _, id := range requested {
		if _, ok := present[id]; !ok {
			return storageInvalid("order id does not exist: "+id, nil)
		}
	}
	order, err := readOrderIDs(ctx, orderTarget)
	if err != nil {
		return err
	}
	current := mergeOrderIDs(order, existing)
	remaining := make([]string, 0, len(current)-len(requested))
	for _, id := range current {
		if _, ok := seen[id]; !ok {
			remaining = append(remaining, id)
		}
	}
	return writeOrderIDs(ctx, orderTarget, append(requested, remaining...))
}

func orderedIDsForExisting(ctx context.Context, orderTarget string, existing []string) ([]string, error) {
	order, err := readOrderIDs(ctx, orderTarget)
	if err != nil {
		return nil, err
	}
	return mergeOrderIDs(order, existing), nil
}

func prependOrderID(ctx context.Context, orderTarget string, id string, existing []string) error {
	order, err := readOrderIDs(ctx, orderTarget)
	if err != nil {
		return err
	}
	return writeOrderIDs(ctx, orderTarget, mergeOrderIDs(append([]string{id}, order...), existing))
}

func removeOrderID(ctx context.Context, orderTarget string, id string, existing []string) error {
	order, err := readOrderIDs(ctx, orderTarget)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(order))
	for _, item := range order {
		if item != id {
			filtered = append(filtered, item)
		}
	}
	return writeOrderIDs(ctx, orderTarget, mergeOrderIDs(filtered, existing))
}

func replaceOrderID(ctx context.Context, orderTarget string, oldID string, newID string, existing []string) error {
	order, err := readOrderIDs(ctx, orderTarget)
	if err != nil {
		return err
	}
	replaced := make([]string, 0, len(order)+1)
	found := false
	for _, item := range order {
		if item == oldID {
			if !found {
				replaced = append(replaced, newID)
				found = true
			}
			continue
		}
		if item != newID {
			replaced = append(replaced, item)
		}
	}
	if !found {
		replaced = append([]string{newID}, replaced...)
	}
	return writeOrderIDs(ctx, orderTarget, mergeOrderIDs(replaced, existing))
}
