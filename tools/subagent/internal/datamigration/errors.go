package datamigration

import "fmt"

const systemName = "data-migration-system"

// AppError 是可判定错误码的迁移错误：码与消息固定，原因链保留。
type AppError struct {
	Code    string
	Message string
	System  string
	Cause   error
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s: %s", e.System, e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func migrationError(code string, message string, cause error) error {
	return &AppError{System: systemName, Code: code, Message: message, Cause: cause}
}

func migrationInvalid(message string, cause error) error {
	return migrationError("migration.invalid_request", message, cause)
}

func migrationPrepareFailed(message string, cause error) error {
	return migrationError("migration.prepare_failed", message, cause)
}

func migrationStepMissing(message string, cause error) error {
	return migrationError("migration.step_missing", message, cause)
}

func migrationStepFailed(message string, cause error) error {
	return migrationError("migration.step_failed", message, cause)
}

func migrationVerifyFailed(message string, cause error) error {
	return migrationError("migration.verify_failed", message, cause)
}

func migrationVersionTooHigh(message string, cause error) error {
	return migrationError("migration.version_too_high", message, cause)
}

func migrationRecoveryFailed(message string, cause error) error {
	return migrationError("migration.recovery_failed", message, cause)
}

func migrationStatusUnknown(message string, cause error) error {
	return migrationError("migration.status_unknown", message, cause)
}
