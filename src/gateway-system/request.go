package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const maxRequestBodyBytes = 4 << 20

func decodeJSON[T any](r *http.Request) (T, error) {
	var value T
	payload, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes+1))
	if err != nil {
		return value, gatewayInvalid("failed to read request body", err)
	}
	if int64(len(payload)) > maxRequestBodyBytes {
		return value, gatewayInvalid("request body is too large", nil)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, gatewayInvalid("request body is invalid json", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return value, gatewayInvalid("request body must contain only one json value", err)
	}
	return value, nil
}

func pathValue(r *http.Request, key string) (string, error) {
	value := r.PathValue(key)
	if value == "" {
		return "", gatewayInvalid("path value is required", nil)
	}
	return value, nil
}
