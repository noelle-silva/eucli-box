package gateway

import (
	"encoding/json"
	"net/http"

	"eucli-box/pkg/installsource"
)

// installSourceOrFail 按路径中的类别取来源状态视图；未知类别写出错误并返回 false。
func (s *system) installSourceOrFail(w http.ResponseWriter, r *http.Request) (InstallSourceSystem, bool) {
	kind := r.PathValue("kind")
	source, ok := s.installSourceFor(kind)
	if !ok {
		writeError(w, gatewayInvalid("不支持的安装来源类别 "+kind, nil))
		return nil, false
	}
	return source, true
}

// handleInstallSource 返回该类别当前来源选择；配置不可用时携带原因照常应答。
func (s *system) handleInstallSource(w http.ResponseWriter, r *http.Request) {
	source, ok := s.installSourceOrFail(w, r)
	if !ok {
		return
	}
	writeData(w, http.StatusOK, installsource.SourceView{Source: source.CurrentSource(), Problem: source.Problem()})
}

// handleSetInstallSource 切换该类别来源；只接受官方保留字或该类已注册货架。
func (s *system) handleSetInstallSource(w http.ResponseWriter, r *http.Request) {
	source, ok := s.installSourceOrFail(w, r)
	if !ok {
		return
	}
	var body struct {
		Source *string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Source == nil {
		writeError(w, gatewayInvalid(`请求体必须是 {"source": "official" 或货架名字}`, nil))
		return
	}
	next, err := source.SetSource(r.Context(), *body.Source)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	writeData(w, http.StatusOK, installsource.SourceView{Source: next})
}

// handleListShelves 返回该类别按注册顺序排列的货架注册表；配置不可用时携带原因照常应答。
func (s *system) handleListShelves(w http.ResponseWriter, r *http.Request) {
	source, ok := s.installSourceOrFail(w, r)
	if !ok {
		return
	}
	writeData(w, http.StatusOK, installsource.ShelvesView{Shelves: source.Shelves(), Problem: source.Problem()})
}

// handleAddShelf 注册该类别货架；名字与路径必须同时提供。
func (s *system) handleAddShelf(w http.ResponseWriter, r *http.Request) {
	source, ok := s.installSourceOrFail(w, r)
	if !ok {
		return
	}
	var body struct {
		Name *string `json:"name"`
		Path *string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == nil || body.Path == nil {
		writeError(w, gatewayInvalid("注册货架必须提供 name 与 path", nil))
		return
	}
	shelves, err := source.AddShelf(r.Context(), *body.Name, *body.Path)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	writeData(w, http.StatusOK, installsource.ShelvesView{Shelves: shelves})
}

// handleUpdateShelf 改名 / 改路径；newName 与 newPath 至少提供一项。
func (s *system) handleUpdateShelf(w http.ResponseWriter, r *http.Request) {
	source, ok := s.installSourceOrFail(w, r)
	if !ok {
		return
	}
	var body struct {
		Name    *string `json:"name"`
		NewName *string `json:"newName"`
		NewPath *string `json:"newPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == nil {
		writeError(w, gatewayInvalid("改货架必须提供 name", nil))
		return
	}
	shelves, err := source.UpdateShelf(r.Context(), *body.Name, body.NewName, body.NewPath)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	writeData(w, http.StatusOK, installsource.ShelvesView{Shelves: shelves})
}

// handleRemoveShelf 删除货架。
func (s *system) handleRemoveShelf(w http.ResponseWriter, r *http.Request) {
	source, ok := s.installSourceOrFail(w, r)
	if !ok {
		return
	}
	var body struct {
		Name *string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == nil {
		writeError(w, gatewayInvalid("删除货架必须提供 name", nil))
		return
	}
	shelves, err := source.RemoveShelf(r.Context(), *body.Name)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	writeData(w, http.StatusOK, installsource.ShelvesView{Shelves: shelves})
}
