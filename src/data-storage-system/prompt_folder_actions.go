package datastorage

import (
	"context"
	"slices"
	"strings"
	"time"

	"eucli-box/pkg/datapaths"
	"eucli-box/pkg/types"
)

// 收藏夹的定点动作：删除、重命名、排序。
// 三者都只改收藏夹索引，不碰提示词本体；每次动作读—改—写走同一把实体锁，
// 保存前复用整库校验，结构问题一律拒绝。

// DeletePromptFolder 删除一个收藏夹。
// 收藏夹可被多个上级收录，删除时要摘掉根级与每个上级对它的引用，
// 它记录的子收藏夹按原顺序上移到这些位置；它自己的书签一并消失，提示词本体不动。
func (s *system) DeletePromptFolder(ctx context.Context, scope string, ownerID string, folderID string) (types.PromptFolderLibrary, error) {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder id is required", nil)
	}
	return s.mutatePromptFolders(ctx, scope, ownerID, func(library types.PromptFolderLibrary) (types.PromptFolderLibrary, error) {
		target, found := findPromptFolder(library, folderID)
		if !found {
			return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+folderID, nil)
		}
		targetKey := promptEntryKey(promptEntryKindFolder, folderID)

		if index := indexOfID(library.RootFolderIDs, folderID); index >= 0 {
			promoted := missingIDs(target.FolderIDs, library.RootFolderIDs)
			library.RootFolderIDs = spliceIDs(library.RootFolderIDs, index, 1, promoted...)
			if next, replaced := replaceEntryKey(library.RootEntryOrder, targetKey, folderKeys(promoted)); replaced {
				library.RootEntryOrder = next
			}
		}
		for position := range library.Folders {
			parent := &library.Folders[position]
			if parent.ID == folderID {
				continue
			}
			index := indexOfID(parent.FolderIDs, folderID)
			if index < 0 {
				continue
			}
			promoted := missingIDs(target.FolderIDs, parent.FolderIDs)
			parent.FolderIDs = spliceIDs(parent.FolderIDs, index, 1, promoted...)
			if next, replaced := replaceEntryKey(parent.EntryOrder, targetKey, folderKeys(promoted)); replaced {
				parent.EntryOrder = next
			}
		}

		folders := make([]types.PromptFolder, 0, len(library.Folders)-1)
		for _, folder := range library.Folders {
			if folder.ID != folderID {
				folders = append(folders, folder)
			}
		}
		library.Folders = folders
		return library, nil
	})
}

// RenamePromptFolder 只改收藏夹名字：不动物件结构、不动索引顺序。
func (s *system) RenamePromptFolder(ctx context.Context, scope string, ownerID string, folderID string, name string) (types.PromptFolderLibrary, error) {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder id is required", nil)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder name is required", nil)
	}
	return s.mutatePromptFolders(ctx, scope, ownerID, func(library types.PromptFolderLibrary) (types.PromptFolderLibrary, error) {
		for position := range library.Folders {
			if library.Folders[position].ID != folderID {
				continue
			}
			library.Folders[position].Name = name
			return library, nil
		}
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+folderID, nil)
	})
}

// ReorderPromptFolderEntries 重排某一层的混排顺序：parentID 为空表示根级。
// 提交的顺序必须与本层的收藏夹与条目完全一致，少一个、多一个、重复都拒绝。
func (s *system) ReorderPromptFolderEntries(ctx context.Context, scope string, ownerID string, parentID string, entryOrder []string) (types.PromptFolderLibrary, error) {
	parentID = strings.TrimSpace(parentID)
	order := make([]string, 0, len(entryOrder))
	for _, item := range entryOrder {
		order = append(order, strings.TrimSpace(item))
	}
	return s.mutatePromptFolders(ctx, scope, ownerID, func(library types.PromptFolderLibrary) (types.PromptFolderLibrary, error) {
		if parentID == "" {
			library.RootEntryOrder = order
			return library, nil
		}
		for position := range library.Folders {
			if library.Folders[position].ID != parentID {
				continue
			}
			library.Folders[position].EntryOrder = order
			return library, nil
		}
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+parentID, nil)
	})
}

// MovePromptFolder 把收藏夹从当前层移动到另一层：空 id 表示根级。
// 收藏夹可被多个上级收录，所以只摘掉来源这一处，其它位置的引用保持不变。
func (s *system) MovePromptFolder(ctx context.Context, scope string, ownerID string, folderID string, fromParentID string, toParentID string) (types.PromptFolderLibrary, error) {
	folderID = strings.TrimSpace(folderID)
	fromParentID = strings.TrimSpace(fromParentID)
	toParentID = strings.TrimSpace(toParentID)
	if folderID == "" {
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder id is required", nil)
	}
	if fromParentID == toParentID {
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder already lives here: "+folderID, nil)
	}
	return s.mutatePromptFolders(ctx, scope, ownerID, func(library types.PromptFolderLibrary) (types.PromptFolderLibrary, error) {
		if _, found := findPromptFolder(library, folderID); !found {
			return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+folderID, nil)
		}
		targetKey := promptEntryKey(promptEntryKindFolder, folderID)
		if toParentID != "" {
			if _, found := findPromptFolder(library, toParentID); !found {
				return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+toParentID, nil)
			}
			if toParentID == folderID || isPromptFolderDescendant(library, folderID, toParentID) {
				return types.PromptFolderLibrary{}, storageInvalid("prompt folder cannot be moved into its own descendant: "+toParentID, nil)
			}
		}
		if fromParentID == "" {
			index := indexOfID(library.RootFolderIDs, folderID)
			if index < 0 {
				return types.PromptFolderLibrary{}, storageInvalid("prompt folder is not listed under root: "+folderID, nil)
			}
			library.RootFolderIDs = spliceIDs(library.RootFolderIDs, index, 1)
			if next, removed := removeEntryKey(library.RootEntryOrder, targetKey); removed {
				library.RootEntryOrder = next
			}
		} else {
			detached := false
			for position := range library.Folders {
				parent := &library.Folders[position]
				if parent.ID != fromParentID {
					continue
				}
				index := indexOfID(parent.FolderIDs, folderID)
				if index < 0 {
					return types.PromptFolderLibrary{}, storageInvalid("prompt folder "+fromParentID+" does not contain: "+folderID, nil)
				}
				parent.FolderIDs = spliceIDs(parent.FolderIDs, index, 1)
				if next, removed := removeEntryKey(parent.EntryOrder, targetKey); removed {
					parent.EntryOrder = next
				}
				detached = true
				break
			}
			if !detached {
				return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+fromParentID, nil)
			}
		}
		if toParentID == "" {
			library.RootFolderIDs = append(library.RootFolderIDs, folderID)
			library.RootEntryOrder = append(library.RootEntryOrder, targetKey)
			return library, nil
		}
		for position := range library.Folders {
			parent := &library.Folders[position]
			if parent.ID != toParentID {
				continue
			}
			if indexOfID(parent.FolderIDs, folderID) >= 0 {
				return types.PromptFolderLibrary{}, storageInvalid("prompt folder "+toParentID+" already contains: "+folderID, nil)
			}
			parent.FolderIDs = append(parent.FolderIDs, folderID)
			parent.EntryOrder = append(parent.EntryOrder, targetKey)
			break
		}
		return library, nil
	})
}

// AddFolderToFolder 把收藏夹收藏到另一层：只加一处索引，别处引用不动；空 id 表示收进根级。
func (s *system) AddFolderToFolder(ctx context.Context, scope string, ownerID string, folderID string, parentID string) (types.PromptFolderLibrary, error) {
	folderID = strings.TrimSpace(folderID)
	parentID = strings.TrimSpace(parentID)
	if folderID == "" {
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder id is required", nil)
	}
	return s.mutatePromptFolders(ctx, scope, ownerID, func(library types.PromptFolderLibrary) (types.PromptFolderLibrary, error) {
		if _, found := findPromptFolder(library, folderID); !found {
			return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+folderID, nil)
		}
		targetKey := promptEntryKey(promptEntryKindFolder, folderID)
		if parentID == "" {
			if indexOfID(library.RootFolderIDs, folderID) >= 0 {
				return types.PromptFolderLibrary{}, storageInvalid("prompt folder already lives under root: "+folderID, nil)
			}
			library.RootFolderIDs = append(library.RootFolderIDs, folderID)
			library.RootEntryOrder = append(library.RootEntryOrder, targetKey)
			return library, nil
		}
		if parentID == folderID || isPromptFolderDescendant(library, folderID, parentID) {
			return types.PromptFolderLibrary{}, storageInvalid("prompt folder cannot be collected into its own descendant: "+parentID, nil)
		}
		for position := range library.Folders {
			parent := &library.Folders[position]
			if parent.ID != parentID {
				continue
			}
			if indexOfID(parent.FolderIDs, folderID) >= 0 {
				return types.PromptFolderLibrary{}, storageInvalid("prompt folder "+parentID+" already contains: "+folderID, nil)
			}
			parent.FolderIDs = append(parent.FolderIDs, folderID)
			parent.EntryOrder = append(parent.EntryOrder, targetKey)
			return library, nil
		}
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+parentID, nil)
	})
}

// AddPromptToFolder 把一条提示词收进收藏夹：追加到索引与混排顺序的末尾。
func (s *system) AddPromptToFolder(ctx context.Context, scope string, ownerID string, folderID string, promptID string) (types.PromptFolderLibrary, error) {
	folderID = strings.TrimSpace(folderID)
	promptID = strings.TrimSpace(promptID)
	if folderID == "" {
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder id is required", nil)
	}
	if promptID == "" {
		return types.PromptFolderLibrary{}, storageInvalid("prompt id is required", nil)
	}
	return s.mutatePromptFolders(ctx, scope, ownerID, func(library types.PromptFolderLibrary) (types.PromptFolderLibrary, error) {
		for position := range library.Folders {
			folder := &library.Folders[position]
			if folder.ID != folderID {
				continue
			}
			if slices.Contains(folder.PromptIDs, promptID) {
				return types.PromptFolderLibrary{}, storageInvalid("prompt folder "+folderID+" already contains prompt: "+promptID, nil)
			}
			folder.PromptIDs = append(folder.PromptIDs, promptID)
			folder.EntryOrder = append(folder.EntryOrder, promptEntryKey(promptEntryKindPrompt, promptID))
			return library, nil
		}
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+folderID, nil)
	})
}

// MovePromptToFolder 把一条提示词从一个收藏夹移动到另一个收藏夹：
// 先摘掉原夹索引，再追加到目标夹，两步在同一次变换里完成。
// fromFolderID 为空表示条目原来不在任何夹里（只追加）；toFolderID 为空表示移出到根级（只摘除）。
func (s *system) MovePromptToFolder(ctx context.Context, scope string, ownerID string, promptID string, fromFolderID string, toFolderID string) (types.PromptFolderLibrary, error) {
	promptID = strings.TrimSpace(promptID)
	fromFolderID = strings.TrimSpace(fromFolderID)
	toFolderID = strings.TrimSpace(toFolderID)
	if promptID == "" {
		return types.PromptFolderLibrary{}, storageInvalid("prompt id is required", nil)
	}
	if fromFolderID == toFolderID {
		return types.PromptFolderLibrary{}, storageInvalid("prompt already lives here: "+promptID, nil)
	}
	return s.mutatePromptFolders(ctx, scope, ownerID, func(library types.PromptFolderLibrary) (types.PromptFolderLibrary, error) {
		promptKey := promptEntryKey(promptEntryKindPrompt, promptID)
		if toFolderID != "" {
			found := false
			for position := range library.Folders {
				folder := &library.Folders[position]
				if folder.ID != toFolderID {
					continue
				}
				found = true
				if slices.Contains(folder.PromptIDs, promptID) {
					return types.PromptFolderLibrary{}, storageInvalid("prompt folder "+toFolderID+" already contains prompt: "+promptID, nil)
				}
			}
			if !found {
				return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+toFolderID, nil)
			}
		}
		if fromFolderID != "" {
			found := false
			for position := range library.Folders {
				folder := &library.Folders[position]
				if folder.ID != fromFolderID {
					continue
				}
				found = true
				if !slices.Contains(folder.PromptIDs, promptID) {
					return types.PromptFolderLibrary{}, storageInvalid("prompt folder "+fromFolderID+" does not contain prompt: "+promptID, nil)
				}
				remaining := make([]string, 0, len(folder.PromptIDs)-1)
				for _, id := range folder.PromptIDs {
					if id != promptID {
						remaining = append(remaining, id)
					}
				}
				folder.PromptIDs = remaining
				if next, removed := removeEntryKey(folder.EntryOrder, promptKey); removed {
					folder.EntryOrder = next
				}
			}
			if !found {
				return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+fromFolderID, nil)
			}
		}
		if toFolderID != "" {
			for position := range library.Folders {
				folder := &library.Folders[position]
				if folder.ID != toFolderID {
					continue
				}
				folder.PromptIDs = append(folder.PromptIDs, promptID)
				folder.EntryOrder = append(folder.EntryOrder, promptKey)
				break
			}
		}
		return library, nil
	})
}

// RemovePromptFromFolder 把一条提示词移出收藏夹：只摘索引与顺序，本体一条不动。
func (s *system) RemovePromptFromFolder(ctx context.Context, scope string, ownerID string, folderID string, promptID string) (types.PromptFolderLibrary, error) {
	folderID = strings.TrimSpace(folderID)
	promptID = strings.TrimSpace(promptID)
	if folderID == "" {
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder id is required", nil)
	}
	if promptID == "" {
		return types.PromptFolderLibrary{}, storageInvalid("prompt id is required", nil)
	}
	return s.mutatePromptFolders(ctx, scope, ownerID, func(library types.PromptFolderLibrary) (types.PromptFolderLibrary, error) {
		for position := range library.Folders {
			folder := &library.Folders[position]
			if folder.ID != folderID {
				continue
			}
			if !slices.Contains(folder.PromptIDs, promptID) {
				return types.PromptFolderLibrary{}, storageInvalid("prompt folder "+folderID+" does not contain prompt: "+promptID, nil)
			}
			remaining := make([]string, 0, len(folder.PromptIDs)-1)
			for _, id := range folder.PromptIDs {
				if id != promptID {
					remaining = append(remaining, id)
				}
			}
			folder.PromptIDs = remaining
			if next, removed := removeEntryKey(folder.EntryOrder, promptEntryKey(promptEntryKindPrompt, promptID)); removed {
				folder.EntryOrder = next
			}
			return library, nil
		}
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+folderID, nil)
	})
}

// mutatePromptFolders 是收藏夹定点动作的统一骨架：
// 同一把实体锁内读出当前库与本体库，交给动作变换，再走整库校验后落盘。
func (s *system) mutatePromptFolders(ctx context.Context, scope string, ownerID string, mutate func(types.PromptFolderLibrary) (types.PromptFolderLibrary, error)) (types.PromptFolderLibrary, error) {
	file, err := s.promptScopeFile(scope, ownerID, datapaths.RelPromptFoldersFile)
	if err != nil {
		return types.PromptFolderLibrary{}, err
	}
	promptsFile, _, err := s.paths.promptScopeFile(scope, ownerID, datapaths.RelPromptLibraryFile)
	if err != nil {
		return types.PromptFolderLibrary{}, err
	}

	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	previous, err := readPromptFolderLibrary(ctx, file)
	if err != nil {
		return types.PromptFolderLibrary{}, err
	}
	prompts, err := readPromptLibrary(ctx, promptsFile)
	if err != nil {
		return types.PromptFolderLibrary{}, err
	}
	// 定点动作在完整顺序上做增删改：缺省顺序先派生，避免追加一项得到半截排列。
	next, err := mutate(completePromptFolderLibrary(previous, prompts))
	if err != nil {
		return types.PromptFolderLibrary{}, err
	}
	normalized, err := normalizePromptFolderLibraryForSave(next, previous, prompts, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return types.PromptFolderLibrary{}, err
	}
	if err := writeJSON(ctx, file, normalized); err != nil {
		return types.PromptFolderLibrary{}, storageWriteFailed("failed to save prompt folders", err)
	}
	return normalized, nil
}

// locatePromptFolder 找到某个收藏夹在父级清单中的位置：parentID 为空表示根级。
func locatePromptFolder(library types.PromptFolderLibrary, folderID string) (string, int, bool) {
	for index, id := range library.RootFolderIDs {
		if id == folderID {
			return "", index, true
		}
	}
	for _, folder := range library.Folders {
		for index, id := range folder.FolderIDs {
			if id == folderID {
				return folder.ID, index, true
			}
		}
	}
	return "", 0, false
}

// spliceIDs 用 insert 替换 ids 中从 at 开始的一段落，返回新切片。
func spliceIDs(ids []string, at int, count int, insert ...string) []string {
	result := make([]string, 0, len(ids)-count+len(insert))
	result = append(result, ids[:at]...)
	result = append(result, insert...)
	result = append(result, ids[at+count:]...)
	return result
}

// sameIDSet 判定两份清单是否同一集合（顺序无关，数量一致）。
func sameIDSet(order []string, existing []string) bool {
	if len(order) != len(existing) {
		return false
	}
	counts := make(map[string]int, len(existing))
	for _, id := range existing {
		counts[id]++
	}
	for _, id := range order {
		if counts[id] == 0 {
			return false
		}
		counts[id]--
	}
	return true
}

// findPromptFolder 按 id 找收藏夹。
func findPromptFolder(library types.PromptFolderLibrary, folderID string) (types.PromptFolder, bool) {
	for _, folder := range library.Folders {
		if folder.ID == folderID {
			return folder, true
		}
	}
	return types.PromptFolder{}, false
}

// isPromptFolderDescendant 判定 candidate 是否是 ancestor 的后代（含自身）。
func isPromptFolderDescendant(library types.PromptFolderLibrary, ancestorID string, candidateID string) bool {
	byID := make(map[string]types.PromptFolder, len(library.Folders))
	for _, folder := range library.Folders {
		byID[folder.ID] = folder
	}
	var walk func(id string) bool
	walk = func(id string) bool {
		if id == candidateID {
			return true
		}
		folder, ok := byID[id]
		if !ok {
			return false
		}
		for _, childID := range folder.FolderIDs {
			if walk(childID) {
				return true
			}
		}
		return false
	}
	return ancestorID != "" && walk(ancestorID)
}

// folderKeys 把收藏夹 id 列表转成混排 key 列表。
func folderKeys(ids []string) []string {
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, promptEntryKey(promptEntryKindFolder, id))
	}
	return keys
}

// indexOfID 返回 id 在列表中的位置，不存在返回 -1。
func indexOfID(ids []string, id string) int {
	for index, item := range ids {
		if strings.TrimSpace(item) == id {
			return index
		}
	}
	return -1
}

// missingIDs 返回 candidates 中尚未出现在 existing 里的项（保持顺序）。
func missingIDs(candidates []string, existing []string) []string {
	missing := make([]string, 0, len(candidates))
	for _, id := range candidates {
		if indexOfID(existing, id) >= 0 {
			continue
		}
		missing = append(missing, id)
	}
	return missing
}
