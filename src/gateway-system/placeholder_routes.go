package gateway

import (
	"net/http"

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
	result, err := s.placeholders.ResolveText(r.Context(), request.Text)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (s *system) handlePlaceholderProblems(w http.ResponseWriter, r *http.Request) {
	problems, err := s.placeholders.Problems(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, problems)
}

func (s *system) handlePlaceholderDependencies(w http.ResponseWriter, r *http.Request) {
	name, err := pathValue(r, "name")
	if err != nil {
		writeError(w, err)
		return
	}
	tree, err := s.placeholders.DependencyTree(r.Context(), name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, tree)
}
