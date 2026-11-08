package gateway

import (
	"net/http"
)

// 收藏夹定点动作：删除、重命名、混排排序、收藏与取消收藏。
// 动作只提交必要参数；规矩（子夹上移、顺序集合一致、重复收藏）全部由存储层判定。

type promptFolderRenameRequest struct {
	Name string `json:"name"`
}

type promptEntryOrderRequest struct {
	EntryOrder []string `json:"entryOrder"`
}

type promptMembershipRequest struct {
	PromptID string `json:"promptId"`
}

type promptFolderMoveRequest struct {
	FromParentID string `json:"fromParentId"`
	ToParentID   string `json:"toParentId"`
}

type promptFolderCollectRequest struct {
	ParentID string `json:"parentId"`
}

type promptMoveToFolderRequest struct {
	FromFolderID string `json:"fromFolderId"`
	ToFolderID   string `json:"toFolderId"`
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

// handleReorderPromptFolderEntries 重排某一层的混排顺序：parentId 为空表示根级。
func (s *system) handleReorderPromptFolderEntries(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[promptEntryOrderRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	library, err := s.promptFolders.ReorderPromptFolderEntries(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r), r.URL.Query().Get("parentId"), body.EntryOrder)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

// handleAddPromptToFolder 把一条提示词收进收藏夹。
func (s *system) handleAddPromptToFolder(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[promptMembershipRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	library, err := s.promptFolders.AddPromptToFolder(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r), r.PathValue("folderID"), body.PromptID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

// handleRemovePromptFromFolder 把一条提示词移出收藏夹。
func (s *system) handleRemovePromptFromFolder(w http.ResponseWriter, r *http.Request) {
	library, err := s.promptFolders.RemovePromptFromFolder(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r), r.PathValue("folderID"), r.PathValue("promptID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

// handleMovePromptFolder 把收藏夹从当前层移动到另一层；空 id 表示根级。
func (s *system) handleMovePromptFolder(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[promptFolderMoveRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	library, err := s.promptFolders.MovePromptFolder(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r), r.PathValue("folderID"), body.FromParentID, body.ToParentID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

// handleAddFolderToFolder 把收藏夹收藏到另一层：只加索引。
func (s *system) handleAddFolderToFolder(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[promptFolderCollectRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	library, err := s.promptFolders.AddFolderToFolder(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r), r.PathValue("folderID"), body.ParentID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

// handleMovePromptToFolder 把提示词从一个收藏夹移动到另一个：收藏夹为空表示根级。
func (s *system) handleMovePromptToFolder(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[promptMoveToFolderRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	library, err := s.promptFolders.MovePromptToFolder(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r), r.PathValue("promptID"), body.FromFolderID, body.ToFolderID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}
