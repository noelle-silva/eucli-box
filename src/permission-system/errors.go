package permission

import apperrors "eucli-box/pkg/errors"

const systemName = "permission-system"

func permissionInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "permission.invalid_request", message, cause)
}

func permissionRoleFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "permission.role_failed", message, cause)
}

func permissionConfirmationInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "permission.confirmation_invalid", message, cause)
}
