package gateway

import (
	"encoding/json"
	"net/http"

	"eucli-box/pkg/installsource"
)

// handleInstallSource 返回当前来源选择；配置不可用时携带原因照常应答。
func (s *system) handleInstallSource(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, installsource.SourceView{Source: s.config.InstallSource.CurrentSource(), Problem: s.config.InstallSource.Problem()})
}

// handleSetInstallSource 切换来源；只接受官方保留字或已注册货架。
func (s *system) handleSetInstallSource(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source *string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Source == nil {
		writeError(w, gatewayInvalid(`请求体必须是 {"source": "official" 或货架名字}`, nil))
		return
	}
	next, err := s.config.InstallSource.SetSource(r.Context(), *body.Source)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	writeData(w, http.StatusOK, installsource.SourceView{Source: next})
}

// handleListShelves 返回按注册顺序排列的货架注册表；配置不可用时携带原因照常应答。
func (s *system) handleListShelves(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, installsource.ShelvesView{Shelves: s.config.InstallSource.Shelves(), Problem: s.config.InstallSource.Problem()})
}

// handleAddShelf 注册货架；名字与路径必须同时提供。
func (s *system) handleAddShelf(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name *string `json:"name"`
		Path *string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == nil || body.Path == nil {
		writeError(w, gatewayInvalid("注册货架必须提供 name 与 path", nil))
		return
	}
	shelves, err := s.config.InstallSource.AddShelf(r.Context(), *body.Name, *body.Path)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	writeData(w, http.StatusOK, installsource.ShelvesView{Shelves: shelves})
}

// handleUpdateShelf 改名 / 改路径；newName 与 newPath 至少提供一项。
func (s *system) handleUpdateShelf(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    *string `json:"name"`
		NewName *string `json:"newName"`
		NewPath *string `json:"newPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == nil {
		writeError(w, gatewayInvalid("改货架必须提供 name", nil))
		return
	}
	shelves, err := s.config.InstallSource.UpdateShelf(r.Context(), *body.Name, body.NewName, body.NewPath)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	writeData(w, http.StatusOK, installsource.ShelvesView{Shelves: shelves})
}

// handleRemoveShelf 删除货架。
func (s *system) handleRemoveShelf(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name *string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == nil {
		writeError(w, gatewayInvalid("删除货架必须提供 name", nil))
		return
	}
	shelves, err := s.config.InstallSource.RemoveShelf(r.Context(), *body.Name)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	writeData(w, http.StatusOK, installsource.ShelvesView{Shelves: shelves})
}
