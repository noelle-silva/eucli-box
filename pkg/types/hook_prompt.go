package types

import "strings"

const (
	HookPromptRoleSystem    = "system"
	HookPromptRoleUser      = "user"
	HookPromptRoleAssistant = "assistant"

	HookPromptPositionSessionTop       = "session_top"
	HookPromptPositionBeforeUser       = "before_user"
	HookPromptPositionAfterUser        = "after_user"
	HookPromptPositionBeforeLatest     = "before_latest"
	HookPromptPositionAfterLatest      = "after_latest"
	HookPromptPositionInsideUserTop    = "inside_user_top"
	HookPromptPositionInsideUserBottom = "inside_user_bottom"

	SessionMetadataHookPromptPresetID = "hookPrompt.presetId"
)

var HookPromptPositions = []string{
	HookPromptPositionSessionTop,
	HookPromptPositionBeforeUser,
	HookPromptPositionAfterUser,
	HookPromptPositionBeforeLatest,
	HookPromptPositionAfterLatest,
	HookPromptPositionInsideUserTop,
	HookPromptPositionInsideUserBottom,
}

type HookPromptLibrary struct {
	Presets []HookPromptPreset `json:"presets"`
}

type HookPromptPreset struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Messages  []HookPromptMessage `json:"messages"`
	CreatedAt string              `json:"createdAt,omitempty"`
	UpdatedAt string              `json:"updatedAt,omitempty"`
}

type HookPromptMessage struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Position  string `json:"position"`
	Content   string `json:"content"`
	Order     int    `json:"order"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

func NormalizeHookPromptRole(value string) string {
	switch strings.TrimSpace(value) {
	case HookPromptRoleAssistant:
		return HookPromptRoleAssistant
	case HookPromptRoleSystem:
		return HookPromptRoleSystem
	default:
		return HookPromptRoleUser
	}
}

func HookPromptRoleForPosition(position string, role string) string {
	position = strings.TrimSpace(position)
	if position == HookPromptPositionInsideUserTop || position == HookPromptPositionInsideUserBottom {
		return HookPromptRoleUser
	}
	return NormalizeHookPromptRole(role)
}

func IsHookPromptPosition(value string) bool {
	value = strings.TrimSpace(value)
	for _, position := range HookPromptPositions {
		if value == position {
			return true
		}
	}
	return false
}

func HookPromptPresetIDFromSessionMetadata(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	return strings.TrimSpace(metadata[SessionMetadataHookPromptPresetID])
}

func PutHookPromptPresetSessionMetadata(metadata map[string]string, presetID string) map[string]string {
	presetID = strings.TrimSpace(presetID)
	if len(metadata) == 0 && presetID == "" {
		return nil
	}
	out := copySessionMetadata(metadata)
	if presetID == "" {
		delete(out, SessionMetadataHookPromptPresetID)
	} else {
		out[SessionMetadataHookPromptPresetID] = presetID
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func HookPromptPresetSessionMetadataPatch(presetID string) map[string]string {
	return map[string]string{SessionMetadataHookPromptPresetID: strings.TrimSpace(presetID)}
}
