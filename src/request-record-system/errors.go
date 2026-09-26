package requestrecord

import apperrors "eucli-box/pkg/errors"

const systemName = "request-record-system"

func recordInvalid(message string, cause error) error {
	return apperrors.Wrap(systemName, "request_record.invalid_request", message, cause)
}
