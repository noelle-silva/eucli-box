package agentruntime

import apperrors "eucli-box/pkg/errors"

const systemName = "agent-runtime-system"

func runtimeInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "runtime.invalid_request", message, cause)
}

func runtimeNotFound(message string, cause error) error {
	return apperrors.Wrap(systemName, "runtime.not_found", message, cause)
}

func runtimeStorageFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "runtime.storage_failed", message, cause)
}

func runtimeRoleFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "runtime.role_failed", message, cause)
}

func runtimeProviderFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "runtime.provider_failed", message, cause)
}

func runtimeToolFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "runtime.tool_failed", message, cause)
}

func runtimeStateInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "runtime.state_invalid", message, cause)
}
