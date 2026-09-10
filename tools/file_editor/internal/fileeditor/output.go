package fileeditor

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
