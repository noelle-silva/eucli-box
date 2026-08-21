package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"eucli-box/pkg/installsource"
)

// handleInstallSource 返回当前安装来源状态。
func (s *system) handleInstallSource(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, installsource.StateView{Kind: s.config.InstallSource.Current()})
}

// handleSetInstallSource 切换安装来源；正式模式或非法值由状态本体拒绝。
func (s *system) handleSetInstallSource(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, gatewayInvalid("请求体必须是 {\"kind\": \"official\"|\"development\"}", nil))
		return
	}
	kind, err := installsource.ParseKind(strings.TrimSpace(body.Kind))
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	next, err := s.config.InstallSource.Set(r.Context(), kind)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	writeData(w, http.StatusOK, installsource.StateView{Kind: next})
}
