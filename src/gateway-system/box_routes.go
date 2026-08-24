package gateway

import (
	"net/http"

	"eucli-box/pkg/types"
)

func (s *system) handleBoxInfo(w http.ResponseWriter, r *http.Request) {
	info := types.EucliBoxReleaseInfo{Version: s.boxRelease.Version, DataVersion: s.boxRelease.DataVersion}
	if compatibility := s.clientCompatibility(r); compatibility != nil {
		info.ClientCompatibility = compatibility
	}
	writeData(w, http.StatusOK, map[string]any{
		"version":             info.Version,
		"dataVersion":         info.DataVersion,
		"clientCompatibility": info.ClientCompatibility,
	})
}
