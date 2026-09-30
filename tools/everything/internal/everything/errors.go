package everything

import (
	"errors"
	"fmt"
)

// codedError 是带机器可读错误码的错误：Error 只给可读文案，码经 errorCode 提取。
// 后端错误码经响应信封透传；工具自身的配置错误使用本地码。
type codedError struct {
	code    string
	message string
}

func (e *codedError) Error() string { return e.message }

// coded 构造带机器可读错误码的错误。
func coded(code string, format string, args ...any) error {
	return &codedError{code: code, message: fmt.Sprintf(format, args...)}
}

// errorCode 提取错误链上的机器可读错误码；没有码时返回空串。
func errorCode(err error) string {
	var target *codedError
	if errors.As(err, &target) {
		return target.code
	}
	return ""
}
