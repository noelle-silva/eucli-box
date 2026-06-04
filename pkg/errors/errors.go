package apperrors

import "fmt"

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
