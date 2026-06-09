package agentruntime

import (
	"errors"
	"strings"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

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

func errorPayloadFromError(err error, fallback string) *types.ErrorPayload {
	if payload := apperrors.BuildErrorPayload(err); payload != nil {
		return payload
	}
	message := strings.TrimSpace(fallback)
	if err != nil && message == "" {
		message = strings.TrimSpace(err.Error())
	}
	if message == "" {
		return nil
	}
	return &types.ErrorPayload{Message: message}
}

func runFailureFromError(err error, fallback string) (string, *types.ErrorPayload) {
	payload := errorPayloadFromError(err, fallback)
	if payload == nil {
		return strings.TrimSpace(fallback), nil
	}
	reason := mostSpecificErrorMessage(payload)
	if reason == "" {
		reason = strings.TrimSpace(fallback)
	}
	return reason, payload
}

func mostSpecificErrorMessage(payload *types.ErrorPayload) string {
	if payload == nil {
		return ""
	}
	message := strings.TrimSpace(payload.Message)
	if next := mostSpecificErrorMessage(payload.Cause); next != "" {
		message = next
	}
	for _, cause := range payload.Causes {
		if next := mostSpecificErrorMessage(cause); next != "" {
			message = next
		}
	}
	return message
}

func innermostAppError(err error) *apperrors.AppError {
	var found *apperrors.AppError
	for err != nil {
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) {
			break
		}
		found = appErr
		err = appErr.Unwrap()
	}
	return found
}

func isStorageConflict(err error) bool {
	appErr := innermostAppError(err)
	return appErr != nil && appErr.Code == "storage.conflict"
}
