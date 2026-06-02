package aiassist

import apperrors "eucli-box/pkg/errors"

const systemName = "ai-assist-system"

func assistInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "assist.invalid_request", message, cause)
}

func assistFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "assist.failed", message, cause)
}
