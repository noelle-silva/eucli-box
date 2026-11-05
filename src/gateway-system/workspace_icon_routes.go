package gateway

import (
	"net/http"

	"eucli-box/pkg/types"
)

// 工作区身份图标是独立资源：与工作区配置解耦，读写都走本组路由，
// 全量保存工作区的请求不会碰到图标（与角色头像、群组头像同一形态）。
func (s *system) handleSaveWorkspaceIcon(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := pathValue(r, "workspaceID")
	if err != nil {
		writeError(w, err)
		return
	}
	content, err := decodeJSON[types.WorkspaceIconContent](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.workspaces.SaveWorkspaceIcon(r.Context(), workspaceID, content); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleLoadWorkspaceIcon(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := pathValue(r, "workspaceID")
	if err != nil {
		writeError(w, err)
		return
	}
	content, err := s.workspaces.LoadWorkspaceIcon(r.Context(), workspaceID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, content)
}

func (s *system) handleDeleteWorkspaceIcon(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := pathValue(r, "workspaceID")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.workspaces.DeleteWorkspaceIcon(r.Context(), workspaceID); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}
