package modelprovider

import (
	"strings"

	"eucli-box/pkg/types"
)

func modelToolDescription(tool types.ToolDefinition) string {
	description := types.ToolPromptDescription(tool)
	mode := types.CleanToolInvocationMode(tool.DefaultInvocationMode)
	if mode != types.ToolInvocationModeAsync {
		return description
	}
	description = strings.TrimSpace(description)
	if description == "" {
		return "Default invocation mode: async."
	}
	return description + "\nDefault invocation mode: async."
}
