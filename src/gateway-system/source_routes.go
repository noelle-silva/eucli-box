package gateway

import (
	"encoding/json"
	"net/http"

	"eucli-box/pkg/installsource"
)

// sourceOrFail 按路径中的类别取来源状态视图；未知类别写出错误并返回 false。
func (s *system) sourceOrFail(w http.ResponseWriter, r *http.Request) (InstallSourceSystem, bool) {
	kind := r.PathValue("kind")
	source, ok := s.installSourceFor(kind)
	if !ok {
		writeError(w, gatewayInvalid("不支持的安装来源类别 "+kind, nil))
		return nil, false
	}
	return source, true
}

// handleListSources 返回该类别按注册顺序排列的来源注册表；配置不可用时携带原因照常应答。
func (s *system) handleListSources(w http.ResponseWriter, r *http.Request) {
	source, ok := s.sourceOrFail(w, r)
	if !ok {
		return
	}
	writeData(w, http.StatusOK, installsource.SourcesView{Sources: source.Sources(), Problem: source.Problem()})
}

// handleAddSource 注册来源；名字必填，提供方式缺省为本机，其余位置字段按提供方式校验。
func (s *system) handleAddSource(w http.ResponseWriter, r *http.Request) {
	source, ok := s.sourceOrFail(w, r)
	if !ok {
		return
	}
	var body struct {
		Name     *string `json:"name"`
		Kind     *string `json:"kind"`
		Path     *string `json:"path"`
		IndexURL *string `json:"indexUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == nil {
		writeError(w, gatewayInvalid("注册来源必须提供 name", nil))
		return
	}
	entry := installsource.Source{Name: *body.Name}
	if body.Kind != nil {
		entry.Kind = *body.Kind
	}
	if body.Path != nil {
		entry.Path = *body.Path
	}
	if body.IndexURL != nil {
		entry.IndexURL = *body.IndexURL
	}
	sources, err := source.AddSource(r.Context(), entry)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	writeData(w, http.StatusOK, installsource.SourcesView{Sources: sources})
}

// handleUpdateSource 改名 / 改提供方式 / 改位置；至少提供一项新值。
func (s *system) handleUpdateSource(w http.ResponseWriter, r *http.Request) {
	source, ok := s.sourceOrFail(w, r)
	if !ok {
		return
	}
	var body struct {
		Name        *string `json:"name"`
		NewName     *string `json:"newName"`
		NewKind     *string `json:"newKind"`
		NewPath     *string `json:"newPath"`
		NewIndexURL *string `json:"newIndexUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == nil {
		writeError(w, gatewayInvalid("改来源必须提供 name", nil))
		return
	}
	sources, err := source.UpdateSource(r.Context(), *body.Name, installsource.SourcePatch{
		NewName:     body.NewName,
		NewKind:     body.NewKind,
		NewPath:     body.NewPath,
		NewIndexURL: body.NewIndexURL,
	})
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	writeData(w, http.StatusOK, installsource.SourcesView{Sources: sources})
}

// handleRemoveSource 删除来源。
func (s *system) handleRemoveSource(w http.ResponseWriter, r *http.Request) {
	source, ok := s.sourceOrFail(w, r)
	if !ok {
		return
	}
	var body struct {
		Name *string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == nil {
		writeError(w, gatewayInvalid("删除来源必须提供 name", nil))
		return
	}
	sources, err := source.RemoveSource(r.Context(), *body.Name)
	if err != nil {
		writeError(w, gatewayInvalid(err.Error(), nil))
		return
	}
	writeData(w, http.StatusOK, installsource.SourcesView{Sources: sources})
}
