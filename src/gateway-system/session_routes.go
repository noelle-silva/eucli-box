package gateway

import (
	"net/http"

	"eucli-box/pkg/types"
)

type createSessionRequest struct {
	Title string `json:"title"`
}

type updateSessionTitleRequest struct {
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

func (s *system) handleUpdateSessionTitle(w http.ResponseWriter, r *http.Request) {
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
	request, err := decodeJSON[updateSessionTitleRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	session, err := s.sessions.UpdateSessionTitle(r.Context(), roleID, sessionID, request.Title)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, session)
}

func (s *system) handleUpdateSessionMessage(w http.ResponseWriter, r *http.Request) {
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
	messageID, err := pathValue(r, "messageID")
	if err != nil {
		writeError(w, err)
		return
	}
	request, err := decodeJSON[types.SessionMessagePatch](r)
	if err != nil {
		writeError(w, err)
		return
	}
	message, err := s.sessions.UpdateSessionMessage(r.Context(), roleID, sessionID, messageID, request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, message)
}

func (s *system) handleDeleteSessionMessage(w http.ResponseWriter, r *http.Request) {
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
	messageID, err := pathValue(r, "messageID")
	if err != nil {
		writeError(w, err)
		return
	}
	session, err := s.sessions.DeleteSessionMessage(r.Context(), roleID, sessionID, messageID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, session)
}

func (s *system) handleDeleteSessionMessageSubtree(w http.ResponseWriter, r *http.Request) {
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
	messageID, err := pathValue(r, "messageID")
	if err != nil {
		writeError(w, err)
		return
	}
	session, err := s.sessions.DeleteSessionMessageSubtree(r.Context(), roleID, sessionID, messageID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, session)
}

func (s *system) handleLoadSessionFavorites(w http.ResponseWriter, r *http.Request) {
	favorites, err := s.sessions.LoadSessionFavorites(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, favorites)
}

func (s *system) handleSaveSessionFavorites(w http.ResponseWriter, r *http.Request) {
	favorites, err := decodeJSON[types.SessionFavorites](r)
	if err != nil {
		writeError(w, err)
		return
	}
	saved, err := s.sessions.SaveSessionFavorites(r.Context(), favorites)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, saved)
}

func (s *system) handleLoadSessionAttachmentImage(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Query().Get("path")
	dataURL, err := s.sessions.LoadSessionAttachmentImage(r.Context(), relPath)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, dataURL)
}
