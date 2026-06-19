package toolcalling

import (
	"context"
	"fmt"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func (s *system) NormalizeIntent(ctx context.Context, intent types.ToolIntent) (types.ToolAction, error) {
	if err := ctx.Err(); err != nil {
		return types.ToolAction{}, toolInvalid("intent normalization cancelled", err)
	}
	toolName := strings.TrimSpace(intent.ToolName)
	if toolName == "" {
		return types.ToolAction{}, toolInvalid("tool intent requires tool name", nil)
	}
	actionID := strings.TrimSpace(intent.ID)
	if actionID == "" {
		actionID = utils.NewID("tool-action")
	}
	arguments := map[string]any{}
	invocationMode := intent.InvocationMode
	for key, value := range intent.Arguments {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			return types.ToolAction{}, toolInvalid("tool intent contains empty argument name", nil)
		}
		if trimmed == "invocationMode" || trimmed == "invocation_mode" || trimmed == "eucliInvocationMode" {
			mode := types.ToolInvocationMode(strings.TrimSpace(fmt.Sprint(value)))
			if !types.ValidToolInvocationMode(mode) || strings.TrimSpace(string(mode)) == "" {
				return types.ToolAction{}, toolInvalid("tool invocation mode must be sync or async", nil)
			}
			invocationMode = types.NormalizeToolInvocationMode(mode)
			continue
		}
		arguments[trimmed] = value
	}
	source := strings.TrimSpace(intent.Source)
	if source == "" {
		source = types.ToolCallSourceNative
	}
	if !types.ValidToolInvocationMode(invocationMode) {
		return types.ToolAction{}, toolInvalid("tool invocation mode must be sync or async", nil)
	}
	return types.ToolAction{ID: actionID, ToolName: toolName, Arguments: arguments, InvocationMode: types.ToolInvocationMode(strings.TrimSpace(string(invocationMode))), Source: source, Raw: intent.Raw, CreatedAt: time.Now().UTC()}, nil
}
