package datastorage

import apperrors "eucli-box/pkg/errors"

const systemName = "data-storage-system"

func storageInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "storage.invalid_request", message, cause)
}

func storageNotFound(message string, cause error) error {
	return apperrors.Wrap(systemName, "storage.not_found", message, cause)
}

func storageInitFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "storage.initialize_failed", message, cause)
}

func storageReadFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "storage.read_failed", message, cause)
}

func storageWriteFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "storage.write_failed", message, cause)
}

func storageDeleteFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "storage.delete_failed", message, cause)
}

func storageIndexFailed(message string, cause error) error {
	return apperrors.Wrap(systemName, "storage.index_failed", message, cause)
}
