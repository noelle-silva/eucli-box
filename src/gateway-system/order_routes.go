package gateway

import "net/http"

type reorderRequest struct {
	IDs []string `json:"ids"`
}

func (s *system) handleReorderRoles(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[reorderRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.roles.ReorderRoles(r.Context(), request.IDs); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleReorderWorkspaces(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[reorderRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.workspaces.ReorderWorkspaces(r.Context(), request.IDs); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}
