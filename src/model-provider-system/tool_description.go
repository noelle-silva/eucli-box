package modelprovider

import (
	"strings"

	"eucli-box/pkg/types"
)

func modelToolDescription(tool types.ToolDefinition) string {
	if promptDescription := strings.TrimSpace(tool.PromptDescription); promptDescription != "" {
		return promptDescription
	}
	return strings.TrimSpace(tool.Description)
}
