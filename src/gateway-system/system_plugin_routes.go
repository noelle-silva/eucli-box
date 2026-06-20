package gateway

import (
	"net/http"

	"eucli-box/pkg/types"
)

func (s *system) handleListSystemPlugins(w http.ResponseWriter, r *http.Request) {
	plugins, err := s.systemPlugins.ListPlugins(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, plugins)
}

func (s *system) handleLoadSystemPlugin(w http.ResponseWriter, r *http.Request) {
	pluginID, err := pathValue(r, "pluginID")
	if err != nil {
		writeError(w, err)
		return
	}
	plugin, err := s.systemPlugins.LoadPlugin(r.Context(), pluginID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, plugin)
}

func (s *system) handleSaveSystemPluginUserConfig(w http.ResponseWriter, r *http.Request) {
	pluginID, err := pathValue(r, "pluginID")
	if err != nil {
		writeError(w, err)
		return
	}
	config, err := decodeJSON[types.SystemPluginUserConfig](r)
	if err != nil {
		writeError(w, err)
		return
	}
	plugin, err := s.systemPlugins.SavePluginUserConfig(r.Context(), pluginID, config)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, plugin)
}
