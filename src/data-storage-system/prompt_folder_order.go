package datastorage

import (
	"strings"

	"eucli-box/pkg/types"
)

// 收藏夹混排顺序：收藏夹与提示词条目同处一个列表。
// 顺序元素形如 folder:<id> 或 prompt:<id>；FolderIDs 与 PromptIDs 仍是索引事实，
// EntryOrder 只负责显示顺序，两者必须描述同一集合。

const (
	promptEntryKindFolder = "folder"
	promptEntryKindPrompt = "prompt"
)

func promptEntryKey(kind string, id string) string {
	return kind + ":" + id
}

func parsePromptEntryKey(key string) (string, string, bool) {
	kind, id, found := strings.Cut(key, ":")
	if !found {
		return "", "", false
	}
	kind = strings.TrimSpace(kind)
	id = strings.TrimSpace(id)
	if id == "" {
		return "", "", false
	}
	if kind != promptEntryKindFolder && kind != promptEntryKindPrompt {
		return "", "", false
	}
	return kind, id, true
}

// deriveEntryOrder 给出缺省的混排顺序：收藏夹在前、条目在后。
func deriveEntryOrder(folderIDs []string, promptIDs []string) []string {
	order := make([]string, 0, len(folderIDs)+len(promptIDs))
	for _, id := range folderIDs {
		order = append(order, promptEntryKey(promptEntryKindFolder, id))
	}
	for _, id := range promptIDs {
		order = append(order, promptEntryKey(promptEntryKindPrompt, id))
	}
	return order
}

// normalizeEntryOrder 校验一份夹内混排顺序：必须是本层收藏夹与条目的完整排列。
// 空顺序按缺省派生；多一项、少一项、重复项、非法前缀一律拒绝。
func normalizeEntryOrder(raw []string, folderIDs []string, promptIDs []string, label string) ([]string, error) {
	if len(raw) == 0 {
		return deriveEntryOrder(folderIDs, promptIDs), nil
	}
	expected := make(map[string]struct{}, len(folderIDs)+len(promptIDs))
	for _, id := range folderIDs {
		expected[promptEntryKey(promptEntryKindFolder, id)] = struct{}{}
	}
	for _, id := range promptIDs {
		expected[promptEntryKey(promptEntryKindPrompt, id)] = struct{}{}
	}
	order := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		key := strings.TrimSpace(item)
		if _, _, ok := parsePromptEntryKey(key); !ok {
			return nil, storageInvalid(label+" contains an invalid entry: "+key, nil)
		}
		if _, exists := expected[key]; !exists {
			return nil, storageInvalid(label+" references an entry outside this level: "+key, nil)
		}
		if _, duplicated := seen[key]; duplicated {
			return nil, storageInvalid(label+" contains duplicate entry: "+key, nil)
		}
		seen[key] = struct{}{}
		order = append(order, key)
	}
	if len(order) != len(expected) {
		return nil, storageInvalid(label+" must list every entry of this level", nil)
	}
	return order, nil
}

// rootLayerPromptIDs 给出根层级直属的提示词：没有被任何收藏夹收录的那些。
// 根层级是收藏夹树的最外层，只承载自己直接持有的内容：
// 收进收藏夹就从根层离开，摘下收藏夹就重新回到根层，不做全库兜底。
func rootLayerPromptIDs(library types.PromptFolderLibrary, promptLibrary types.PromptLibrary) []string {
	filed := make(map[string]struct{})
	for _, folder := range library.Folders {
		for _, id := range folder.PromptIDs {
			filed[strings.TrimSpace(id)] = struct{}{}
		}
	}
	ids := make([]string, 0, len(promptLibrary.Prompts))
	for _, prompt := range promptLibrary.Prompts {
		if _, ok := filed[prompt.ID]; ok {
			continue
		}
		ids = append(ids, prompt.ID)
	}
	return ids
}

// deriveRootEntryOrder 给出根级缺省顺序：根收藏夹在前、根层直属提示词在后。
func deriveRootEntryOrder(rootFolderIDs []string, rootPromptIDs []string) []string {
	order := make([]string, 0, len(rootFolderIDs)+len(rootPromptIDs))
	for _, id := range rootFolderIDs {
		order = append(order, promptEntryKey(promptEntryKindFolder, id))
	}
	for _, id := range rootPromptIDs {
		order = append(order, promptEntryKey(promptEntryKindPrompt, id))
	}
	return order
}

// normalizeRootEntryOrder 校验根级混排顺序：必须覆盖根收藏夹与根层直属提示词。
func normalizeRootEntryOrder(raw []string, rootFolderIDs []string, rootPromptIDs []string, label string) ([]string, error) {
	if len(raw) == 0 {
		return deriveRootEntryOrder(rootFolderIDs, rootPromptIDs), nil
	}
	return normalizeEntryOrder(raw, rootFolderIDs, rootPromptIDs, label)
}

// removeEntryKey 从顺序里摘掉一项；返回新顺序与是否发生变化。
func removeEntryKey(order []string, key string) ([]string, bool) {
	next := make([]string, 0, len(order))
	removed := false
	for _, item := range order {
		if strings.TrimSpace(item) == key {
			removed = true
			continue
		}
		next = append(next, item)
	}
	if !removed {
		return order, false
	}
	return next, true
}

// replaceEntryKey 用 insert 替换顺序中的某一项（保持位置）；返回新顺序与是否发生变化。
// 删除收藏夹时用它把被删夹的位置让给上移的子收藏夹。
func replaceEntryKey(order []string, key string, insert []string) ([]string, bool) {
	next := make([]string, 0, len(order)+len(insert))
	replaced := false
	for _, item := range order {
		if strings.TrimSpace(item) == key {
			next = append(next, insert...)
			replaced = true
			continue
		}
		next = append(next, item)
	}
	if !replaced {
		return order, false
	}
	return next, true
}

// validateStoredEntryOrder 读盘时只做格式与去重校验：本层条目集合要另一份数据才能判定。
func validateStoredEntryOrder(order []string, label string) error {
	seen := make(map[string]struct{}, len(order))
	for _, item := range order {
		key := strings.TrimSpace(item)
		if _, _, ok := parsePromptEntryKey(key); !ok {
			return storageInvalid(label+" contains an invalid entry: "+key, nil)
		}
		if _, duplicated := seen[key]; duplicated {
			return storageInvalid(label+" contains duplicate entry: "+key, nil)
		}
		seen[key] = struct{}{}
	}
	return nil
}
