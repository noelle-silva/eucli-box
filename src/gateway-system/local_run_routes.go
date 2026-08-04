package gateway

import "net/http"

func (s *system) localAuthWrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.config.LocalRun {
			writeError(w, gatewayNotAuthorized("local route is unavailable", nil))
			return
		}
		if err := s.validateRequestKey(r); err != nil {
			writeError(w, err)
			return
		}
		next(w, r)
	}
}

func (s *system) handleLocalRun(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]any{
		"installIdentity":  s.localInfo.InstallIdentity,
		"dataIdentity":     s.localInfo.DataIdentity,
		"runIdentity":      s.localInfo.RunIdentity,
		"processId":        s.localInfo.ProcessID,
		"processStartedAt": s.localInfo.ProcessStartedAt,
		"version":          s.boxRelease.Version,
		"clientCompatibility": map[string]any{
			"compatible": true,
		},
	})
}

func (s *system) handleLocalRunStop(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[struct {
		RunIdentity string `json:"runIdentity"`
	}](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if request.RunIdentity != s.localInfo.RunIdentity {
		writeError(w, gatewayNotAuthorized("local run identity mismatch", nil))
		return
	}
	writeData(w, http.StatusOK, map[string]string{"status": "stop_requested"})
	s.localStopOnce.Do(func() { go s.config.LocalStop() })
}
