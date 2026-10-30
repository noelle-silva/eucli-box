package types

import (
	"encoding/json"
	"strings"
)

// RoleModelCandidate 是角色候选模型池中的一项：标识用于被默认选择引用，
// 坐标描述真实可解析的模型来源。
type RoleModelCandidate struct {
	ID         string          `json:"id"`
	Coordinate ModelCoordinate `json:"coordinate"`
}

// ModelConfig 是角色模型配置：候选模型池 + 默认候选引用 + 温度。
// 默认候选只保存引用标识，不复制坐标，候选池是模型的唯一事实源。
type ModelConfig struct {
	Candidates         []RoleModelCandidate `json:"candidates"`
	DefaultCandidateID string               `json:"defaultCandidateId"`
	Temperature        float64              `json:"temperature"`
	TemperatureEnabled *bool                `json:"temperatureEnabled,omitempty"`
}

// DefaultCandidate 返回默认候选；引用为空或指向不存在的候选时返回 false。
func (c ModelConfig) DefaultCandidate() (RoleModelCandidate, bool) {
	id := strings.TrimSpace(c.DefaultCandidateID)
	if id == "" {
		return RoleModelCandidate{}, false
	}
	for _, candidate := range c.Candidates {
		if strings.TrimSpace(candidate.ID) == id {
			return candidate, true
		}
	}
	return RoleModelCandidate{}, false
}

// ModelCandidateKey 返回坐标的规范键：同一坐标在候选池内只应出现一次。
func ModelCandidateKey(coordinate ModelCoordinate) string {
	kind := strings.TrimSpace(coordinate.Kind)
	groupID := strings.TrimSpace(coordinate.GroupID)
	providerID := strings.TrimSpace(coordinate.ProviderID)
	modelID := strings.TrimSpace(coordinate.ModelID)
	if kind == ModelCoordinateKindGroup || groupID != "" {
		return "model_group:" + groupID + ":" + modelID
	}
	return "provider:" + providerID + ":" + modelID
}

// ModelConfigUnmarshal 兼容两种持久化形态：候选池形态与早期的单一坐标形态。
// 早期形态在解码时直接翻译为「一个候选 + 它即默认」，不产生额外迁移动作。
type modelConfigJSON struct {
	Candidates         []RoleModelCandidate `json:"candidates"`
	DefaultCandidateID string               `json:"defaultCandidateId"`
	Coordinate         ModelCoordinate      `json:"coordinate"`
	Temperature        float64              `json:"temperature"`
	TemperatureEnabled *bool                `json:"temperatureEnabled,omitempty"`
}

// UnmarshalJSON 让解码本身成为旧形态的唯一翻译点，读取方只面对候选池。
func (c *ModelConfig) UnmarshalJSON(payload []byte) error {
	var raw modelConfigJSON
	if err := json.Unmarshal(payload, &raw); err != nil {
		return err
	}
	c.Temperature = raw.Temperature
	c.TemperatureEnabled = raw.TemperatureEnabled
	c.Candidates = raw.Candidates
	c.DefaultCandidateID = strings.TrimSpace(raw.DefaultCandidateID)
	if len(c.Candidates) == 0 && hasModelCoordinate(raw.Coordinate) {
		candidate := RoleModelCandidate{ID: ModelCandidateKey(raw.Coordinate), Coordinate: raw.Coordinate}
		c.Candidates = []RoleModelCandidate{candidate}
		c.DefaultCandidateID = candidate.ID
	}
	return nil
}

func hasModelCoordinate(coordinate ModelCoordinate) bool {
	if strings.TrimSpace(coordinate.ModelID) == "" {
		return false
	}
	return strings.TrimSpace(coordinate.GroupID) != "" || strings.TrimSpace(coordinate.ProviderID) != ""
}
