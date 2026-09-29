package aiimage

import (
	"eucli-box/pkg/types"
)

// failure 构造失败结果：每层说明自己当时在做什么，保留真实原因。
func failure(scope string, err error, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	message := scope
	if err != nil {
		message = scope + ": " + err.Error()
	}
	metadata["error"] = message
	return types.ToolExecutionOutput{
		Status:   types.ToolStatusFailed,
		Content:  message,
		Error:    message,
		Metadata: metadata,
	}
}

// success 构造成功结果。
func success(content string, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: metadata}
}
