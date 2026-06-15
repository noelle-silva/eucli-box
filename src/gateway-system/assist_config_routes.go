package gateway

import (
	"net/http"

	"eucli-box/pkg/types"
)

func (s *system) handleLoadContextCompressionConfig(w http.ResponseWriter, r *http.Request) {
	config, err := s.stickers.LoadContextCompressionConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, config)
}

func (s *system) handleSaveContextCompressionConfig(w http.ResponseWriter, r *http.Request) {
	config, err := decodeJSON[types.ContextCompressionConfig](r)
	if err != nil {
		writeError(w, err)
		return
	}
	saved, err := s.stickers.SaveContextCompressionConfig(r.Context(), config)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, saved)
}
