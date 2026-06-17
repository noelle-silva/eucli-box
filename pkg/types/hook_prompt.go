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

	HookPromptSelectionModePreset  = "preset"
	HookPromptSelectionModeNone    = "none"
	HookPromptSelectionModeInherit = "inherit"

	SessionMetadataHookPromptMode     = "hookPrompt.mode"
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

type HookPromptSelection struct {
	Mode     string `json:"mode,omitempty"`
	PresetID string `json:"presetId,omitempty"`
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
	selection := HookPromptSelectionFromSessionMetadata(metadata)
	if selection.Mode != HookPromptSelectionModePreset {
		return ""
	}
	return selection.PresetID
}

func HookPromptSelectionFromSessionMetadata(metadata map[string]string) HookPromptSelection {
	if len(metadata) == 0 {
		return HookPromptSelection{Mode: HookPromptSelectionModeInherit}
	}
	return NormalizeHookPromptSelection(metadata[SessionMetadataHookPromptMode], metadata[SessionMetadataHookPromptPresetID])
}

func NormalizeHookPromptSelection(mode string, presetID string) HookPromptSelection {
	mode = strings.TrimSpace(mode)
	presetID = strings.TrimSpace(presetID)
	switch mode {
	case HookPromptSelectionModeNone:
		return HookPromptSelection{Mode: HookPromptSelectionModeNone}
	case HookPromptSelectionModePreset:
		if presetID != "" {
			return HookPromptSelection{Mode: HookPromptSelectionModePreset, PresetID: presetID}
		}
		return HookPromptSelection{Mode: HookPromptSelectionModeInherit}
	case HookPromptSelectionModeInherit:
		return HookPromptSelection{Mode: HookPromptSelectionModeInherit}
	default:
		if presetID != "" {
			return HookPromptSelection{Mode: HookPromptSelectionModePreset, PresetID: presetID}
		}
		return HookPromptSelection{Mode: HookPromptSelectionModeInherit}
	}
}

func NormalizeHookPromptSessionUpdate(mode string, presetID string) HookPromptSelection {
	mode = strings.TrimSpace(mode)
	if mode == "" && strings.TrimSpace(presetID) == "" {
		return HookPromptSelection{Mode: HookPromptSelectionModeNone}
	}
	return NormalizeHookPromptSelection(mode, presetID)
}

func SameHookPromptSelection(left HookPromptSelection, right HookPromptSelection) bool {
	left = NormalizeHookPromptSelection(left.Mode, left.PresetID)
	right = NormalizeHookPromptSelection(right.Mode, right.PresetID)
	return left.Mode == right.Mode && left.PresetID == right.PresetID
}

func PutHookPromptPresetSessionMetadata(metadata map[string]string, presetID string) map[string]string {
	return PutHookPromptSessionMetadata(metadata, NormalizeHookPromptSelection(HookPromptSelectionModePreset, presetID))
}

func PutHookPromptSessionMetadata(metadata map[string]string, selection HookPromptSelection) map[string]string {
	selection = NormalizeHookPromptSelection(selection.Mode, selection.PresetID)
	out := copySessionMetadata(metadata)
	delete(out, SessionMetadataHookPromptMode)
	delete(out, SessionMetadataHookPromptPresetID)
	if selection.Mode == HookPromptSelectionModeNone {
		out[SessionMetadataHookPromptMode] = HookPromptSelectionModeNone
	} else if selection.Mode == HookPromptSelectionModePreset {
		out[SessionMetadataHookPromptMode] = HookPromptSelectionModePreset
		out[SessionMetadataHookPromptPresetID] = selection.PresetID
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func HookPromptPresetSessionMetadataPatch(presetID string) map[string]string {
	return HookPromptSessionMetadataPatch(NormalizeHookPromptSelection(HookPromptSelectionModePreset, presetID))
}

func HookPromptSessionMetadataPatch(selection HookPromptSelection) map[string]string {
	selection = NormalizeHookPromptSelection(selection.Mode, selection.PresetID)
	switch selection.Mode {
	case HookPromptSelectionModeNone:
		return map[string]string{SessionMetadataHookPromptMode: HookPromptSelectionModeNone, SessionMetadataHookPromptPresetID: ""}
	case HookPromptSelectionModePreset:
		return map[string]string{SessionMetadataHookPromptMode: HookPromptSelectionModePreset, SessionMetadataHookPromptPresetID: selection.PresetID}
	default:
		return map[string]string{SessionMetadataHookPromptMode: "", SessionMetadataHookPromptPresetID: ""}
	}
}
