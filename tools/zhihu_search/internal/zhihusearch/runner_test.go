package zhihusearch

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

func TestExecuteSearchesZhihuContent(t *testing.T) {
	var path string
	var query string
	var count string
	var authHeader string
	var timestampHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		query = r.URL.Query().Get("Query")
		count = r.URL.Query().Get("Count")
		authHeader = r.Header.Get("Authorization")
		timestampHeader = r.Header.Get("X-Request-Timestamp")
		_, _ = w.Write([]byte(`{"Code":0,"Message":"success","Data":{"Items":[{"Title":"AI Agent 应用实践","Url":"https://www.zhihu.com/question/1","AuthorName":"知乎用户","ContentText":"实践摘要","VoteUpCount":12,"CommentCount":3,"EditTime":1710000000}]}}`))
	}))
	defer server.Close()
	fixture := newZhihuSearchFixture(t, server.URL)

	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"query": "AI Agent", "count": 3}, UserConfig: map[string]any{"zhihuAccessSecret": "zhihu-secret"}, ToolBodyDirectory: fixture.toolDir})

	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if path != "/api/v1/content/zhihu_search" || query != "AI Agent" || count != "3" {
		t.Fatalf("request path=%q query=%q count=%q", path, query, count)
	}
	if authHeader != "Bearer zhihu-secret" || timestampHeader == "" {
		t.Fatalf("auth=%q timestamp=%q", authHeader, timestampHeader)
	}
	if !strings.Contains(result.Content, "AI Agent 应用实践") || !strings.Contains(result.Content, "https://www.zhihu.com/question/1") {
		t.Fatalf("content = %s", result.Content)
	}
	if result.Metadata["searchType"] != searchTypeZhihu || result.Metadata["resultsCount"] != 1 {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteSearchesGlobalAndCapsCount(t *testing.T) {
	var path string
	var count string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		count = r.URL.Query().Get("Count")
		_, _ = w.Write([]byte(`{"Code":0,"Message":"success","Data":{"Items":[{"Title":"Global result","Url":"https://www.zhihu.com/pin/1","AuthorName":"作者","ContentText":"全局摘要","EditTime":"2026-01-01"}]}}`))
	}))
	defer server.Close()
	fixture := newZhihuSearchFixture(t, server.URL)

	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"query": "rave", "search_type": "global", "count": 25}, UserConfig: map[string]any{"zhihuAccessSecret": "zhihu-secret"}, ToolBodyDirectory: fixture.toolDir})

	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if path != "/api/v1/content/global_search" || count != "20" {
		t.Fatalf("request path=%q count=%q", path, count)
	}
	if result.Metadata["searchType"] != searchTypeGlobal || strings.Contains(result.Content, "Votes/Comments") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteFailsWhenSecretMissing(t *testing.T) {
	fixture := newZhihuSearchFixture(t, "http://127.0.0.1")

	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"query": "AI Agent"}, ToolBodyDirectory: fixture.toolDir})

	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "requires API key") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteRejectsInvalidSearchType(t *testing.T) {
	fixture := newZhihuSearchFixture(t, "http://127.0.0.1")

	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"query": "AI Agent", "searchType": "bad"}, UserConfig: map[string]any{"zhihuAccessSecret": "zhihu-secret"}, ToolBodyDirectory: fixture.toolDir})

	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "zhihu_search or global_search") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteFailsWhenAPIReportsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Code":401,"Message":"invalid secret","Data":{"Items":[]}}`))
	}))
	defer server.Close()
	fixture := newZhihuSearchFixture(t, server.URL)

	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"query": "AI Agent"}, UserConfig: map[string]any{"zhihuAccessSecret": "zhihu-secret"}, ToolBodyDirectory: fixture.toolDir})

	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "invalid secret") || result.Metadata["code"] != 401 {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteIgnoresEndpointArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Code":0,"Message":"success","Data":{"Items":[]}}`))
	}))
	defer server.Close()
	fixture := newZhihuSearchFixture(t, server.URL)

	result := Execute(context.Background(), types.ToolExecutionInput{Arguments: map[string]any{"query": "AI Agent", "baseURL": "http://127.0.0.1:1"}, UserConfig: map[string]any{"zhihuAccessSecret": "zhihu-secret"}, ToolBodyDirectory: fixture.toolDir})

	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
}

type zhihuSearchFixture struct {
	toolDir string
}

func newZhihuSearchFixture(t *testing.T, baseURL string) zhihuSearchFixture {
	t.Helper()
	toolDir := t.TempDir()
	config := Config{
		BaseURL:          baseURL,
		APIKeyEnv:        "ZHIHU_ACCESS_SECRET",
		APIKeyUserConfig: "zhihuAccessSecret",
		Endpoints: map[string]string{
			searchTypeZhihu:  "/api/v1/content/zhihu_search",
			searchTypeGlobal: "/api/v1/content/global_search",
		},
		Limits: LimitsConfig{DefaultTimeoutMs: 30000, MaxTimeoutMs: 120000, DefaultCount: 10, ZhihuSearchMaxCount: 10, GlobalSearchMaxCount: 20, MaxOutputChars: 20000},
	}
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal(config) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "config.json"), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	return zhihuSearchFixture{toolDir: toolDir}
}
