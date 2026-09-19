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

// handleArtifactOperations 返回所有有操作事实的发布物状态：
// 运行中的任务与落盘的失败/取消终态记录，供客户端恢复任务展示。
func (s *system) handleArtifactOperations(w http.ResponseWriter, r *http.Request) {
	operations := []types.ArtifactInstallState{}
	if tools, err := s.tools.ListToolOperations(r.Context()); err == nil {
		operations = append(operations, tools...)
	}
	if plugins, err := s.systemPlugins.ListPluginOperations(r.Context()); err == nil {
		operations = append(operations, plugins...)
	}
	writeData(w, http.StatusOK, types.ArtifactOperationList{Operations: operations})
}
