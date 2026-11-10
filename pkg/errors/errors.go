package apperrors

import (
	"fmt"

	"eucli-box/pkg/types"
)

type AppError struct {
	Code    string
	Message string
	System  string
	Cause   error
	Details any
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.System == "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %s", e.System, e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func New(system string, code string, message string) *AppError {
	return &AppError{System: system, Code: code, Message: message}
}

func Wrap(system string, code string, message string, cause error) *AppError {
	return &AppError{System: system, Code: code, Message: message, Cause: cause}
}

func NewWithDetails(system string, code string, message string, details any) *AppError {
	return &AppError{System: system, Code: code, Message: message, Details: details}
}

func WrapWithDetails(system string, code string, message string, cause error, details any) *AppError {
	return &AppError{System: system, Code: code, Message: message, Cause: cause, Details: details}
}

// WrapWithCauses 构造一个携带多个并列原因的错误；原因会展开成 ErrorPayload.Causes，
// 便于一次看清每个来源各自是怎么失败的。
func WrapWithCauses(system string, code string, message string, causes []error) error {
	return &multiCauseError{system: system, code: code, message: message, causes: causes}
}

// multiCauseError 是一个错误码 + 多个并列原因的复合错误。
type multiCauseError struct {
	system  string
	code    string
	message string
	causes  []error
}

func (e *multiCauseError) Error() string {
	return fmt.Sprintf("%s: %s: %s", e.system, e.code, e.message)
}

func (e *multiCauseError) Unwrap() []error {
	return e.causes
}

func (e *multiCauseError) ErrorIdentity() (string, string, string) {
	return e.system, e.code, e.message
}

// codedError 暴露错误身份，供 BuildErrorPayload 还原出错误码。
type codedError interface {
	error
	ErrorIdentity() (system string, code string, message string)
}

func BuildErrorPayload(err error) *types.ErrorPayload {
	if err == nil {
		return nil
	}
	return buildErrorPayload(err)
}

func buildErrorPayload(err error) *types.ErrorPayload {
	if err == nil {
		return nil
	}
	if coded, ok := err.(codedError); ok {
		system, code, message := coded.ErrorIdentity()
		payload := &types.ErrorPayload{Code: code, Message: message, System: system}
		attachErrorPayloadCauses(payload, errorPayloadCauses(err))
		return payload
	}
	if appErr, ok := err.(*AppError); ok {
		payload := &types.ErrorPayload{Code: appErr.Code, Message: appErr.Message, System: appErr.System, Details: appErr.Details}
		attachErrorPayloadCauses(payload, errorPayloadCauses(err))
		return payload
	}
	payload := &types.ErrorPayload{Message: err.Error()}
	attachErrorPayloadCauses(payload, errorPayloadCauses(err))
	return payload
}

func attachErrorPayloadCauses(payload *types.ErrorPayload, causes []*types.ErrorPayload) {
	if payload == nil || len(causes) == 0 {
		return
	}
	if len(causes) == 1 {
		payload.Cause = causes[0]
		return
	}
	payload.Causes = causes
}

func errorPayloadCauses(err error) []*types.ErrorPayload {
	if err == nil {
		return nil
	}
	if unwrapped, ok := err.(interface{ Unwrap() []error }); ok {
		causes := make([]*types.ErrorPayload, 0, len(unwrapped.Unwrap()))
		for _, cause := range unwrapped.Unwrap() {
			if payload := buildErrorPayload(cause); payload != nil {
				causes = append(causes, payload)
			}
		}
		return causes
	}
	if unwrapped, ok := err.(interface{ Unwrap() error }); ok {
		if payload := buildErrorPayload(unwrapped.Unwrap()); payload != nil {
			return []*types.ErrorPayload{payload}
		}
	}
	return nil
}
