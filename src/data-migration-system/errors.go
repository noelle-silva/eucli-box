package datamigration

import apperrors "eucli-box/pkg/errors"

const systemName = "data-migration-system"

func migrationInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "migration.invalid_request", message, cause)
}

func migrationPrepareFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "migration.prepare_failed", message, cause)
}

func migrationStepMissing(message string, cause error) error {
	return apperrors.Wrap(systemName, "migration.step_missing", message, cause)
}

func migrationStepFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "migration.step_failed", message, cause)
}

func migrationVerifyFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "migration.verify_failed", message, cause)
}

func migrationVersionTooHigh(message string, cause error) error {
	return apperrors.Wrap(systemName, "migration.version_too_high", message, cause)
}

func migrationRecoveryFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "migration.recovery_failed", message, cause)
}

func migrationStatusUnknown(message string, cause error) error {
	return apperrors.Wrap(systemName, "migration.status_unknown", message, cause)
}
