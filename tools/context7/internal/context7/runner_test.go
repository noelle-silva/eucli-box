package context7

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

func TestExecuteSearchesLibraries(t *testing.T) {
	var authHeader string
	var libraryName string
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		libraryName = r.URL.Query().Get("libraryName")
		query = r.URL.Query().Get("query")
		if r.Method != http.MethodGet || r.URL.Path != "/search" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"results":[{"id":"/facebook/react","title":"React","description":"UI library","state":"finalized","totalSnippets":2500,"trustScore":9.2,"benchmarkScore":95.5,"versions":["v18.2.0"]}],"searchFilterApplied":false}`))
	}))
	defer server.Close()
	fixture := newContext7Fixture(t, server.URL+"/search", server.URL+"/context", true)

	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"action": "search", "libraryName": "react", "query": "hooks", "maxOutputChars": 24000}, UserConfig: map[string]any{"context7ApiKey": "ctx7sk-test"}, ToolBodyDirectory: fixture.toolDir})

	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if authHeader != "Bearer ctx7sk-test" || libraryName != "react" || query != "hooks" {
		t.Fatalf("request header=%q libraryName=%q query=%q", authHeader, libraryName, query)
	}
	if !strings.Contains(result.Content, "/facebook/react") || !strings.Contains(result.Content, "Trust score: 9.2") || result.Metadata["resultsCount"] != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteFetchesDocs(t *testing.T) {
	var requestType string
	var fast string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestType = r.URL.Query().Get("type")
		fast = r.URL.Query().Get("fast")
		if r.Method != http.MethodGet || r.URL.Path != "/context" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"codeSnippets":[{"codeTitle":"Middleware","codeDescription":"Auth middleware","codeLanguage":"typescript","codeTokens":120,"codeId":"https://example.test/middleware","pageTitle":"Middleware","codeList":[{"language":"typescript","code":"export function middleware() {}"}]}],"infoSnippets":[{"pageId":"https://example.test/info","breadcrumb":"Routing > Middleware","content":"Middleware runs before requests.","contentTokens":40}]}`))
	}))
	defer server.Close()
	fixture := newContext7Fixture(t, server.URL+"/search", server.URL+"/context", true)

	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"action": "docs", "libraryId": "/vercel/next.js", "query": "middleware auth", "fast": true, "maxOutputChars": 24000}, ToolBodyDirectory: fixture.toolDir})

	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if requestType != "json" || fast != "true" {
		t.Fatalf("request type=%q fast=%q", requestType, fast)
	}
	if !strings.Contains(result.Content, "Middleware") || !strings.Contains(result.Content, "export function middleware") || result.Metadata["codeSnippets"] != 1 || result.Metadata["infoSnippets"] != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteRequiresLibraryNameForSearch(t *testing.T) {
	fixture := newContext7Fixture(t, "http://127.0.0.1/search", "http://127.0.0.1/context", true)

	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"action": "search", "query": "hooks"}, ToolBodyDirectory: fixture.toolDir})

	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "libraryName") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteRequiresAPIKeyWhenAnonymousDisabled(t *testing.T) {
	fixture := newContext7Fixture(t, "http://127.0.0.1/search", "http://127.0.0.1/context", false)

	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"action": "search", "libraryName": "react", "query": "hooks"}, ToolBodyDirectory: fixture.toolDir})

	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "requires API key") {
		t.Fatalf("result = %#v", result)
	}
}

type context7Fixture struct {
	toolDir string
}

func newContext7Fixture(t *testing.T, searchEndpoint string, contextEndpoint string, anonymousAllowed bool) context7Fixture {
	t.Helper()
	toolDir := t.TempDir()
	config := Config{
		SearchEndpoint:   searchEndpoint,
		ContextEndpoint:  contextEndpoint,
		APIKeyEnv:        "CONTEXT7_API_KEY",
		APIKeyUserConfig: "context7ApiKey",
		AnonymousAllowed: anonymousAllowed,
		Limits:           LimitsConfig{DefaultTimeoutMs: 30000, MaxTimeoutMs: 120000, MaxOutputChars: 24000},
	}
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal(config) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "config.json"), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	return context7Fixture{toolDir: toolDir}
}
