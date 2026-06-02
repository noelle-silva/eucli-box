package gateway

import (
	"net/http"
	"strings"

	"eucli-box/pkg/types"
)

type stickerCategoryRequest struct {
	CategoryName string `json:"categoryName"`
}

type addStickerRequest struct {
	CategoryName string `json:"categoryName"`
	StickerName  string `json:"stickerName"`
	DataURL      string `json:"dataUrl"`
}

type renameStickerRequest struct {
	CategoryName   string `json:"categoryName"`
	OldStickerName string `json:"oldStickerName"`
	NewStickerName string `json:"newStickerName"`
}

type deleteStickerRequest struct {
	CategoryName string `json:"categoryName"`
	StickerName  string `json:"stickerName"`
}

func (s *system) handleLoadStickerLibrary(w http.ResponseWriter, r *http.Request) {
	library, err := s.stickers.LoadStickerLibrary(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

func (s *system) handleListStickerCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := s.stickers.ListStickerCategories(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, categories)
}

func (s *system) handleCreateStickerCategory(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[stickerCategoryRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	category, err := s.stickers.CreateStickerCategory(r.Context(), request.CategoryName)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusCreated, category)
}

func (s *system) handleLoadStickerCategory(w http.ResponseWriter, r *http.Request) {
	categoryName, err := pathValue(r, "categoryName")
	if err != nil {
		writeError(w, err)
		return
	}
	category, err := s.stickers.LoadStickerCategory(r.Context(), categoryName)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, category)
}

func (s *system) handleDeleteStickerCategory(w http.ResponseWriter, r *http.Request) {
	categoryName, err := pathValue(r, "categoryName")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.stickers.DeleteStickerCategory(r.Context(), categoryName); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleAddSticker(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[addStickerRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	item, err := s.stickers.AddSticker(r.Context(), request.CategoryName, request.StickerName, request.DataURL)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusCreated, item)
}

func (s *system) handleRenameSticker(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[renameStickerRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	item, err := s.stickers.RenameSticker(r.Context(), request.CategoryName, request.OldStickerName, request.NewStickerName)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, item)
}

func (s *system) handleDeleteSticker(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[deleteStickerRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.stickers.DeleteSticker(r.Context(), request.CategoryName, request.StickerName); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleLoadStickerImage(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if relPath == "" {
		writeError(w, gatewayInvalid("sticker image path is required", nil))
		return
	}
	dataURL, err := s.stickers.LoadStickerImage(r.Context(), relPath)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, dataURL)
}

func (s *system) handleGenerateStickerName(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[types.StickerNameRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := s.assist.GenerateStickerName(r.Context(), request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (s *system) handleLoadStickerNamingConfig(w http.ResponseWriter, r *http.Request) {
	config, err := s.stickers.LoadStickerNamingConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, config)
}

func (s *system) handleSaveStickerNamingConfig(w http.ResponseWriter, r *http.Request) {
	config, err := decodeJSON[types.StickerNamingConfig](r)
	if err != nil {
		writeError(w, err)
		return
	}
	saved, err := s.stickers.SaveStickerNamingConfig(r.Context(), config)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, saved)
}
