package imagereader

import (
	"eucli-box/tools/image_reader/internal/types"
)

// success 构造成功结果。
func success(content string, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: metadata}
}

// failure 构造失败结果：说明当时在做什么，保留真实原因。
func failure(scope string, err error) types.ToolExecutionOutput {
	message := scope
	if err != nil {
		message = scope + ": " + err.Error()
	}
	return types.ToolExecutionOutput{
		Status:   types.ToolStatusFailed,
		Content:  message,
		Error:    message,
		Metadata: map[string]any{"error": message},
	}
}
