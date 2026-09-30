package everything

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func writeConfigFile(t *testing.T, dir string, payload string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfigRejectsInvalidPayloads(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{"missing", `{}`, "defaultRequestTimeoutMs"},
		{"zero timeout", `{"limits":{"defaultRequestTimeoutMs":0,"maxOutputChars":1000}}`, "defaultRequestTimeoutMs"},
		{"zero output", `{"limits":{"defaultRequestTimeoutMs":1000,"maxOutputChars":0}}`, "maxOutputChars"},
		{"unknown field", `{"limits":{"defaultRequestTimeoutMs":1000,"maxOutputChars":1000},"extra":1}`, "decode"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			writeConfigFile(t, dir, testCase.payload)
			if _, err := loadConfig(dir); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want containing %q", err, testCase.want)
			}
		})
	}
}

func TestLoadConfigAcceptsValidPayload(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `{"limits":{"defaultRequestTimeoutMs":30000,"maxOutputChars":30000}}`)
	config, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if config.Limits.DefaultRequestTimeoutMs != 30000 || config.Limits.MaxOutputChars != 30000 {
		t.Fatalf("config = %#v", config)
	}
}

func TestLoadUserConfigNormalizesEndpointAndKey(t *testing.T) {
	input := types.ToolExecutionInput{UserConfig: map[string]any{
		"endpoint": " http://127.0.0.1:43210/ ",
		"key":      " secret ",
	}}
	connection, err := loadUserConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	if connection.Endpoint != "http://127.0.0.1:43210" || connection.Key != "secret" {
		t.Fatalf("connection = %#v", connection)
	}
}

func TestLoadUserConfigRejectsInvalidPayloads(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]any
		want   string
	}{
		{"missing endpoint", map[string]any{"key": "k"}, "缺少访问地址"},
		{"missing key", map[string]any{"endpoint": "http://127.0.0.1:1"}, "缺少访问钥匙"},
		{"bad scheme", map[string]any{"endpoint": "ftp://127.0.0.1:1", "key": "k"}, "只支持 http/https"},
		{"endpoint with path", map[string]any{"endpoint": "http://127.0.0.1:1/rpc", "key": "k"}, "纯地址"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := loadUserConfig(types.ToolExecutionInput{UserConfig: testCase.config})
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want containing %q", err, testCase.want)
			}
		})
	}
}

func TestEffectiveMaxOutputCharsAppliesUserAndArgument(t *testing.T) {
	config := Config{Limits: LimitsConfig{MaxOutputChars: 50000}}
	if limit, err := effectiveMaxOutputChars(types.ToolExecutionInput{}, config); err != nil || limit != 50000 {
		t.Fatalf("base limit = %d, err = %v", limit, err)
	}
	userLowered := types.ToolExecutionInput{UserConfig: map[string]any{"maxOutputChars": 1000}}
	if limit, err := effectiveMaxOutputChars(userLowered, config); err != nil || limit != 1000 {
		t.Fatalf("user limit = %d, err = %v", limit, err)
	}
	argumentLowered := types.ToolExecutionInput{
		UserConfig: map[string]any{"maxOutputChars": 1000},
		Arguments:  map[string]any{"maxOutputChars": 500},
	}
	if limit, err := effectiveMaxOutputChars(argumentLowered, config); err != nil || limit != 500 {
		t.Fatalf("argument limit = %d, err = %v", limit, err)
	}
	if _, err := effectiveMaxOutputChars(types.ToolExecutionInput{UserConfig: map[string]any{"maxOutputChars": 60000}}, config); err == nil || !strings.Contains(err.Error(), "between 1 and 50000") {
		t.Fatalf("user err = %v", err)
	}
	exceedingArgument := types.ToolExecutionInput{
		UserConfig: map[string]any{"maxOutputChars": 1000},
		Arguments:  map[string]any{"maxOutputChars": 2000},
	}
	if _, err := effectiveMaxOutputChars(exceedingArgument, config); err == nil || !strings.Contains(err.Error(), "between 1 and 1000") {
		t.Fatalf("argument err = %v", err)
	}
}
