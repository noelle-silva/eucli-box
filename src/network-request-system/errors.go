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

func dnsFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "network.dns_failed", message, cause)
}

func connectionRefused(message string, cause error) error {
	return apperrors.Wrap(systemName, "network.connection_refused", message, cause)
}

func connectionFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "network.connection_failed", message, cause)
}

func tlsFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "network.tls_failed", message, cause)
}

func connectionLost(message string, cause error) error {
	return apperrors.Wrap(systemName, "network.connection_lost", message, cause)
}
