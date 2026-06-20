package gateway

import (
	"net/http"

	"eucli-box/pkg/placeholder"
	"eucli-box/pkg/types"
)

func (s *system) handleLoadPlaceholderLibrary(w http.ResponseWriter, r *http.Request) {
	library, err := s.placeholders.LoadPlaceholderLibrary(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

func (s *system) handleSavePlaceholderLibrary(w http.ResponseWriter, r *http.Request) {
	library, err := decodeJSON[types.PlaceholderLibrary](r)
	if err != nil {
		writeError(w, err)
		return
	}
	saved, err := s.placeholders.SavePlaceholderLibrary(r.Context(), library)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, saved)
}

func (s *system) handlePreviewPlaceholders(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[types.PlaceholderPreviewRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	library, err := s.placeholders.LoadPlaceholderLibrary(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, placeholder.Resolve(request.Text, library))
}

func (s *system) handlePlaceholderProblems(w http.ResponseWriter, r *http.Request) {
	library, err := s.placeholders.LoadPlaceholderLibrary(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, placeholder.Problems(library))
}

func (s *system) handlePlaceholderDependencies(w http.ResponseWriter, r *http.Request) {
	name, err := pathValue(r, "name")
	if err != nil {
		writeError(w, err)
		return
	}
	library, err := s.placeholders.LoadPlaceholderLibrary(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, placeholder.DependencyTree(name, library))
}
