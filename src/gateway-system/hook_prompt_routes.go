package gateway

import (
	"net/http"

	"eucli-box/pkg/types"
)

func (s *system) handleLoadHookPromptLibrary(w http.ResponseWriter, r *http.Request) {
	library, err := s.hooks.LoadHookPromptLibrary(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

func (s *system) handleSaveHookPromptLibrary(w http.ResponseWriter, r *http.Request) {
	library, err := decodeJSON[types.HookPromptLibrary](r)
	if err != nil {
		writeError(w, err)
		return
	}
	saved, err := s.hooks.SaveHookPromptLibrary(r.Context(), library)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, saved)
}
