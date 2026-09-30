package everything

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func fixtureConfig() Config {
	return Config{Limits: LimitsConfig{DefaultRequestTimeoutMs: 30000, MaxOutputChars: 30000}}
}

func TestParseSearchRequestRequiresQuery(t *testing.T) {
	if _, err := parseSearchRequest(types.ToolExecutionInput{Arguments: map[string]any{}}, fixtureConfig()); err == nil || !strings.Contains(err.Error(), "query") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSearchRequestResolvesRelativeScopePath(t *testing.T) {
	host := t.TempDir()
	want := filepath.Join(host, "sub")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}
	request, err := parseSearchRequest(types.ToolExecutionInput{
		Arguments:            map[string]any{"query": "notes", "scopePath": "sub"},
		HostWorkingDirectory: host,
	}, fixtureConfig())
	if err != nil {
		t.Fatal(err)
	}
	if request.ScopePath != want {
		t.Fatalf("scopePath = %q, want %q", request.ScopePath, want)
	}
}

func TestParseSearchRequestRejectsMissingScopePath(t *testing.T) {
	if _, err := parseSearchRequest(types.ToolExecutionInput{
		Arguments:            map[string]any{"query": "notes", "scopePath": "missing"},
		HostWorkingDirectory: t.TempDir(),
	}, fixtureConfig()); err == nil || !strings.Contains(err.Error(), "scopePath") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSearchRequestRejectsNonPositiveMaxResults(t *testing.T) {
	for _, value := range []any{0, -1} {
		if _, err := parseSearchRequest(types.ToolExecutionInput{Arguments: map[string]any{"query": "notes", "maxResults": value}}, fixtureConfig()); err == nil || !strings.Contains(err.Error(), "maxResults") {
			t.Fatalf("maxResults=%v err = %v", value, err)
		}
	}
}

func TestParseSearchRequestRejectsNegativeTimeout(t *testing.T) {
	if _, err := parseSearchRequest(types.ToolExecutionInput{Arguments: map[string]any{"query": "notes", "timeoutMs": -1}}, fixtureConfig()); err == nil || !strings.Contains(err.Error(), "timeoutMs") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSearchRequestAcceptsOptionalFields(t *testing.T) {
	scope := t.TempDir()
	request, err := parseSearchRequest(types.ToolExecutionInput{Arguments: map[string]any{
		"query":      "notes",
		"scopePath":  scope,
		"maxResults": 20,
		"timeoutMs":  5000,
		"description": "查找笔记",
	}}, fixtureConfig())
	if err != nil {
		t.Fatal(err)
	}
	if request.Query != "notes" || request.ScopePath != scope || request.MaxResults != 20 || request.TimeoutMs != 5000 || request.Description != "查找笔记" {
		t.Fatalf("request = %#v", request)
	}
	if request.MaxOutputChars != 30000 {
		t.Fatalf("maxOutputChars = %d", request.MaxOutputChars)
	}
}
