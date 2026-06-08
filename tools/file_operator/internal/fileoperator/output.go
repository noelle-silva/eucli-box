package fileoperator

import (
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

func success(content string, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: metadata}
}

func failure(scope string, err error, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	errorMessage := scope
	if err != nil {
		errorMessage = scope + ": " + err.Error()
	}
	metadata["error"] = errorMessage
	return types.ToolExecutionOutput{Status: types.ToolStatusFailed, Content: errorMessage, Error: errorMessage, Metadata: metadata}
}

func truncateText(text string, limit int) (string, bool) {
	if limit <= 0 || len(text) <= limit {
		return text, false
	}
	if limit < 32 {
		return text[:limit], true
	}
	return text[:limit] + "\n[truncated: output exceeded maxOutputChars]", true
}

func truncateLine(line string, maxChars int) (string, bool) {
	if maxChars <= 0 || len(line) <= maxChars {
		return line, false
	}
	return line[:maxChars] + "...[line truncated]", true
}

func formatHeader(title string, fields map[string]any) string {
	var builder strings.Builder
	builder.WriteString("<")
	builder.WriteString(title)
	builder.WriteString(">\n")
	for key, value := range fields {
		builder.WriteString(fmt.Sprintf("%s: %v\n", key, value))
	}
	builder.WriteString("\n")
	return builder.String()
}
