package systemplugin

import apperrors "eucli-box/pkg/errors"

const systemName = "system-plugin-system"

func pluginInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "system_plugin.invalid_request", message, cause)
}

func pluginReadFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "system_plugin.read_failed", message, cause)
}

func pluginWriteFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "system_plugin.write_failed", message, cause)
}

func pluginExecutionFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "system_plugin.execution_failed", message, cause)
}
