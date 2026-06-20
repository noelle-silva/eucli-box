package placeholder

import apperrors "eucli-box/pkg/errors"

const systemName = "placeholder-system"

func placeholderInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "placeholder.invalid_request", message, cause)
}

func placeholderReadFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "placeholder.read_failed", message, cause)
}

func placeholderWriteFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "placeholder.write_failed", message, cause)
}
