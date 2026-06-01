package gateway

import (
	"net/http"

	"eucli-box/pkg/types"
)

type createSessionRequest struct {
	Title string `json:"title"`
}

func (s *system) handleListSessions(w http.ResponseWriter, r *http.Request) {
	roleID, err := pathValue(r, "roleID")
	if err != nil {
		writeError(w, err)
		return
	}
	sessions, err := s.sessions.ListSessions(r.Context(), roleID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, sessions)
}

func (s *system) handleLoadSession(w http.ResponseWriter, r *http.Request) {
	roleID, err := pathValue(r, "roleID")
	if err != nil {
		writeError(w, err)
		return
	}
	sessionID, err := pathValue(r, "sessionID")
	if err != nil {
		writeError(w, err)
		return
	}
	session, err := s.sessions.LoadSession(r.Context(), roleID, sessionID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, session)
}

func (s *system) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	roleID, err := pathValue(r, "roleID")
	if err != nil {
		writeError(w, err)
		return
	}
	request, err := decodeJSON[createSessionRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.roles.LoadRole(r.Context(), roleID); err != nil {
		writeError(w, err)
		return
	}
	session, err := s.sessions.CreateSession(r.Context(), roleID, request.Title)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusCreated, session)
}

func (s *system) handleSaveSession(w http.ResponseWriter, r *http.Request) {
	roleID, err := pathValue(r, "roleID")
	if err != nil {
		writeError(w, err)
		return
	}
	session, err := decodeJSON[types.Session](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := validateSession(roleID, session); err != nil {
		writeError(w, err)
		return
	}
	if err := s.sessions.SaveSession(r.Context(), session); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	roleID, err := pathValue(r, "roleID")
	if err != nil {
		writeError(w, err)
		return
	}
	sessionID, err := pathValue(r, "sessionID")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.sessions.DeleteSession(r.Context(), roleID, sessionID); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}
