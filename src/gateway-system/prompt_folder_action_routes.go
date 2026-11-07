package gateway

import (
	"net/http"
)

// 收藏夹定点动作：删除、重命名、兄弟排序、夹内条目排序。
// 四个动作都只提交必要参数与顺序，不要求客户端回传整棵树；
// 规矩（子夹上移、集合不许增减、结构完整）全部由存储层判定。

type promptFolderRenameRequest struct {
	Name string `json:"name"`
}

type promptFolderOrderRequest struct {
	FolderIDs []string `json:"folderIds"`
}

type promptOrderRequest struct {
	PromptIDs []string `json:"promptIds"`
}

func (s *system) handleDeletePromptFolder(w http.ResponseWriter, r *http.Request) {
	library, err := s.promptFolders.DeletePromptFolder(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r), r.PathValue("folderID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

func (s *system) handleRenamePromptFolder(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[promptFolderRenameRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	library, err := s.promptFolders.RenamePromptFolder(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r), r.PathValue("folderID"), body.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

// handleReorderPromptFolders 重排同一父级下的收藏夹：parentId 为空表示根级。
func (s *system) handleReorderPromptFolders(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[promptFolderOrderRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	library, err := s.promptFolders.ReorderPromptFolders(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r), r.URL.Query().Get("parentId"), body.FolderIDs)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

// handleReorderFolderPrompts 重排某个收藏夹内的条目：folderId 必须给出。
func (s *system) handleReorderFolderPrompts(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[promptOrderRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	library, err := s.promptFolders.ReorderFolderPrompts(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r), r.URL.Query().Get("folderId"), body.PromptIDs)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}
