package gateway

import (
	"encoding/json"
	"errors"
	"net/http"

	apperrors "eucli-box/pkg/errors"
)

type successResponse struct {
	Data any `json:"data"`
}

type errorResponse struct {
	Error responseError `json:"error"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	System  string `json:"system"`
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
	status := http.StatusInternalServerError
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		status = statusForCode(appErr.Code)
		writeJSON(w, status, errorResponse{Error: responseError{Code: appErr.Code, Message: appErr.Message, System: appErr.System}})
		return
	}
	writeJSON(w, status, errorResponse{Error: responseError{Code: "gateway.internal_error", Message: err.Error(), System: systemName}})
}

func statusForCode(code string) int {
	switch {
	case hasSuffix(code, "invalid_request"):
		return http.StatusBadRequest
	case hasSuffix(code, "not_found"):
		return http.StatusNotFound
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
