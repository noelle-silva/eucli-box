package gateway

import (
	"net/http"

	"eucli-box/pkg/types"
)

func (s *system) handleRelease(w http.ResponseWriter, r *http.Request) {
	info := types.EucliBoxReleaseInfo{Version: s.boxRelease.Version}
	if compatibility := s.clientCompatibility(r); compatibility != nil {
		info.ClientCompatibility = compatibility
	}
	writeData(w, http.StatusOK, info)
}
