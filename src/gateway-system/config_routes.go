package gateway

import (
	"net/http"

	"eucli-box/pkg/types"
)

func (s *system) handleListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := s.roles.ListRoles(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, roles)
}

func (s *system) handleSaveRole(w http.ResponseWriter, r *http.Request) {
	role, err := decodeJSON[types.Role](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := validateRole(role); err != nil {
		writeError(w, err)
		return
	}
	if err := s.roles.SaveRole(r.Context(), role); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleLoadRole(w http.ResponseWriter, r *http.Request) {
	roleID, err := pathValue(r, "roleID")
	if err != nil {
		writeError(w, err)
		return
	}
	role, err := s.roles.LoadRole(r.Context(), roleID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, role)
}

func (s *system) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	roleID, err := pathValue(r, "roleID")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.roles.DeleteRole(r.Context(), roleID); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleSaveRoleAvatar(w http.ResponseWriter, r *http.Request) {
	roleID, err := pathValue(r, "roleID")
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
	if err := s.roles.SaveRoleAvatar(r.Context(), roleID, payload.DataURL); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleLoadRoleAvatar(w http.ResponseWriter, r *http.Request) {
	roleID, err := pathValue(r, "roleID")
	if err != nil {
		writeError(w, err)
		return
	}
	dataURL, err := s.roles.LoadRoleAvatar(r.Context(), roleID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, dataURL)
}

func (s *system) handleDeleteRoleAvatar(w http.ResponseWriter, r *http.Request) {
	roleID, err := pathValue(r, "roleID")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.roles.DeleteRoleAvatar(r.Context(), roleID); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleListProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := s.providers.ListProviders(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, providers)
}

func (s *system) handleSaveProvider(w http.ResponseWriter, r *http.Request) {
	provider, err := decodeJSON[types.Provider](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := validateProvider(provider); err != nil {
		writeError(w, err)
		return
	}
	if err := s.providers.SaveProvider(r.Context(), provider); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleLoadProvider(w http.ResponseWriter, r *http.Request) {
	providerID, err := pathValue(r, "providerID")
	if err != nil {
		writeError(w, err)
		return
	}
	provider, err := s.providers.LoadProvider(r.Context(), providerID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, provider)
}

func (s *system) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	providerID, err := pathValue(r, "providerID")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.providers.DeleteProvider(r.Context(), providerID); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleLoadModelRequestConfig(w http.ResponseWriter, r *http.Request) {
	config, err := s.providers.LoadModelRequestConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, config)
}

func (s *system) handleSaveModelRequestConfig(w http.ResponseWriter, r *http.Request) {
	config, err := decodeJSON[types.ModelRequestConfig](r)
	if err != nil {
		writeError(w, err)
		return
	}
	saved, err := s.providers.SaveModelRequestConfig(r.Context(), config)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, saved)
}

func (s *system) handleRefreshProviderModels(w http.ResponseWriter, r *http.Request) {
	providerID, err := pathValue(r, "providerID")
	if err != nil {
		writeError(w, err)
		return
	}
	models, err := s.providers.RefreshModels(r.Context(), providerID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, models)
}

func (s *system) handleListTools(w http.ResponseWriter, r *http.Request) {
	tools, err := s.tools.ListTools(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, tools)
}

func (s *system) handleSaveTool(w http.ResponseWriter, r *http.Request) {
	tool, err := decodeJSON[types.ToolDefinition](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := validateTool(tool); err != nil {
		writeError(w, err)
		return
	}
	if err := s.tools.SaveTool(r.Context(), tool); err != nil {
		writeError(w, err)
		return
	}
	writeNoContent(w)
}

func (s *system) handleLoadTool(w http.ResponseWriter, r *http.Request) {
	toolID, err := pathValue(r, "toolID")
	if err != nil {
		writeError(w, err)
		return
	}
	tool, err := s.tools.LoadTool(r.Context(), toolID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, tool)
}

func (s *system) handleSaveToolUserConfig(w http.ResponseWriter, r *http.Request) {
	toolID, err := pathValue(r, "toolID")
	if err != nil {
		writeError(w, err)
		return
	}
	request, err := decodeJSON[toolUserConfigRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if request.UserConfig == nil {
		writeError(w, gatewayInvalid("userConfig must be an object", nil))
		return
	}
	tool, err := s.tools.SaveToolUserConfig(r.Context(), toolID, request.UserConfig)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, tool)
}

type toolUserConfigRequest struct {
	UserConfig map[string]any `json:"userConfig"`
}
