package networkrequest

import apperrors "eucli-box/pkg/errors"

const systemName = "network-request-system"

func invalidRequest(message string, cause error) error {
	return apperrors.Wrap(systemName, "network.invalid_request", message, cause)
}

func requestFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "network.request_failed", message, cause)
}

func requestTimeout(message string, cause error) error {
	return apperrors.Wrap(systemName, "network.timeout", message, cause)
}
