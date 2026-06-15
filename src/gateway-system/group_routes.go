package gateway

import (
	"net/http"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) handleListChatGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.groups.ListChatGroups(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, groups)
}

func (s *system) handleSaveChatGroup(w http.ResponseWriter, r *http.Request) {
	group, err := decodeJSON[types.ChatGroup](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.validateChatGroup(r, group); err != nil {
		writeError(w, err)
		return
	}
	if err := s.groups.SaveChatGroup(r.Context(), group); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleLoadChatGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := pathValue(r, "groupID")
	if err != nil {
		writeError(w, err)
		return
	}
	group, err := s.groups.LoadChatGroup(r.Context(), groupID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, group)
}

func (s *system) handleDeleteChatGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := pathValue(r, "groupID")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.groups.DeleteChatGroup(r.Context(), groupID); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleSaveChatGroupAvatar(w http.ResponseWriter, r *http.Request) {
	groupID, err := pathValue(r, "groupID")
	if err != nil {
		writeError(w, err)
		return
	}
	payload, err := decodeJSON[struct {
		DataURL string `json:"dataUrl"`
	}](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.groups.SaveChatGroupAvatar(r.Context(), groupID, payload.DataURL); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleLoadChatGroupAvatar(w http.ResponseWriter, r *http.Request) {
	groupID, err := pathValue(r, "groupID")
	if err != nil {
		writeError(w, err)
		return
	}
	dataURL, err := s.groups.LoadChatGroupAvatar(r.Context(), groupID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, dataURL)
}

func (s *system) handleDeleteChatGroupAvatar(w http.ResponseWriter, r *http.Request) {
	groupID, err := pathValue(r, "groupID")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.groups.DeleteChatGroupAvatar(r.Context(), groupID); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) validateChatGroup(r *http.Request, group types.ChatGroup) error {
	if strings.TrimSpace(group.ID) == "" {
		return gatewayInvalid("group id is required", nil)
	}
	if strings.TrimSpace(group.Name) == "" {
		return gatewayInvalid("group name is required", nil)
	}
	if len(group.MemberRoleIDs) == 0 {
		return gatewayInvalid("group must contain at least one role", nil)
	}
	seen := map[string]struct{}{}
	for _, roleID := range group.MemberRoleIDs {
		roleID = strings.TrimSpace(roleID)
		if roleID == "" {
			return gatewayInvalid("group member role id is required", nil)
		}
		if _, ok := seen[roleID]; ok {
			return gatewayInvalid("group member role id is duplicated", nil)
		}
		seen[roleID] = struct{}{}
		if _, err := s.roles.LoadRole(r.Context(), roleID); err != nil {
			return err
		}
	}
	return nil
}
