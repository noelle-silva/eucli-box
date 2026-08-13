package gateway

import (
	"encoding/json"
	"net/http"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

type successResponse struct {
	Data any `json:"data"`
}

type errorResponse struct {
	Error types.ErrorPayload `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, successResponse{Data: data})
}

func writeNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, err error) {
	payload := errorPayloadForResponse(err)
	writeJSON(w, statusForPayload(&payload), errorResponse{Error: payload})
}

func errorPayloadForResponse(err error) types.ErrorPayload {
	if payload := apperrors.BuildErrorPayload(err); payload != nil {
		if payload.Code == "" && payload.System == "" {
			return types.ErrorPayload{Code: "gateway.internal_error", Message: "internal gateway error", System: systemName, Cause: payload}
		}
		return *payload
	}
	return types.ErrorPayload{Code: "gateway.internal_error", Message: "internal gateway error", System: systemName}
}

func statusForPayload(payload *types.ErrorPayload) int {
	if payload == nil {
		return http.StatusInternalServerError
	}
	status := statusForCode(payload.Code)
	if status != http.StatusInternalServerError {
		return status
	}
	if status := statusForPayload(payload.Cause); status != http.StatusInternalServerError {
		return status
	}
	for _, cause := range payload.Causes {
		if status := statusForPayload(cause); status != http.StatusInternalServerError {
			return status
		}
	}
	return http.StatusInternalServerError
}

func statusForCode(code string) int {
	switch {
	case hasSuffix(code, "invalid_request"):
		return http.StatusBadRequest
	case hasSuffix(code, "not_found"):
		return http.StatusNotFound
	case hasSuffix(code, "incompatible_client"):
		return http.StatusConflict
	case hasSuffix(code, "forbidden"):
		return http.StatusForbidden
	case hasSuffix(code, "unauthorized"):
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

func hasSuffix(value string, suffix string) bool {
	if len(value) < len(suffix) {
		return false
	}
	return value[len(value)-len(suffix):] == suffix
}
