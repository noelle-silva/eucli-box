package modelprovider

import "eucli-box/pkg/types"

func modelToolDescription(tool types.ToolDefinition) string {
	return types.ToolPromptDescription(tool)
}
