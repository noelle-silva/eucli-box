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

func (s *system) handleReorderProviders(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[reorderRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.providers.ReorderProviders(r.Context(), request.IDs); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleReorderTools(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[reorderRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.tools.ReorderTools(r.Context(), request.IDs); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleReorderSystemPlugins(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[reorderRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.systemPlugins.ReorderPlugins(r.Context(), request.IDs); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

type reorderStickersRequest struct {
	CategoryName string   `json:"categoryName"`
	Names        []string `json:"names"`
}

func (s *system) handleReorderStickerCategories(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[reorderRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.stickers.ReorderStickerCategories(r.Context(), request.IDs); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleReorderStickers(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[reorderStickersRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.stickers.ReorderStickers(r.Context(), request.CategoryName, request.Names); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleReorderPlaceholders(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[reorderRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	library, err := s.placeholders.ReorderPlaceholders(r.Context(), request.IDs)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

func (s *system) handleReorderHookPromptPresets(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[reorderRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	library, err := s.hooks.ReorderHookPromptPresets(r.Context(), request.IDs)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}
