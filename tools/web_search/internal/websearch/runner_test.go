package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func TestExecuteSearchesTavilyProvider(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_, _ = w.Write([]byte(`{"query":"golang","results":[{"title":"Go","url":"https://go.dev","content":"Go language","score":0.95}]}`))
	}))
	defer server.Close()
	fixture := newWebSearchFixture(t, server.URL, "http://127.0.0.1/anysearch")
	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"query": "golang", "provider": "tavily", "maxResults": 3, "searchDepth": "advanced"}, UserConfig: map[string]any{"tavilyApiKey": "tvly-test"}, DefaultConfig: map[string]any{"maxOutputChars": 20000}, ToolBodyDirectory: fixture.toolDir})
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if received["api_key"] != "tvly-test" || received["search_depth"] != "advanced" || received["max_results"] != float64(3) {
		t.Fatalf("request = %#v", received)
	}
	if !strings.Contains(result.Content, "[Go](https://go.dev)") || result.Metadata["provider"] != "tavily" {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteSearchesAnySearchProviderAnonymously(t *testing.T) {
	var authHeader string
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"results":[{"title":"AnySearch","url":"https://anysearch.com","snippet":"AI search"}],"metadata":{"request_id":"req-1","total_results":1,"search_time_ms":42}}}`))
	}))
	defer server.Close()
	fixture := newWebSearchFixture(t, "http://127.0.0.1/tavily", server.URL)
	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"query": "agent search", "provider": "anysearch", "domain": "code", "tag": "code.doc", "contentTypes": []any{"web", "doc"}}, DefaultConfig: map[string]any{"maxOutputChars": 20000}, ToolBodyDirectory: fixture.toolDir})
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if authHeader != "" {
		t.Fatalf("auth header = %q", authHeader)
	}
	if received["domain"] != "code" || received["tag"] != "code.doc" {
		t.Fatalf("request = %#v", received)
	}
	providerMetadata, ok := result.Metadata["providerMetadata"].(map[string]any)
	if !ok || providerMetadata["request_id"] != "req-1" || !strings.Contains(result.Content, "AnySearch") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteUsesAnySearchDefaultProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"results":[{"title":"Default","url":"https://anysearch.com","snippet":"default provider"}],"metadata":{}}}`))
	}))
	defer server.Close()
	fixture := newWebSearchFixture(t, "http://127.0.0.1/tavily", server.URL)
	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"query": "default provider"}, DefaultConfig: map[string]any{"provider": "anysearch", "maxOutputChars": 20000}, ToolBodyDirectory: fixture.toolDir})
	if result.Status != types.ToolStatusSuccess || result.Metadata["provider"] != "anysearch" || !strings.Contains(result.Content, "Default") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteFailsWhenTavilyKeyMissing(t *testing.T) {
	fixture := newWebSearchFixture(t, "http://127.0.0.1/tavily", "http://127.0.0.1/anysearch")
	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"query": "golang", "provider": "tavily"}, ToolBodyDirectory: fixture.toolDir})
	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "requires API key") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteRejectsProviderMaxResults(t *testing.T) {
	fixture := newWebSearchFixture(t, "http://127.0.0.1/tavily", "http://127.0.0.1/anysearch")
	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"query": "golang", "provider": "tavily", "maxResults": 21}, UserConfig: map[string]any{"tavilyApiKey": "tvly-test"}, ToolBodyDirectory: fixture.toolDir})
	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "between 1 and 20") {
		t.Fatalf("result = %#v", result)
	}
}

type webSearchFixture struct {
	toolDir string
}

func newWebSearchFixture(t *testing.T, tavilyEndpoint string, anySearchEndpoint string) webSearchFixture {
	t.Helper()
	toolDir := t.TempDir()
	config := Config{
		DefaultProvider:             "anysearch",
		AllowModelProviderSelection: true,
		Providers: []ProviderConfig{
			{ID: "tavily", Kind: providerKindTavily, Enabled: true, Endpoint: tavilyEndpoint, APIKeyEnv: "TAVILY_API_KEY", APIKeyUserConfig: "tavilyApiKey", MaxResults: 20},
			{ID: "anysearch", Kind: providerKindAnySearch, Enabled: true, Endpoint: anySearchEndpoint, APIKeyEnv: "ANYSEARCH_API_KEY", APIKeyUserConfig: "anysearchApiKey", AnonymousAllowed: true, MaxResults: 100},
		},
		Limits: LimitsConfig{DefaultTimeoutMs: 30000, MaxTimeoutMs: 120000, DefaultMaxResults: 5, MaxResults: 100, MaxOutputChars: 20000},
	}
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal(config) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "config.json"), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	return webSearchFixture{toolDir: toolDir}
}
