package datastorage

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/datapaths"
	"eucli-box/pkg/types"
)

// 收藏夹的定点动作：删除、重命名、排序。
// 三者都只改收藏夹索引，不碰提示词本体；每次动作读—改—写走同一把实体锁，
// 保存前复用整库校验，结构问题一律拒绝。

// DeletePromptFolder 删除一个收藏夹：子收藏夹按原顺序上移到被删夹原来的位置，
// 被删夹里的书签一并消失，提示词本体不受影响。
func (s *system) DeletePromptFolder(ctx context.Context, scope string, ownerID string, folderID string) (types.PromptFolderLibrary, error) {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder id is required", nil)
	}
	return s.mutatePromptFolders(ctx, scope, ownerID, func(library types.PromptFolderLibrary) (types.PromptFolderLibrary, error) {
		var target types.PromptFolder
		found := false
		for _, folder := range library.Folders {
			if folder.ID == folderID {
				target = folder
				found = true
				break
			}
		}
		if !found {
			return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+folderID, nil)
		}
		parentID, index, ok := locatePromptFolder(library, folderID)
		if !ok {
			return types.PromptFolderLibrary{}, storageInvalid("prompt folder has no parent: "+folderID, nil)
		}

		if parentID == "" {
			library.RootFolderIDs = spliceIDs(library.RootFolderIDs, index, 1, target.FolderIDs...)
		} else {
			for position := range library.Folders {
				if library.Folders[position].ID == parentID {
					library.Folders[position].FolderIDs = spliceIDs(library.Folders[position].FolderIDs, index, 1, target.FolderIDs...)
					break
				}
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

// ReorderPromptFolders 只重排同一父级下的收藏夹顺序：
// 提交的清单必须与现有兄弟完全一致，少一个或多一个都拒绝，动作只换位置。
func (s *system) ReorderPromptFolders(ctx context.Context, scope string, ownerID string, parentID string, folderIDs []string) (types.PromptFolderLibrary, error) {
	parentID = strings.TrimSpace(parentID)
	order, err := normalizeFolderIDList(folderIDs, "folder order")
	if err != nil {
		return types.PromptFolderLibrary{}, err
	}
	return s.mutatePromptFolders(ctx, scope, ownerID, func(library types.PromptFolderLibrary) (types.PromptFolderLibrary, error) {
		if parentID == "" {
			if !sameIDSet(order, library.RootFolderIDs) {
				return types.PromptFolderLibrary{}, storageInvalid("folder order must contain exactly the current root folders", nil)
			}
			library.RootFolderIDs = order
			return library, nil
		}
		for position := range library.Folders {
			if library.Folders[position].ID != parentID {
				continue
			}
			if !sameIDSet(order, library.Folders[position].FolderIDs) {
				return types.PromptFolderLibrary{}, storageInvalid("folder order must contain exactly the current child folders of: "+parentID, nil)
			}
			library.Folders[position].FolderIDs = order
			return library, nil
		}
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder does not exist: "+parentID, nil)
	})
}

// ReorderFolderPrompts 只重排某个收藏夹内的条目顺序：
// 提交的清单必须与现有索引完全一致，只换位置，不增不减。
func (s *system) ReorderFolderPrompts(ctx context.Context, scope string, ownerID string, folderID string, promptIDs []string) (types.PromptFolderLibrary, error) {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return types.PromptFolderLibrary{}, storageInvalid("prompt folder id is required", nil)
	}
	order, err := normalizeFolderIDList(promptIDs, "prompt order")
	if err != nil {
		return types.PromptFolderLibrary{}, err
	}
	return s.mutatePromptFolders(ctx, scope, ownerID, func(library types.PromptFolderLibrary) (types.PromptFolderLibrary, error) {
		for position := range library.Folders {
			if library.Folders[position].ID != folderID {
				continue
			}
			if !sameIDSet(order, library.Folders[position].PromptIDs) {
				return types.PromptFolderLibrary{}, storageInvalid("prompt order must contain exactly the current prompts of: "+folderID, nil)
			}
			library.Folders[position].PromptIDs = order
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
	next, err := mutate(previous)
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
