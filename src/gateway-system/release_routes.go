package gateway

import (
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

// handleArtifactInstallations 返回业务端当前真实的已装事实。
func (s *system) handleArtifactInstallations(w http.ResponseWriter, r *http.Request) {
	list, err := s.releaseSource.ListInstallations(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, list)
}

// handleReleaseCandidates 返回当前安装来源下某分类的候选事实与比对结论。
func (s *system) handleReleaseCandidates(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	list, err := s.releaseSource.ListCandidates(r.Context(), kind)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	writeData(w, http.StatusOK, list)
}
