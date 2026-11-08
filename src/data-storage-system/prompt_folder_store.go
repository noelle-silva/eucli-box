package datastorage

import (
	"context"
	"slices"
	"strings"
	"time"

	"eucli-box/pkg/datapaths"
	"eucli-box/pkg/types"
)

// 提示词收藏夹：叠在提示词本体之上的索引层。
// 收藏夹只保存树结构与索引的提示词 id，不复制内容；
// 顺序由 RootFolderIDs / FolderIDs / PromptIDs 的数组顺序表达。
// 三个作用域与提示词本体共用同一套落点解析，各存各的文件。

// LoadPromptFolders 读取某个作用域的收藏夹；文件尚不存在时返回空库。
func (s *system) LoadPromptFolders(ctx context.Context, scope string, ownerID string) (types.PromptFolderLibrary, error) {
	file, err := s.promptScopeFile(scope, ownerID, datapaths.RelPromptFoldersFile)
	if err != nil {
		return types.PromptFolderLibrary{}, err
	}
	return readPromptFolderLibrary(ctx, file)
}

// SavePromptFolders 整库保存某个作用域的收藏夹。
// 被索引的提示词必须在本作用域本体库中存在；悬空索引如实报错。
func (s *system) SavePromptFolders(ctx context.Context, scope string, ownerID string, library types.PromptFolderLibrary) (types.PromptFolderLibrary, error) {
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
	promptLibrary, err := readPromptLibrary(ctx, promptsFile)
	if err != nil {
		return types.PromptFolderLibrary{}, err
	}
	normalized, err := normalizePromptFolderLibraryForSave(library, previous, promptLibrary, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return types.PromptFolderLibrary{}, err
	}
	if err := writeJSON(ctx, file, normalized); err != nil {
		return types.PromptFolderLibrary{}, storageWriteFailed("failed to save prompt folders", err)
	}
	return normalized, nil
}

// readPromptFolderLibrary 读取收藏夹文件并校验其结构。
// 文件不存在视为空库；文件在手但结构不合规时如实报错，不静默清洗。
func readPromptFolderLibrary(ctx context.Context, target string) (types.PromptFolderLibrary, error) {
	if !dataFileExists(target) {
		return emptyPromptFolderLibrary(), nil
	}
	library, err := readJSON[types.PromptFolderLibrary](ctx, target)
	if err != nil {
		return types.PromptFolderLibrary{}, storageReadFailed("failed to read prompt folders", err)
	}
	if err := validateStoredPromptFolderLibrary(library); err != nil {
		return types.PromptFolderLibrary{}, err
	}
	return normalizeEmptyCollections(library), nil
}

func emptyPromptFolderLibrary() types.PromptFolderLibrary {
	return types.PromptFolderLibrary{RootFolderIDs: []string{}, Folders: []types.PromptFolder{}}
}

// completePromptFolderLibrary 让定点动作在完整顺序上做增删改。
// 根级顺序跟着提示词本体走：本体新增或删除条目后，根顺序同步补齐或摘除，
// 这是「根级集合＝根收藏夹＋全部提示词」这条规则的自动维护，不是掩盖错误。
// 夹内顺序只补「完全缺失」的层；写了一半的视为数据损坏，交给保存校验如实报错。
func completePromptFolderLibrary(library types.PromptFolderLibrary, prompts types.PromptLibrary) types.PromptFolderLibrary {
	library.RootEntryOrder = syncRootEntryOrder(library.RootEntryOrder, library.RootFolderIDs, prompts)
	for index := range library.Folders {
		if len(library.Folders[index].EntryOrder) == 0 {
			library.Folders[index].EntryOrder = deriveEntryOrder(library.Folders[index].FolderIDs, library.Folders[index].PromptIDs)
		}
	}
	return library
}

// syncRootEntryOrder 把根级顺序同步到「根收藏夹＋全部提示词」：
// 保留仍有效的项与原顺序，丢掉已不存在的项，缺失项按「收藏夹在前、条目在后」补到末尾。
func syncRootEntryOrder(order []string, rootFolderIDs []string, promptLibrary types.PromptLibrary) []string {
	expected := make(map[string]struct{}, len(rootFolderIDs)+len(promptLibrary.Prompts))
	for _, id := range rootFolderIDs {
		expected[promptEntryKey(promptEntryKindFolder, id)] = struct{}{}
	}
	for _, prompt := range promptLibrary.Prompts {
		expected[promptEntryKey(promptEntryKindPrompt, prompt.ID)] = struct{}{}
	}

	seen := make(map[string]struct{}, len(expected))
	order = append([]string{}, order...)
	result := make([]string, 0, len(expected))
	for _, item := range order {
		key := strings.TrimSpace(item)
		if _, ok := expected[key]; !ok {
			continue
		}
		if _, duplicated := seen[key]; duplicated {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	for _, id := range rootFolderIDs {
		key := promptEntryKey(promptEntryKindFolder, id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	for _, prompt := range promptLibrary.Prompts {
		key := promptEntryKey(promptEntryKindPrompt, prompt.ID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result
}

// normalizeEmptyCollections 把空集合归位成空切片，避免 JSON 里出现 null。
func normalizeEmptyCollections(library types.PromptFolderLibrary) types.PromptFolderLibrary {
	if library.RootFolderIDs == nil {
		library.RootFolderIDs = []string{}
	}
	if library.RootEntryOrder == nil {
		library.RootEntryOrder = []string{}
	}
	if library.Folders == nil {
		library.Folders = []types.PromptFolder{}
	}
	for index := range library.Folders {
		if library.Folders[index].FolderIDs == nil {
			library.Folders[index].FolderIDs = []string{}
		}
		if library.Folders[index].PromptIDs == nil {
			library.Folders[index].PromptIDs = []string{}
		}
		if library.Folders[index].EntryOrder == nil {
			library.Folders[index].EntryOrder = []string{}
		}
	}
	return library
}

// validateStoredPromptFolderLibrary 判定磁盘上的收藏夹结构是否合规。
func validateStoredPromptFolderLibrary(library types.PromptFolderLibrary) error {
	byID, err := indexPromptFolders(library.Folders)
	if err != nil {
		return err
	}
	if err := validateStoredEntryOrder(library.RootEntryOrder, "stored root entry order"); err != nil {
		return err
	}
	for _, folder := range library.Folders {
		if err := validateStoredEntryOrder(folder.EntryOrder, "stored prompt folder "+folder.ID+" entry order"); err != nil {
			return err
		}
	}
	return validatePromptFolderTree(library, byID)
}

// normalizePromptFolderLibraryForSave 校验并规范化待保存的收藏夹。
// 时间字段一律由存储给出；结构化问题一律拒绝，不做静默修正。
func normalizePromptFolderLibraryForSave(library types.PromptFolderLibrary, previous types.PromptFolderLibrary, prompts types.PromptLibrary, now string) (types.PromptFolderLibrary, error) {
	previousByID := make(map[string]types.PromptFolder, len(previous.Folders))
	for _, folder := range previous.Folders {
		previousByID[folder.ID] = folder
	}
	promptIDs := make(map[string]struct{}, len(prompts.Prompts))
	for _, prompt := range prompts.Prompts {
		promptIDs[prompt.ID] = struct{}{}
	}

	rootIDs, err := normalizeFolderIDList(library.RootFolderIDs, "root folder")
	if err != nil {
		return types.PromptFolderLibrary{}, err
	}
	folders := make([]types.PromptFolder, 0, len(library.Folders))
	seen := make(map[string]struct{}, len(library.Folders))
	for _, folder := range library.Folders {
		folder.ID = strings.TrimSpace(folder.ID)
		folder.Name = strings.TrimSpace(folder.Name)
		if folder.ID == "" {
			return types.PromptFolderLibrary{}, storageInvalid("prompt folder id is required", nil)
		}
		if folder.Name == "" {
			return types.PromptFolderLibrary{}, storageInvalid("prompt folder name is required: "+folder.ID, nil)
		}
		if _, ok := seen[folder.ID]; ok {
			return types.PromptFolderLibrary{}, storageInvalid("duplicate prompt folder id: "+folder.ID, nil)
		}
		seen[folder.ID] = struct{}{}

		folder.FolderIDs, err = normalizeFolderIDList(folder.FolderIDs, "child folder")
		if err != nil {
			return types.PromptFolderLibrary{}, err
		}
		folder.PromptIDs, err = normalizePromptIndexList(folder.PromptIDs, promptIDs, folder.ID)
		if err != nil {
			return types.PromptFolderLibrary{}, err
		}
		folder.EntryOrder, err = normalizeEntryOrder(folder.EntryOrder, folder.FolderIDs, folder.PromptIDs, "prompt folder "+folder.ID+" entry order")
		if err != nil {
			return types.PromptFolderLibrary{}, err
		}

		existing, exists := previousByID[folder.ID]
		switch {
		case !exists:
			folder.CreatedAt = now
			folder.UpdatedAt = now
		case promptFolderChanged(existing, folder):
			folder.CreatedAt = existing.CreatedAt
			folder.UpdatedAt = now
		default:
			folder.CreatedAt = existing.CreatedAt
			folder.UpdatedAt = existing.UpdatedAt
		}
		folders = append(folders, folder)
	}

	rootOrder, err := normalizeRootEntryOrder(library.RootEntryOrder, rootIDs, prompts, "root entry order")
	if err != nil {
		return types.PromptFolderLibrary{}, err
	}
	normalized := types.PromptFolderLibrary{RootFolderIDs: rootIDs, RootEntryOrder: rootOrder, Folders: folders}
	byID := make(map[string]types.PromptFolder, len(folders))
	for _, folder := range folders {
		byID[folder.ID] = folder
	}
	if err := validatePromptFolderTree(normalized, byID); err != nil {
		return types.PromptFolderLibrary{}, err
	}
	return normalized, nil
}

// indexPromptFolders 建立 id 索引并拒绝重复 id。
func indexPromptFolders(folders []types.PromptFolder) (map[string]types.PromptFolder, error) {
	byID := make(map[string]types.PromptFolder, len(folders))
	for _, folder := range folders {
		if strings.TrimSpace(folder.ID) == "" {
			return nil, storageInvalid("stored prompt folders contain an entry without id", nil)
		}
		if _, ok := byID[folder.ID]; ok {
			return nil, storageInvalid("stored prompt folders contain duplicate id: "+folder.ID, nil)
		}
		byID[folder.ID] = folder
	}
	return byID, nil
}

// validatePromptFolderTree 校验收藏夹之间的引用关系。
// 收藏夹是索引，同一个收藏夹可以被多个上级收录，因此不要求唯一父级；
// 引用必须存在、不能自引用、不能成环。
func validatePromptFolderTree(library types.PromptFolderLibrary, byID map[string]types.PromptFolder) error {
	for _, rootID := range library.RootFolderIDs {
		rootID = strings.TrimSpace(rootID)
		if _, ok := byID[rootID]; !ok {
			return storageInvalid("prompt folder references missing folder: "+rootID, nil)
		}
	}
	for _, folder := range library.Folders {
		for _, childID := range folder.FolderIDs {
			childID = strings.TrimSpace(childID)
			if childID == strings.TrimSpace(folder.ID) {
				return storageInvalid("prompt folder cannot contain itself: "+folder.ID, nil)
			}
			if _, ok := byID[childID]; !ok {
				return storageInvalid("prompt folder references missing folder: "+childID, nil)
			}
		}
	}

	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := make(map[string]int, len(byID))
	var walk func(id string) error
	walk = func(id string) error {
		switch state[id] {
		case visiting:
			return storageInvalid("prompt folder tree contains a cycle at: "+id, nil)
		case done:
			return nil
		}
		state[id] = visiting
		folder, ok := byID[id]
		if !ok {
			return storageInvalid("prompt folder is missing: "+id, nil)
		}
		for _, childID := range folder.FolderIDs {
			if err := walk(strings.TrimSpace(childID)); err != nil {
				return err
			}
		}
		state[id] = done
		return nil
	}
	for id := range byID {
		if err := walk(id); err != nil {
			return err
		}
	}
	return nil
}

// normalizeFolderIDList 规范化收藏夹 id 列表：去空白、拒绝空值与重复。
func normalizeFolderIDList(ids []string, label string) ([]string, error) {
	normalized := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, storageInvalid(label + " id is required", nil)
		}
		if _, ok := seen[id]; ok {
			return nil, storageInvalid(label + " id is duplicated: "+id, nil)
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	return normalized, nil
}

// normalizePromptIndexList 规范化索引列表：同夹内不许重复，且必须指向本作用域存在的提示词。
func normalizePromptIndexList(ids []string, promptIDs map[string]struct{}, folderID string) ([]string, error) {
	normalized := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, storageInvalid("prompt folder "+folderID+" contains an empty prompt reference", nil)
		}
		if _, ok := seen[id]; ok {
			return nil, storageInvalid("prompt folder "+folderID+" indexes prompt more than once: "+id, nil)
		}
		seen[id] = struct{}{}
		if _, ok := promptIDs[id]; !ok {
			return nil, storageInvalid("prompt folder "+folderID+" indexes missing prompt: "+id, nil)
		}
		normalized = append(normalized, id)
	}
	return normalized, nil
}

// promptFolderChanged 判定收藏夹是否真的变化：名称、子级顺序与索引顺序都算。
func promptFolderChanged(previous types.PromptFolder, current types.PromptFolder) bool {
	return previous.Name != current.Name ||
		!slices.Equal(previous.FolderIDs, current.FolderIDs) ||
		!slices.Equal(previous.PromptIDs, current.PromptIDs)
}

// detachPromptFromFolders 从所有收藏夹摘掉指向某个提示词的索引。
// 其余条目的相对顺序保持不变，受影响的收藏夹刷新更新时间。
func detachPromptFromFolders(library types.PromptFolderLibrary, promptID string, now string) (types.PromptFolderLibrary, bool) {
	key := promptEntryKey(promptEntryKindPrompt, promptID)
	changed := false
	if next, removed := removeEntryKey(library.RootEntryOrder, key); removed {
		library.RootEntryOrder = next
		changed = true
	}
	folders := make([]types.PromptFolder, 0, len(library.Folders))
	for _, folder := range library.Folders {
		if !slices.Contains(folder.PromptIDs, promptID) {
			folders = append(folders, folder)
			continue
		}
		remaining := make([]string, 0, len(folder.PromptIDs))
		for _, id := range folder.PromptIDs {
			if id != promptID {
				remaining = append(remaining, id)
			}
		}
		folder.PromptIDs = remaining
		if next, removed := removeEntryKey(folder.EntryOrder, key); removed {
			folder.EntryOrder = next
		}
		folder.UpdatedAt = now
		changed = true
		folders = append(folders, folder)
	}
	if !changed {
		return library, false
	}
	library.Folders = folders
	return library, true
}
