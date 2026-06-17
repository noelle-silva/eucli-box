package gateway

import (
	"net/http"
	"strings"

	"eucli-box/pkg/types"
)

type createWorkspaceSessionRequest struct {
	Title string `json:"title"`
}

func (s *system) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	workspaces, err := s.workspaces.ListWorkspaces(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, workspaces)
}

func (s *system) handleSaveWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, err := decodeJSON[types.Workspace](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := validateWorkspace(workspace); err != nil {
		writeError(w, err)
		return
	}
	if err := s.workspaces.SaveWorkspace(r.Context(), workspace); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleLoadWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := pathValue(r, "workspaceID")
	if err != nil {
		writeError(w, err)
		return
	}
	workspace, err := s.workspaces.LoadWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, workspace)
}

func (s *system) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := pathValue(r, "workspaceID")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.workspaces.DeleteWorkspace(r.Context(), workspaceID); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleListWorkspaceSessions(w http.ResponseWriter, r *http.Request) {
	workspaceID, roleID, err := workspaceRolePathValues(r)
	if err != nil {
		writeError(w, err)
		return
	}
	sessions, err := s.sessions.ListWorkspaceSessions(r.Context(), workspaceID, roleID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, sessions)
}

func (s *system) handleLoadWorkspaceSession(w http.ResponseWriter, r *http.Request) {
	workspaceID, roleID, sessionID, ok := workspaceRoleSessionPathValues(w, r)
	if !ok {
		return
	}
	session, err := s.sessions.LoadWorkspaceSession(r.Context(), workspaceID, roleID, sessionID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, session)
}

func (s *system) handleCreateWorkspaceSession(w http.ResponseWriter, r *http.Request) {
	workspaceID, roleID, err := workspaceRolePathValues(r)
	if err != nil {
		writeError(w, err)
		return
	}
	request, err := decodeJSON[createWorkspaceSessionRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.workspaces.LoadWorkspace(r.Context(), workspaceID); err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.roles.LoadRole(r.Context(), roleID); err != nil {
		writeError(w, err)
		return
	}
	session, err := s.sessions.CreateWorkspaceSession(r.Context(), workspaceID, roleID, request.Title)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusCreated, session)
}

func (s *system) handleSaveWorkspaceSession(w http.ResponseWriter, r *http.Request) {
	workspaceID, roleID, err := workspaceRolePathValues(r)
	if err != nil {
		writeError(w, err)
		return
	}
	session, err := decodeJSON[types.Session](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := validateWorkspaceSession(workspaceID, session); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(session.RoleID) != roleID {
		writeError(w, gatewayInvalid("session roleId does not match route roleID", nil))
		return
	}
	if _, err := s.workspaces.LoadWorkspace(r.Context(), workspaceID); err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.roles.LoadRole(r.Context(), session.RoleID); err != nil {
		writeError(w, err)
		return
	}
	if err := s.sessions.SaveSession(r.Context(), session); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleDeleteWorkspaceSession(w http.ResponseWriter, r *http.Request) {
	workspaceID, roleID, sessionID, ok := workspaceRoleSessionPathValues(w, r)
	if !ok {
		return
	}
	if err := s.sessions.DeleteWorkspaceSession(r.Context(), workspaceID, roleID, sessionID); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleUpdateWorkspaceSessionTitle(w http.ResponseWriter, r *http.Request) {
	workspaceID, roleID, sessionID, ok := workspaceRoleSessionPathValues(w, r)
	if !ok {
		return
	}
	request, err := decodeJSON[updateSessionTitleRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	session, err := s.sessions.UpdateWorkspaceSessionTitle(r.Context(), workspaceID, roleID, sessionID, request.Title)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, session)
}

func (s *system) handleUpdateWorkspaceSessionMessage(w http.ResponseWriter, r *http.Request) {
	workspaceID, roleID, sessionID, messageID, ok := workspaceRoleSessionMessagePathValues(w, r)
	if !ok {
		return
	}
	request, err := decodeJSON[types.SessionMessagePatch](r)
	if err != nil {
		writeError(w, err)
		return
	}
	message, err := s.sessions.UpdateWorkspaceSessionMessage(r.Context(), workspaceID, roleID, sessionID, messageID, request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, message)
}

func (s *system) handleDeleteWorkspaceSessionMessage(w http.ResponseWriter, r *http.Request) {
	workspaceID, roleID, sessionID, messageID, ok := workspaceRoleSessionMessagePathValues(w, r)
	if !ok {
		return
	}
	session, err := s.sessions.DeleteWorkspaceSessionMessage(r.Context(), workspaceID, roleID, sessionID, messageID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, session)
}

func (s *system) handleDeleteWorkspaceSessionMessageSubtree(w http.ResponseWriter, r *http.Request) {
	workspaceID, roleID, sessionID, messageID, ok := workspaceRoleSessionMessagePathValues(w, r)
	if !ok {
		return
	}
	session, err := s.sessions.DeleteWorkspaceSessionMessageSubtree(r.Context(), workspaceID, roleID, sessionID, messageID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, session)
}

func workspaceRolePathValues(r *http.Request) (string, string, error) {
	workspaceID, err := pathValue(r, "workspaceID")
	if err != nil {
		return "", "", err
	}
	roleID, err := pathValue(r, "roleID")
	if err != nil {
		return "", "", err
	}
	return workspaceID, roleID, nil
}

func workspaceRoleSessionPathValues(w http.ResponseWriter, r *http.Request) (string, string, string, bool) {
	workspaceID, roleID, err := workspaceRolePathValues(r)
	if err != nil {
		writeError(w, err)
		return "", "", "", false
	}
	sessionID, err := pathValue(r, "sessionID")
	if err != nil {
		writeError(w, err)
		return "", "", "", false
	}
	return workspaceID, roleID, sessionID, true
}

func workspaceRoleSessionMessagePathValues(w http.ResponseWriter, r *http.Request) (string, string, string, string, bool) {
	workspaceID, roleID, sessionID, ok := workspaceRoleSessionPathValues(w, r)
	if !ok {
		return "", "", "", "", false
	}
	messageID, err := pathValue(r, "messageID")
	if err != nil {
		writeError(w, err)
		return "", "", "", "", false
	}
	return workspaceID, roleID, sessionID, messageID, true
}
