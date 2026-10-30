package types

import (
	"encoding/json"
	"testing"
)

// TestModelConfigDecodesLegacyCoordinate 钉住旧单一坐标形态的解码：
// 坐标直接翻译为「一个候选 + 它即默认」，读取方只面对候选池。
func TestModelConfigDecodesLegacyCoordinate(t *testing.T) {
	payload := []byte(`{"coordinate":{"kind":"provider","providerId":"openai-main","modelId":"gpt-4.1"},"temperature":0.7}`)
	var config ModelConfig
	if err := json.Unmarshal(payload, &config); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(config.Candidates) != 1 {
		t.Fatalf("candidates = %#v", config.Candidates)
	}
	candidate := config.Candidates[0]
	if candidate.Coordinate.ProviderID != "openai-main" || candidate.Coordinate.ModelID != "gpt-4.1" {
		t.Fatalf("candidate coordinate = %#v", candidate.Coordinate)
	}
	if candidate.ID != "provider:openai-main:gpt-4.1" {
		t.Fatalf("candidate id = %q", candidate.ID)
	}
	if config.DefaultCandidateID != candidate.ID {
		t.Fatalf("default candidate id = %q", config.DefaultCandidateID)
	}
	if config.Temperature != 0.7 {
		t.Fatalf("temperature = %v", config.Temperature)
	}
}

// TestModelConfigDecodesLegacyModelGroupCoordinate 钉住模型组坐标同样被翻译。
func TestModelConfigDecodesLegacyModelGroupCoordinate(t *testing.T) {
	payload := []byte(`{"coordinate":{"kind":"model_group","groupId":"main-group","modelId":"balanced"}}`)
	var config ModelConfig
	if err := json.Unmarshal(payload, &config); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(config.Candidates) != 1 || config.Candidates[0].ID != "model_group:main-group:balanced" {
		t.Fatalf("candidates = %#v", config.Candidates)
	}
}

// TestModelConfigKeepsCandidatePool 钉住候选池形态原样解码，不做推断。
func TestModelConfigKeepsCandidatePool(t *testing.T) {
	payload := []byte(`{"candidates":[{"id":"a","coordinate":{"kind":"provider","providerId":"p1","modelId":"m1"}},{"id":"b","coordinate":{"kind":"provider","providerId":"p2","modelId":"m2"}}],"defaultCandidateId":"b","temperature":1}`)
	var config ModelConfig
	if err := json.Unmarshal(payload, &config); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(config.Candidates) != 2 || config.DefaultCandidateID != "b" {
		t.Fatalf("config = %#v", config)
	}
	candidate, ok := config.DefaultCandidate()
	if !ok || candidate.Coordinate.ProviderID != "p2" {
		t.Fatalf("default candidate = %#v ok=%v", candidate, ok)
	}
}

// TestModelConfigDefaultCandidateMissing 钉住默认引用不在池内时明确返回 false。
func TestModelConfigDefaultCandidateMissing(t *testing.T) {
	config := ModelConfig{
		Candidates:         []RoleModelCandidate{{ID: "a", Coordinate: ModelCoordinate{ProviderID: "p1", ModelID: "m1"}}},
		DefaultCandidateID: "absent",
	}
	if _, ok := config.DefaultCandidate(); ok {
		t.Fatalf("default candidate should be missing")
	}
}

// TestModelConfigRoundTrip 钉住写盘再读回不改变候选池事实。
func TestModelConfigRoundTrip(t *testing.T) {
	original := ModelConfig{
		Candidates: []RoleModelCandidate{
			{ID: "provider:p1:m1", Coordinate: ModelCoordinate{ProviderID: "p1", ModelID: "m1"}},
			{ID: "model_group:g1:m2", Coordinate: ModelCoordinate{Kind: ModelCoordinateKindGroup, GroupID: "g1", ModelID: "m2"}},
		},
		DefaultCandidateID: "model_group:g1:m2",
		Temperature:        0.5,
	}
	payload, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded ModelConfig
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(decoded.Candidates) != 2 || decoded.DefaultCandidateID != original.DefaultCandidateID {
		t.Fatalf("decoded = %#v", decoded)
	}
	if decoded.Temperature != 0.5 {
		t.Fatalf("temperature = %v", decoded.Temperature)
	}
}
