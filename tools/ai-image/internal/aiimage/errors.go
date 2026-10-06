package aiimage

import (
	"errors"
	"strings"

	apperrors "eucli-box/tools/ai-image/internal/errors"
)

// 失败分类的建议动作：瞬时故障引导重试，参数与配置类失败引导修正。
const (
	actionRetryLater    = "稍后重试"
	actionFixParams     = "检查并修正参数后重试"
	actionFixConfig     = "检查工具配置后重试"
	actionFixModels     = "改用运营商 models 列表内的模型后重试"
	actionFixTimeout    = "将 timeoutMs 调整到 5000-3600000 毫秒"
	actionReplaceImage  = "更换参考图后重试"
	actionCheckNetwork  = "检查运营商地址与网络连接后重试"
	actionCheckProvider = "检查请求参数与运营商配置后重试"
)

// classifiedError 是带失败分类的错误：是否可重试与建议动作。
// 失败结果构造时提取分类写入元数据，错误文本保持原样。
type classifiedError struct {
	err       error
	retryable bool
	action    string
}

func (e *classifiedError) Error() string { return e.err.Error() }

func (e *classifiedError) Unwrap() error { return e.err }

// retryableError 标记瞬时故障：可重试。
func retryableError(err error, action string) error {
	if err == nil {
		return nil
	}
	return &classifiedError{err: err, retryable: true, action: action}
}

// permanentError 标记参数或配置类失败：不可重试。
func permanentError(err error, action string) error {
	if err == nil {
		return nil
	}
	return &classifiedError{err: err, retryable: false, action: action}
}

// networkErrorCode 返回网络请求系统的错误码；非网络错误返回空串。
func networkErrorCode(err error) string {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) && strings.HasPrefix(appErr.Code, "network.") {
		return appErr.Code
	}
	return ""
}
