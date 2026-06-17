package agentruntime

import (
	"context"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) applyHookPromptPreset(ctx context.Context, record *runRecord, messages []types.PromptMessage, latestUserIndex int) ([]types.PromptMessage, error) {
	presetID := types.HookPromptPresetIDFromSessionMetadata(record.session.Metadata)
	if presetID == "" {
		return messages, nil
	}
	library, err := s.storage.LoadHookPromptLibrary(ctx)
	if err != nil {
		return nil, runtimeStorageFailed("failed to load hook prompt library", err)
	}
	preset, ok := hookPromptPresetByID(library, presetID)
	if !ok {
		return nil, runtimeInvalid("selected hook prompt preset was not found", nil)
	}
	groups := groupHookPromptMessages(preset.Messages)
	prepared := buildHookPromptMessages(messages, latestUserIndex, groups)
	return reindexPromptMessages(prepared), nil
}

func buildHookPromptMessages(messages []types.PromptMessage, latestUserIndex int, groups map[string][]types.HookPromptMessage) []types.PromptMessage {
	base := append([]types.PromptMessage(nil), messages...)
	base = applyInsideUserHookPrompts(base, latestUserIndex, groups)
	latestIndex := len(base) - 1
	out := make([]types.PromptMessage, 0, len(base)+hookPromptIndependentMessageCount(groups))
	out = append(out, hookPromptMessagesToPrompts(groups[types.HookPromptPositionSessionTop])...)
	for index, message := range base {
		if index == latestUserIndex {
			out = append(out, hookPromptMessagesToPrompts(groups[types.HookPromptPositionBeforeUser])...)
		}
		if index == latestIndex {
			out = append(out, hookPromptMessagesToPrompts(groups[types.HookPromptPositionBeforeLatest])...)
		}
		out = append(out, message)
		if index == latestUserIndex {
			out = append(out, hookPromptMessagesToPrompts(groups[types.HookPromptPositionAfterUser])...)
		}
		if index == latestIndex {
			out = append(out, hookPromptMessagesToPrompts(groups[types.HookPromptPositionAfterLatest])...)
		}
	}
	if len(base) == 0 {
		out = append(out, hookPromptMessagesToPrompts(groups[types.HookPromptPositionBeforeLatest])...)
		out = append(out, hookPromptMessagesToPrompts(groups[types.HookPromptPositionAfterLatest])...)
	}
	return out
}

func hookPromptPresetByID(library types.HookPromptLibrary, presetID string) (types.HookPromptPreset, bool) {
	presetID = strings.TrimSpace(presetID)
	for _, preset := range library.Presets {
		if strings.TrimSpace(preset.ID) == presetID {
			return preset, true
		}
	}
	return types.HookPromptPreset{}, false
}

func groupHookPromptMessages(messages []types.HookPromptMessage) map[string][]types.HookPromptMessage {
	groups := map[string][]types.HookPromptMessage{}
	for _, message := range messages {
		message.ID = strings.TrimSpace(message.ID)
		message.Position = strings.TrimSpace(message.Position)
		message.Content = strings.TrimSpace(message.Content)
		message.Role = types.HookPromptRoleForPosition(message.Position, message.Role)
		if message.ID == "" || message.Content == "" || !types.IsHookPromptPosition(message.Position) {
			continue
		}
		groups[message.Position] = append(groups[message.Position], message)
	}
	for position := range groups {
		sort.SliceStable(groups[position], func(i, j int) bool {
			if groups[position][i].Order != groups[position][j].Order {
				return groups[position][i].Order < groups[position][j].Order
			}
			return groups[position][i].ID < groups[position][j].ID
		})
	}
	return groups
}

func applyInsideUserHookPrompts(messages []types.PromptMessage, latestUserIndex int, groups map[string][]types.HookPromptMessage) []types.PromptMessage {
	if latestUserIndex < 0 || latestUserIndex >= len(messages) {
		return messages
	}
	top := hookPromptContents(groups[types.HookPromptPositionInsideUserTop])
	bottom := hookPromptContents(groups[types.HookPromptPositionInsideUserBottom])
	if len(top) == 0 && len(bottom) == 0 {
		return messages
	}
	message := messages[latestUserIndex]
	blocks := make([]string, 0, len(top)+1+len(bottom))
	blocks = append(blocks, top...)
	if strings.TrimSpace(message.Content) != "" {
		blocks = append(blocks, strings.TrimSpace(message.Content))
	}
	blocks = append(blocks, bottom...)
	message.Content = strings.Join(blocks, "\n\n")
	messages[latestUserIndex] = message
	return messages
}

func hookPromptContents(messages []types.HookPromptMessage) []string {
	contents := make([]string, 0, len(messages))
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content != "" {
			contents = append(contents, content)
		}
	}
	return contents
}

func hookPromptMessagesToPrompts(messages []types.HookPromptMessage) []types.PromptMessage {
	prompts := make([]types.PromptMessage, 0, len(messages))
	for _, message := range messages {
		prompts = append(prompts, types.PromptMessage{ID: "hook-prompt:" + strings.TrimSpace(message.ID), Role: types.HookPromptRoleForPosition(message.Position, message.Role), Content: strings.TrimSpace(message.Content)})
	}
	return prompts
}

func hookPromptIndependentMessageCount(groups map[string][]types.HookPromptMessage) int {
	return len(groups[types.HookPromptPositionSessionTop]) + len(groups[types.HookPromptPositionBeforeUser]) + len(groups[types.HookPromptPositionAfterUser]) + len(groups[types.HookPromptPositionBeforeLatest]) + len(groups[types.HookPromptPositionAfterLatest])
}

func reindexPromptMessages(messages []types.PromptMessage) []types.PromptMessage {
	for index := range messages {
		messages[index].Order = index
	}
	return messages
}
