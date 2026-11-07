package gateway

import (
	"net/http"

	"eucli-box/pkg/types"
)

// 提示词收藏夹：收藏夹是索引层，与提示词库同一套作用域参数规则。
// 收藏夹与条目的增删改都由整库保存表达；结构校验在存储层收口。

func (s *system) handleLoadPromptFolders(w http.ResponseWriter, r *http.Request) {
	library, err := s.promptFolders.LoadPromptFolders(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

func (s *system) handleSavePromptFolders(w http.ResponseWriter, r *http.Request) {
	library, err := decodeJSON[types.PromptFolderLibrary](r)
	if err != nil {
		writeError(w, err)
		return
	}
	saved, err := s.promptFolders.SavePromptFolders(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r), library)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, saved)
}
