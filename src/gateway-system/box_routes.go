package gateway

import (
	"encoding/json"
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

// handleBoxShutdown 处理业务端停止请求：
// 存在未结束的真实工作时返回工作列表要求确认；没有未结束工作或用户已确认时触发正常退出。
func (s *system) handleBoxShutdown(w http.ResponseWriter, r *http.Request) {
	if s.config.LocalRun && s.config.LocalStop == nil {
		writeError(w, gatewayDependencyFailed("受托模式停止回调未配置", nil))
		return
	}
	var request struct {
		Confirm bool `json:"confirm"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&request)
	}
	activeRuns, err := s.runtime.ListActiveRuns(r.Context())
	if err != nil {
		writeError(w, gatewayDependencyFailed("读取真实工作状态失败", err))
		return
	}
	if len(activeRuns) > 0 && !request.Confirm {
		writeData(w, http.StatusConflict, map[string]any{
			"requiresConfirmation": true,
			"activeWork":           activeRuns,
		})
		return
	}
	if s.config.LocalRun {
		s.localStopOnce.Do(func() { go s.config.LocalStop() })
	} else {
		writeError(w, gatewayForbidden("业务端停止只允许受托本机连接发起", nil))
		return
	}
	writeData(w, http.StatusOK, map[string]string{"status": "shutdown_requested"})
}
