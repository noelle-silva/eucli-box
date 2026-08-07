package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) handleRelease(w http.ResponseWriter, r *http.Request) {
	info := types.EucliBoxReleaseInfo{Version: s.boxRelease.Version, DataVersion: s.boxRelease.DataVersion}
	if compatibility := s.clientCompatibility(r); compatibility != nil {
		info.ClientCompatibility = compatibility
	}
	writeData(w, http.StatusOK, info)
}

func (s *system) handleReleaseChecks(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, s.releaseChecks.Snapshot())
}

func (s *system) handleRefreshReleaseChecks(w http.ResponseWriter, r *http.Request) {
	kind := ""
	if r.Body != nil {
		var body struct {
			Kind string `json:"kind"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			kind = strings.TrimSpace(body.Kind)
		}
	}
	writeData(w, http.StatusOK, s.releaseChecks.Refresh(r.Context(), kind))
}
