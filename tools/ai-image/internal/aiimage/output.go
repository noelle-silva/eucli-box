package aiimage

import (
	"errors"

	"eucli-box/tools/ai-image/internal/types"
)

// failure 构造失败结果：每层说明自己当时在做什么，保留真实原因。
// 带分类的错误会把可重试标记与建议动作写进元数据，并在消息尾部附摘要。
func failure(scope string, err error, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	message := scope
	if err != nil {
		message = scope + ": " + err.Error()
	}
	var classified *classifiedError
	if errors.As(err, &classified) {
		metadata["retryable"] = classified.retryable
		if classified.action != "" {
			metadata["suggestedAction"] = classified.action
			message += "（" + retryabilityLabel(classified.retryable) + "；建议：" + classified.action + "）"
		} else {
			message += "（" + retryabilityLabel(classified.retryable) + "）"
		}
	}
	metadata["error"] = message
	return types.ToolExecutionOutput{
		Status:   types.ToolStatusFailed,
		Content:  message,
		Error:    message,
		Metadata: metadata,
	}
}

// retryabilityLabel 返回可重试性的人类可读标记。
func retryabilityLabel(retryable bool) string {
	if retryable {
		return "可重试"
	}
	return "不可重试"
}

// success 构造成功结果。
func success(content string, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: metadata}
}
