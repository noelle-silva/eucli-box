package toolcalling

import apperrors "eucli-box/pkg/errors"

const systemName = "tool-calling-system"

func toolInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "tool.invalid_request", message, cause)
}

func toolNotFound(message string, cause error) error {
	return apperrors.Wrap(systemName, "tool.not_found", message, cause)
}

func toolStorageFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "tool.storage_failed", message, cause)
}

func toolPermissionFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "tool.permission_failed", message, cause)
}

func toolExecutionInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "tool.execution_invalid", message, cause)
}
