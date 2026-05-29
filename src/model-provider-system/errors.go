package modelprovider

import apperrors "eucli-box/pkg/errors"

const systemName = "model-provider-system"

func providerInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "provider.invalid_request", message, cause)
}

func providerUnsupportedProtocol(message string, cause error) error {
	return apperrors.Wrap(systemName, "provider.unsupported_protocol", message, cause)
}

func providerNotFound(message string, cause error) error {
	return apperrors.Wrap(systemName, "provider.not_found", message, cause)
}

func providerModelNotFound(message string, cause error) error {
	return apperrors.Wrap(systemName, "provider.model_not_found", message, cause)
}

func providerNetworkFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "provider.network_failed", message, cause)
}

func providerServiceFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "provider.service_failed", message, cause)
}

func providerParseFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "provider.parse_failed", message, cause)
}

func providerStorageFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "provider.storage_failed", message, cause)
}
