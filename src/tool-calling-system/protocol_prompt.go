package toolcalling

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) TextToolInstructions(ctx context.Context, tools []types.ToolDefinition) (types.PromptMessage, error) {
	if err := ctx.Err(); err != nil {
		return types.PromptMessage{}, toolProtocolInvalid("TOOL_REQUEST_CANCELLED: text tool request instruction building was cancelled", err)
	}
	if len(tools) == 0 {
		return types.PromptMessage{}, nil
	}
	content := buildTextToolInstructions(tools)
	if strings.TrimSpace(content) == "" {
		return types.PromptMessage{}, nil
	}
	now := time.Now().UTC()
	return types.PromptMessage{Role: "system", Content: content, CreatedAt: now, UpdatedAt: now}, nil
}

func buildTextToolInstructions(tools []types.ToolDefinition) string {
	var builder strings.Builder
	builder.WriteString("Tool calling instructions:\n")
	builder.WriteString("- You may receive native provider tool-calling capability. If you use that native channel for an action, do not also emit a text tool request for the same action.\n")
	builder.WriteString("- If you need to request a tool through normal text, use exactly this block format and do not wrap it in a code fence:\n")
	builder.WriteString(toolRequestStartMarker + "\n")
	builder.WriteString("[tool]: tool-name\n")
	builder.WriteString("[argument_name]: single-line value\n")
	builder.WriteString(toolRequestEndMarker + "\n")
	builder.WriteString("- The first key inside every block must be [tool]. Use one block per tool call.\n")
	builder.WriteString("- You may emit multiple independent tool request blocks in the same response; the runtime can execute independent tools in parallel and will return one result per block.\n")
	builder.WriteString("- If one tool result is needed to decide another tool call's arguments, request the dependent tool only after receiving the first result.\n")
	builder.WriteString("- Do not invent tool results. After requesting a tool, wait for the tool result before continuing.\n")
	builder.WriteString("- Available tools for text request blocks:\n")
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			name = strings.TrimSpace(tool.ID)
		}
		if name == "" {
			continue
		}
		description := toolPromptDescription(tool)
		if description == "" {
			description = "No description provided."
		}
		builder.WriteString("  - ")
		builder.WriteString(name)
		builder.WriteString(": ")
		builder.WriteString(description)
		if len(tool.InputSchema) > 0 {
			if payload, err := json.Marshal(tool.InputSchema); err == nil {
				builder.WriteString(" Input schema: ")
				builder.Write(payload)
			}
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func toolPromptDescription(tool types.ToolDefinition) string {
	if promptDescription := strings.TrimSpace(tool.PromptDescription); promptDescription != "" {
		return promptDescription
	}
	return strings.TrimSpace(tool.Description)
}
