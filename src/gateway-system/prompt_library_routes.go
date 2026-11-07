package gateway

import (
	"net/http"

	"eucli-box/pkg/types"
)

// 提示词仓库：一组端点承载三个作用域，scope 由路径给出，归属 id 由查询参数给出。
// 作用域合法性、归属存在性与库内容校验都在存储层收口，网关只做搬运。

func (s *system) handleLoadPromptLibrary(w http.ResponseWriter, r *http.Request) {
	library, err := s.promptLibraries.LoadPromptLibrary(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, library)
}

func (s *system) handleSavePromptLibrary(w http.ResponseWriter, r *http.Request) {
	library, err := decodeJSON[types.PromptLibrary](r)
	if err != nil {
		writeError(w, err)
		return
	}
	saved, err := s.promptLibraries.SavePromptLibrary(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r), library)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, saved)
}

func (s *system) handleDeletePrompt(w http.ResponseWriter, r *http.Request) {
	updated, err := s.promptLibraries.DeletePrompt(r.Context(), promptLibraryScope(r), promptLibraryOwnerID(r), r.PathValue("promptID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, updated)
}

func promptLibraryScope(r *http.Request) string {
	return r.PathValue("scope")
}

// promptLibraryOwnerID 返回作用域的归属 id；全局作用域忽略该参数。
func promptLibraryOwnerID(r *http.Request) string {
	return r.URL.Query().Get("ownerId")
}
