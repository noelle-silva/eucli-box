package gateway

import apperrors "eucli-box/pkg/errors"

const systemName = "gateway-system"

func gatewayInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "gateway.invalid_request", message, cause)
}

func gatewayDependencyFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "gateway.dependency_failed", message, cause)
}

func gatewayServerFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "gateway.server_failed", message, cause)
}

func gatewayWebSocketFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "gateway.websocket_failed", message, cause)
}
