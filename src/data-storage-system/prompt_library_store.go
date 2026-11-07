package datastorage

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

// 提示词仓库：全局、角色、工作区三个作用域共用同一套读、写、删动作。
// 作用域只决定文件落点（由 paths.promptLibraryTarget 单点解析），
// 不改变条目内容语义，也不在作用域之间做继承或覆盖。

// LoadPromptLibrary 读取某个作用域的提示词仓库；文件尚不存在时返回空库。
func (s *system) LoadPromptLibrary(ctx context.Context, scope string, ownerID string) (types.PromptLibrary, error) {
	target, err := s.promptLibraryTarget(scope, ownerID)
	if err != nil {
		return types.PromptLibrary{}, err
	}
	return readPromptLibrary(ctx, target.file)
}

// SavePromptLibrary 整库保存某个作用域的提示词仓库。
// 条目时间由存储维护：新条目补创建时间，内容或备注真的变了才刷新更新时间。
func (s *system) SavePromptLibrary(ctx context.Context, scope string, ownerID string, library types.PromptLibrary) (types.PromptLibrary, error) {
	target, err := s.promptLibraryTarget(scope, ownerID)
	if err != nil {
		return types.PromptLibrary{}, err
	}

	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	previous, err := readPromptLibrary(ctx, target.file)
	if err != nil {
		return types.PromptLibrary{}, err
	}
	normalized, err := normalizePromptLibraryForSave(library, previous, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return types.PromptLibrary{}, err
	}
	if err := writeJSON(ctx, target.file, normalized); err != nil {
		return types.PromptLibrary{}, storageWriteFailed("failed to save prompt library", err)
	}
	return normalized, nil
}

// DeletePrompt 删除某条提示词：读、改、写在同一把实体锁内完成。
// 条目不存在时明确报错，不静默成功。
func (s *system) DeletePrompt(ctx context.Context, scope string, ownerID string, promptID string) (types.PromptLibrary, error) {
	promptID = strings.TrimSpace(promptID)
	if promptID == "" {
		return types.PromptLibrary{}, storageInvalid("prompt id is required", nil)
	}
	target, err := s.promptLibraryTarget(scope, ownerID)
	if err != nil {
		return types.PromptLibrary{}, err
	}

	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	library, err := readPromptLibrary(ctx, target.file)
	if err != nil {
		return types.PromptLibrary{}, err
	}
	remaining := make([]types.Prompt, 0, len(library.Prompts))
	found := false
	for _, prompt := range library.Prompts {
		if prompt.ID == promptID {
			found = true
			continue
		}
		remaining = append(remaining, prompt)
	}
	if !found {
		return types.PromptLibrary{}, storageInvalid("prompt does not exist: "+promptID, nil)
	}
	updated := types.PromptLibrary{Prompts: remaining}
	if err := writeJSON(ctx, target.file, updated); err != nil {
		return types.PromptLibrary{}, storageWriteFailed("failed to save prompt library", err)
	}
	return updated, nil
}

// promptLibraryTarget 解析落点并确认归属实体存在：
// 写入或读取一个不存在的角色/工作区的提示词仓库，属于调用方给错了归属，
// 如实报错，不在数据区凭空生成实体目录。
func (s *system) promptLibraryTarget(scope string, ownerID string) (promptLibraryLocation, error) {
	target, err := s.paths.promptLibraryLocation(scope, ownerID)
	if err != nil {
		return promptLibraryLocation{}, err
	}
	if target.ownerDir != "" && !directoryExists(target.ownerDir) {
		return promptLibraryLocation{}, storageInvalid("prompt library owner does not exist for scope "+scope, nil)
	}
	return target, nil
}

// readPromptLibrary 读取库文件并校验其内容。
// 文件不存在视为空库；文件在手但内容不合规时如实报错，不静默清洗。
func readPromptLibrary(ctx context.Context, target string) (types.PromptLibrary, error) {
	if !dataFileExists(target) {
		return types.PromptLibrary{Prompts: []types.Prompt{}}, nil
	}
	library, err := readJSON[types.PromptLibrary](ctx, target)
	if err != nil {
		return types.PromptLibrary{}, storageReadFailed("failed to read prompt library", err)
	}
	if err := validateStoredPromptLibrary(library); err != nil {
		return types.PromptLibrary{}, err
	}
	if library.Prompts == nil {
		library.Prompts = []types.Prompt{}
	}
	return library, nil
}

// validateStoredPromptLibrary 判定磁盘上的库内容是否合规。
func validateStoredPromptLibrary(library types.PromptLibrary) error {
	seen := make(map[string]struct{}, len(library.Prompts))
	for _, prompt := range library.Prompts {
		if strings.TrimSpace(prompt.ID) == "" {
			return storageInvalid("stored prompt library contains an entry without id", nil)
		}
		if _, ok := seen[prompt.ID]; ok {
			return storageInvalid("stored prompt library contains duplicate prompt id: "+prompt.ID, nil)
		}
		seen[prompt.ID] = struct{}{}
		if strings.TrimSpace(prompt.Name) == "" {
			return storageInvalid("stored prompt library entry "+prompt.ID+" has no name", nil)
		}
		if strings.TrimSpace(prompt.Content) == "" {
			return storageInvalid("stored prompt library entry "+prompt.ID+" has no content", nil)
		}
	}
	return nil
}

// normalizePromptLibraryForSave 校验并规范化待保存的库。
// 时间字段一律由存储给出，调用方携带的时间不采信。
func normalizePromptLibraryForSave(library types.PromptLibrary, previous types.PromptLibrary, now string) (types.PromptLibrary, error) {
	previousByID := make(map[string]types.Prompt, len(previous.Prompts))
	for _, prompt := range previous.Prompts {
		previousByID[prompt.ID] = prompt
	}

	seen := make(map[string]struct{}, len(library.Prompts))
	prompts := make([]types.Prompt, 0, len(library.Prompts))
	for _, prompt := range library.Prompts {
		prompt.ID = strings.TrimSpace(prompt.ID)
		prompt.Name = strings.TrimSpace(prompt.Name)
		prompt.Description = strings.TrimSpace(prompt.Description)
		if prompt.ID == "" {
			return types.PromptLibrary{}, storageInvalid("prompt id is required", nil)
		}
		if prompt.Name == "" {
			return types.PromptLibrary{}, storageInvalid("prompt name is required: "+prompt.ID, nil)
		}
		if strings.TrimSpace(prompt.Content) == "" {
			return types.PromptLibrary{}, storageInvalid("prompt content is required: "+prompt.ID, nil)
		}
		if _, ok := seen[prompt.ID]; ok {
			return types.PromptLibrary{}, storageInvalid("duplicate prompt id: "+prompt.ID, nil)
		}
		seen[prompt.ID] = struct{}{}

		existing, exists := previousByID[prompt.ID]
		switch {
		case !exists:
			prompt.CreatedAt = now
			prompt.UpdatedAt = now
		case promptContentChanged(existing, prompt):
			prompt.CreatedAt = existing.CreatedAt
			prompt.UpdatedAt = now
		default:
			prompt.CreatedAt = existing.CreatedAt
			prompt.UpdatedAt = existing.UpdatedAt
		}
		prompts = append(prompts, prompt)
	}
	return types.PromptLibrary{Prompts: prompts}, nil
}

// promptContentChanged 判定条目是否真的变化：备注也算变化的一部分。
func promptContentChanged(previous types.Prompt, current types.Prompt) bool {
	return previous.Name != current.Name ||
		previous.Content != current.Content ||
		previous.Description != current.Description
}
