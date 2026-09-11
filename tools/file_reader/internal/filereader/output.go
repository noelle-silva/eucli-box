package filereader

import (
	"unicode/utf8"

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
	cut := clampToRuneBoundary(text, limit)
	if limit < 32 {
		return cut, true
	}
	return cut + "\n[truncated: output exceeded maxOutputChars]", true
}

func truncateLine(line string, maxChars int) (string, bool) {
	if maxChars <= 0 || len(line) <= maxChars {
		return line, false
	}
	return clampToRuneBoundary(line, maxChars) + "...[line truncated]", true
}

// clampToRuneBoundary cuts text to at most limit bytes without splitting a
// multi-byte character.
func clampToRuneBoundary(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if limit >= len(text) {
		return text
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit]
}
