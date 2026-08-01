package gateway

import (
	"net/http"
	"strings"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

const (
	clientVersionHeader        = "X-Eucli-Studio-Version"
	clientMinimumVersionHeader = "X-Eucli-Studio-Minimum-Box-Version"
	clientMaximumVersionHeader = "X-Eucli-Studio-Maximum-Box-Version"
)

func (s *system) authWrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.validateRequestKey(r); err != nil {
			writeError(w, err)
			return
		}
		if !releaseMaintenancePath(r.URL.Path) {
			if err := s.validateClientCompatibility(r); err != nil {
				writeError(w, err)
				return
			}
		}
		next(w, r)
	}
}

func releaseMaintenancePath(path string) bool {
	switch path {
	case "/api/release", "/api/release-checks", "/api/release-checks/refresh":
		return true
	default:
		return false
	}
}

func (s *system) clientCompatibility(r *http.Request) *types.CompatibilityStatus {
	version := strings.TrimSpace(r.Header.Get(clientVersionHeader))
	minimum := strings.TrimSpace(r.Header.Get(clientMinimumVersionHeader))
	maximum := strings.TrimSpace(r.Header.Get(clientMaximumVersionHeader))
	if version == "" && minimum == "" && maximum == "" {
		return nil
	}
	compatibility := types.EucliBoxCompatibility{MinimumVersion: minimum, MaximumVersionExclusive: maximum}
	status := release.AssessEucliBoxCompatibility(version, s.boxRelease.Version, compatibility)
	return &status
}

func (s *system) validateClientCompatibility(r *http.Request) error {
	status := s.clientCompatibility(r)
	if status == nil || status.Compatible {
		return nil
	}
	return gatewayClientIncompatible(status.Reason, *status)
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
