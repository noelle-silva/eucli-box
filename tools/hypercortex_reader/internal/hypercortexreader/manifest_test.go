package hypercortexreader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestToolManifestMatchesImplementedActions 固定清单与实现的唯一事实：
// 清单声明的动作枚举必须与代码支持的动作集合完全一致，随包配置必须声明输出上限。
func TestToolManifestMatchesImplementedActions(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "tool.json")
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read tool.json: %v", err)
	}
	var manifest struct {
		ID                string `json:"id"`
		Version           string `json:"version"`
		PromptDescription string `json:"promptDescription"`
		InputSchema       struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		} `json:"inputSchema"`
		UserConfigSchema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"userConfigSchema"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("decode tool.json: %v", err)
	}
	if manifest.ID != "hypercortex_reader" || manifest.Version != "0.1.1" {
		t.Fatalf("manifest identity = %s/%s", manifest.ID, manifest.Version)
	}
	if strings.TrimSpace(manifest.PromptDescription) == "" {
		t.Fatal("promptDescription must not be empty")
	}
	declared := append([]string{}, manifest.InputSchema.Properties["action"].Enum...)
	implemented := []string{
		actionSearchNotes, actionReadNote, actionNoteRelations, actionListFavorites,
		actionListRepos, actionSearchAssets, actionListAssets, actionListTrash,
	}
	sort.Strings(declared)
	sort.Strings(implemented)
	if strings.Join(declared, ",") != strings.Join(implemented, ",") {
		t.Fatalf("declared actions = %v, implemented = %v", declared, implemented)
	}
	declaredConfig := make([]string, 0, len(manifest.UserConfigSchema.Properties))
	for key := range manifest.UserConfigSchema.Properties {
		declaredConfig = append(declaredConfig, key)
	}
	expectedConfig := []string{"defaultRepo", "endpoint", "maxOutputChars", "repos"}
	sort.Strings(declaredConfig)
	if strings.Join(declaredConfig, ",") != strings.Join(expectedConfig, ",") {
		t.Fatalf("declared user config = %v, expected = %v", declaredConfig, expectedConfig)
	}
	if _, ok := manifest.InputSchema.Properties["maxOutputChars"]; !ok {
		t.Fatal("inputSchema must declare maxOutputChars")
	}

	configPath := filepath.Join("..", "..", "config.json")
	configPayload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	var bundled Config
	if err := json.Unmarshal(configPayload, &bundled); err != nil {
		t.Fatalf("decode config.json: %v", err)
	}
	if bundled.Limits.MaxOutputChars <= 0 {
		t.Fatalf("config.json limits.maxOutputChars = %d", bundled.Limits.MaxOutputChars)
	}
}
