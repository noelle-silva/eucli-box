package filereader

import (
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
