package roleprompt

import (
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

func validatePrompts(prompts []types.PromptMessage) error {
	if len(prompts) == 0 {
		return roleInvalid("at least one prompt is required", nil)
	}
	for _, prompt := range prompts {
		if strings.TrimSpace(prompt.Content) == "" {
			return roleInvalid("prompt content is required", nil)
		}
		if prompt.Role != "system" {
			return roleInvalid("role prompts must be system messages", nil)
		}
	}
	return nil
}

func sortedPrompts(prompts []types.PromptMessage) []types.PromptMessage {
	result := make([]types.PromptMessage, len(prompts))
	copy(result, prompts)
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Order < result[j].Order
	})
	return result
}
