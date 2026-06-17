package datastorage

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) LoadHookPromptLibrary(ctx context.Context) (types.HookPromptLibrary, error) {
	target := s.paths.hookPromptLibraryFile()
	if !dataFileExists(target) {
		return normalizeHookPromptLibrary(types.HookPromptLibrary{}), nil
	}
	library, err := readJSON[types.HookPromptLibrary](ctx, target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return normalizeHookPromptLibrary(types.HookPromptLibrary{}), nil
		}
		return types.HookPromptLibrary{}, err
	}
	return normalizeHookPromptLibrary(library), nil
}

func (s *system) SaveHookPromptLibrary(ctx context.Context, library types.HookPromptLibrary) (types.HookPromptLibrary, error) {
	library = normalizeHookPromptLibrary(library)
	if err := writeJSON(ctx, s.paths.hookPromptLibraryFile(), library); err != nil {
		return types.HookPromptLibrary{}, err
	}
	return library, nil
}

func normalizeHookPromptLibrary(library types.HookPromptLibrary) types.HookPromptLibrary {
	presets := make([]types.HookPromptPreset, 0, len(library.Presets))
	seen := map[string]struct{}{}
	for _, preset := range library.Presets {
		preset.ID = strings.TrimSpace(preset.ID)
		preset.Name = strings.TrimSpace(preset.Name)
		if preset.ID == "" || preset.Name == "" {
			continue
		}
		if _, ok := seen[preset.ID]; ok {
			continue
		}
		seen[preset.ID] = struct{}{}
		preset.Messages = normalizeHookPromptMessages(preset.Messages)
		presets = append(presets, preset)
	}
	return types.HookPromptLibrary{Presets: presets}
}

func normalizeHookPromptMessages(messages []types.HookPromptMessage) []types.HookPromptMessage {
	out := make([]types.HookPromptMessage, 0, len(messages))
	seen := map[string]struct{}{}
	for _, message := range messages {
		message.ID = strings.TrimSpace(message.ID)
		message.Position = strings.TrimSpace(message.Position)
		message.Role = types.HookPromptRoleForPosition(message.Position, message.Role)
		message.Content = strings.TrimSpace(message.Content)
		if message.ID == "" || message.Content == "" || !types.IsHookPromptPosition(message.Position) {
			continue
		}
		if _, ok := seen[message.ID]; ok {
			continue
		}
		seen[message.ID] = struct{}{}
		out = append(out, message)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Position != out[j].Position {
			return hookPromptPositionRank(out[i].Position) < hookPromptPositionRank(out[j].Position)
		}
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].ID < out[j].ID
	})
	for index := range out {
		out[index].Order = index
	}
	return out
}

func hookPromptPositionRank(position string) int {
	for index, item := range types.HookPromptPositions {
		if position == item {
			return index
		}
	}
	return len(types.HookPromptPositions)
}
