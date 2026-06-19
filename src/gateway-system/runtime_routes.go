package gateway

import (
	"net/http"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) handleStartRun(w http.ResponseWriter, r *http.Request) {
	request, err := decodeJSON[types.RunRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := validateRunRequest(request); err != nil {
		writeError(w, err)
		return
	}
	state, err := s.runtime.StartRun(r.Context(), request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusCreated, state)
}

func (s *system) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID, err := pathValue(r, "runID")
	if err != nil {
		writeError(w, err)
		return
	}
	state, err := s.runtime.GetRun(r.Context(), runID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, state)
}

func (s *system) handleListActiveRuns(w http.ResponseWriter, r *http.Request) {
	states, err := s.runtime.ListActiveRuns(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, states)
}

func (s *system) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	runID, err := pathValue(r, "runID")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.runtime.CancelRun(r.Context(), runID); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleListAsyncToolTasks(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	query := types.AsyncToolTaskQuery{
		RoleID:      strings.TrimSpace(values.Get("roleId")),
		GroupID:     strings.TrimSpace(values.Get("groupId")),
		WorkspaceID: strings.TrimSpace(values.Get("workspaceId")),
		SessionID:   strings.TrimSpace(values.Get("sessionId")),
	}
	tasks, err := s.runtime.ListAsyncToolTasks(r.Context(), query)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, tasks)
}

func (s *system) handleToolConfirmation(w http.ResponseWriter, r *http.Request) {
	confirmation, err := decodeJSON[types.ToolConfirmation](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := validateConfirmation(confirmation); err != nil {
		writeError(w, err)
		return
	}
	if err := s.runtime.SubmitToolConfirmation(r.Context(), confirmation); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}
