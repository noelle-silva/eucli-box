package gateway

import (
	"net/http"
	"strings"

	apperrors "eucli-box/pkg/errors"
)

func (s *system) authWrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.validateRequestKey(r); err != nil {
			writeError(w, err)
			return
		}
		next(w, r)
	}
}

func (s *system) validateRequestKey(r *http.Request) error {
	key := strings.TrimSpace(s.config.Key)
	if key == "" {
		return nil
	}
	requestKey := extractRequestKey(r)
	if requestKey == key {
		return nil
	}
	return gatewayNotAuthorized("eucli-box key mismatch", nil)
}

func extractRequestKey(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func gatewayNotAuthorized(message string, cause error) error {
	return apperrors.Wrap(systemName, "gateway.unauthorized", message, cause)
}
