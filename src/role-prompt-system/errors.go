package roleprompt

import apperrors "eucli-box/pkg/errors"

const systemName = "role-prompt-system"

func roleInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "role.invalid_request", message, cause)
}

func roleNotFound(message string, cause error) error {
	return apperrors.Wrap(systemName, "role.not_found", message, cause)
}

func roleStorageFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "role.storage_failed", message, cause)
}

func roleModelInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "role.model_invalid", message, cause)
}

func roleToolModeMissing(message string, cause error) error {
	return apperrors.Wrap(systemName, "role.tool_mode_missing", message, cause)
}
