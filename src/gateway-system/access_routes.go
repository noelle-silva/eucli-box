package gateway

import (
	"net/http"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) handleListPersistentPorts(w http.ResponseWriter, r *http.Request) {
	if s.access == nil {
		writeError(w, gatewayDependencyFailed("长期访问系统未初始化", nil))
		return
	}
	ports, err := s.access.ListPorts(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, ports)
}

func (s *system) handleAddPersistentPort(w http.ResponseWriter, r *http.Request) {
	if s.access == nil {
		writeError(w, gatewayDependencyFailed("长期访问系统未初始化", nil))
		return
	}
	request, err := decodeJSON[struct {
		Name string `json:"name"`
		Port int    `json:"port"`
	}](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		writeError(w, gatewayInvalid("长期端口名称不能为空", nil))
		return
	}
	if request.Port < 1 || request.Port > 65535 {
		writeError(w, gatewayInvalid("长期端口号必须在 1-65535 之间", nil))
		return
	}
	port, err := s.access.AddPort(r.Context(), request.Name, request.Port)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), err))
		return
	}
	writeData(w, http.StatusCreated, port)
}

func (s *system) handleEnablePersistentPort(w http.ResponseWriter, r *http.Request) {
	if s.access == nil {
		writeError(w, gatewayDependencyFailed("长期访问系统未初始化", nil))
		return
	}
	id, err := pathValue(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	port, err := s.access.EnablePort(r.Context(), id)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), err))
		return
	}
	writeData(w, http.StatusOK, port)
}

func (s *system) handleDisablePersistentPort(w http.ResponseWriter, r *http.Request) {
	if s.access == nil {
		writeError(w, gatewayDependencyFailed("长期访问系统未初始化", nil))
		return
	}
	id, err := pathValue(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	port, err := s.access.DisablePort(r.Context(), id)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), err))
		return
	}
	writeData(w, http.StatusOK, port)
}

func (s *system) handleDeletePersistentPort(w http.ResponseWriter, r *http.Request) {
	if s.access == nil {
		writeError(w, gatewayDependencyFailed("长期访问系统未初始化", nil))
		return
	}
	id, err := pathValue(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.access.DeletePort(r.Context(), id); err != nil {
		writeError(w, gatewayInvalid(err.Error(), err))
		return
	}
	writeNoContent(w)
}

func (s *system) handleListPersistentKeys(w http.ResponseWriter, r *http.Request) {
	if s.access == nil {
		writeError(w, gatewayDependencyFailed("长期访问系统未初始化", nil))
		return
	}
	keys, err := s.access.ListKeys(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, keys)
}

func (s *system) handleAddPersistentKey(w http.ResponseWriter, r *http.Request) {
	if s.access == nil {
		writeError(w, gatewayDependencyFailed("长期访问系统未初始化", nil))
		return
	}
	request, err := decodeJSON[struct {
		Name      string  `json:"name"`
		ExpiresAt *string `json:"expiresAt"`
	}](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		writeError(w, gatewayInvalid("长期 Key 名称不能为空", nil))
		return
	}
	created, err := s.access.CreateKey(r.Context(), request.Name, request.ExpiresAt)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), err))
		return
	}
	writeData(w, http.StatusCreated, created)
}

func (s *system) handleRevealPersistentKey(w http.ResponseWriter, r *http.Request) {
	if s.access == nil {
		writeError(w, gatewayDependencyFailed("长期访问系统未初始化", nil))
		return
	}
	id, err := pathValue(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	plain, err := s.access.RevealKey(r.Context(), id)
	if err != nil {
		writeError(w, gatewayDependencyFailed(err.Error(), err))
		return
	}
	writeData(w, http.StatusOK, map[string]string{"id": id, "plainKey": plain})
}

func (s *system) handleEnablePersistentKey(w http.ResponseWriter, r *http.Request) {
	s.setPersistentKeyEnabled(w, r, true)
}

func (s *system) handleDisablePersistentKey(w http.ResponseWriter, r *http.Request) {
	s.setPersistentKeyEnabled(w, r, false)
}

func (s *system) setPersistentKeyEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	if s.access == nil {
		writeError(w, gatewayDependencyFailed("长期访问系统未初始化", nil))
		return
	}
	id, err := pathValue(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.access.SetKeyEnabled(r.Context(), id, enabled); err != nil {
		writeError(w, gatewayInvalid(err.Error(), err))
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"enabled": enabled})
}

func (s *system) handleSetPersistentKeyExpiration(w http.ResponseWriter, r *http.Request) {
	if s.access == nil {
		writeError(w, gatewayDependencyFailed("长期访问系统未初始化", nil))
		return
	}
	id, err := pathValue(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	request, err := decodeJSON[struct {
		ExpiresAt *string `json:"expiresAt"`
	}](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.access.SetKeyExpiration(r.Context(), id, request.ExpiresAt); err != nil {
		writeError(w, gatewayInvalid(err.Error(), err))
		return
	}
	writeData(w, http.StatusOK, types.PersistentKeyView{ID: id})
}

func (s *system) handleDeletePersistentKey(w http.ResponseWriter, r *http.Request) {
	if s.access == nil {
		writeError(w, gatewayDependencyFailed("长期访问系统未初始化", nil))
		return
	}
	id, err := pathValue(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.access.DeleteKey(r.Context(), id); err != nil {
		writeError(w, gatewayInvalid(err.Error(), err))
		return
	}
	writeNoContent(w)
}
