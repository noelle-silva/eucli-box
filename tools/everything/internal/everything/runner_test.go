package everything

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"eucli-box/tools/everything/internal/types"
)

// fixture 是测试用的假 Everything 应用开放接口与工具配置目录。
type fixture struct {
	t          *testing.T
	dir        string
	server     *httptest.Server
	mu         sync.Mutex
	calls      []recordedCall
	authHeads  []string
	responses  map[string][]any
	failures   map[string]string
	failCodes  map[string]string
	userConfig map[string]any
}

type recordedCall struct {
	Method string
	Params map[string]any
}

func newFixture(t *testing.T, responses map[string][]any) *fixture {
	t.Helper()
	f := &fixture{t: t, responses: responses, failures: map[string]string{}, failCodes: map[string]string{}}
	f.dir = t.TempDir()
	writeConfigFile(t, f.dir, `{"limits":{"defaultRequestTimeoutMs":30000,"maxOutputChars":30000}}`)
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fixture) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/rpc" {
		http.NotFound(w, r)
		return
	}
	var frame struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&frame); err != nil {
		f.t.Errorf("decode rpc frame: %v", err)
		http.Error(w, "bad frame", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.calls = append(f.calls, recordedCall{Method: frame.Method, Params: frame.Params})
	f.authHeads = append(f.authHeads, r.Header.Get("Authorization"))
	message, failed := f.failures[frame.Method]
	code := f.failCodes[frame.Method]
	queue := f.responses[frame.Method]
	hasResult := len(queue) > 0
	var result any
	if hasResult {
		result = queue[0]
		f.responses[frame.Method] = queue[1:]
	}
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if failed {
		payload := map[string]any{"message": message}
		if strings.TrimSpace(code) != "" {
			payload["code"] = code
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": payload})
		return
	}
	if !hasResult {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected method " + frame.Method}})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

func (f *fixture) setConnection(endpoint string, key string) {
	f.userConfig = map[string]any{"endpoint": endpoint, "key": key}
}

func (f *fixture) input(arguments map[string]any) types.ToolExecutionInput {
	userConfig := map[string]any{}
	for key, value := range f.userConfig {
		userConfig[key] = value
	}
	return types.ToolExecutionInput{Arguments: arguments, UserConfig: userConfig, ToolBodyDirectory: f.dir}
}

func (f *fixture) callList() []recordedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedCall, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fixture) authList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.authHeads))
	copy(out, f.authHeads)
	return out
}

func sampleSearchResult() map[string]any {
	return map[string]any{
		"query":     "notes",
		"limit":     80,
		"scopePath": "",
		"results": []any{map[string]any{
			"name":       "notes.md",
			"path":       `C:\Temp`,
			"fullPath":   `C:\Temp\notes.md`,
			"kind":       "file",
			"size":       "12",
			"modifiedAt": "2026/06/01 10:00",
		}},
	}
}

func requireSuccess(t *testing.T, result types.ToolExecutionOutput) {
	t.Helper()
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, content = %s", result.Status, result.Content)
	}
}

func requireFailure(t *testing.T, result types.ToolExecutionOutput, substring string) {
	t.Helper()
	if result.Status != types.ToolStatusFailed {
		t.Fatalf("status = %s, want failed (content = %s)", result.Status, result.Content)
	}
	if !strings.Contains(result.Content, substring) {
		t.Fatalf("failure %q does not contain %q", result.Content, substring)
	}
}

func TestExecuteSearchSendsRequestAndFormatsResult(t *testing.T) {
	f := newFixture(t, map[string][]any{"everything.search": {sampleSearchResult()}})
	f.setConnection(f.server.URL, "key-1")

	result := Execute(context.Background(), f.input(map[string]any{"query": "notes", "maxResults": 20, "description": "查找笔记"}))

	requireSuccess(t, result)
	for _, fragment := range []string{"## Everything Search Results", "notes.md", `C:\Temp\notes.md`, "kind: file"} {
		if !strings.Contains(result.Content, fragment) {
			t.Fatalf("content %q missing %q", result.Content, fragment)
		}
	}
	calls := f.callList()
	if len(calls) != 1 || calls[0].Method != "everything.search" {
		t.Fatalf("calls = %#v", calls)
	}
	if calls[0].Params["query"] != "notes" || calls[0].Params["limit"] != float64(20) {
		t.Fatalf("params = %#v", calls[0].Params)
	}
	if _, ok := calls[0].Params["scopePath"]; ok {
		t.Fatalf("params must omit empty scopePath: %#v", calls[0].Params)
	}
	if auth := f.authList(); len(auth) != 1 || auth[0] != "Bearer key-1" {
		t.Fatalf("auth = %#v", auth)
	}
	if result.Metadata["query"] != "notes" || result.Metadata["resultsCount"] != 1 || result.Metadata["description"] != "查找笔记" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
	if _, ok := result.Metadata["durationMs"].(int64); !ok {
		t.Fatalf("durationMs = %#v", result.Metadata["durationMs"])
	}
}

func TestExecuteSearchSendsScopePathWhenProvided(t *testing.T) {
	scope := t.TempDir()
	f := newFixture(t, map[string][]any{"everything.search": {sampleSearchResult()}})
	f.setConnection(f.server.URL, "key-1")

	result := Execute(context.Background(), f.input(map[string]any{"query": "notes", "scopePath": scope}))

	requireSuccess(t, result)
	params := f.callList()[0].Params
	if params["scopePath"] != scope {
		t.Fatalf("scopePath = %#v", params["scopePath"])
	}
	if result.Metadata["scopePath"] != scope {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteSearchEmptyResult(t *testing.T) {
	f := newFixture(t, map[string][]any{"everything.search": {map[string]any{"query": "ghost", "results": []any{}}}})
	f.setConnection(f.server.URL, "key-1")

	result := Execute(context.Background(), f.input(map[string]any{"query": "ghost"}))

	requireSuccess(t, result)
	if !strings.Contains(result.Content, "No local files matched the query.") {
		t.Fatalf("content = %q", result.Content)
	}
	if result.Metadata["resultsCount"] != 0 {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteSearchTruncatesOutput(t *testing.T) {
	long := strings.Repeat("很长的路径内容", 200)
	results := []any{}
	for index := 0; index < 20; index++ {
		results = append(results, map[string]any{"name": "f", "path": long, "fullPath": long, "kind": "file", "size": "1", "modifiedAt": "x"})
	}
	f := newFixture(t, map[string][]any{"everything.search": {map[string]any{"query": "notes", "results": results}}})
	f.setConnection(f.server.URL, "key-1")
	input := f.input(map[string]any{"query": "notes", "maxOutputChars": 200})

	result := Execute(context.Background(), input)

	requireSuccess(t, result)
	if result.Metadata["truncated"] != true {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
	if runes := len([]rune(result.Content)); runes > 200 {
		t.Fatalf("content runes = %d, want <= 200", runes)
	}
}

func TestExecuteSearchBackendErrorFailsWithCode(t *testing.T) {
	f := newFixture(t, map[string][]any{"everything.search": {}})
	f.setConnection(f.server.URL, "key-1")
	f.mu.Lock()
	f.failures["everything.search"] = "索引尚未就绪"
	f.failCodes["everything.search"] = "INDEX_NOT_READY"
	f.mu.Unlock()

	result := Execute(context.Background(), f.input(map[string]any{"query": "notes"}))

	requireFailure(t, result, "索引尚未就绪")
	if result.Metadata["code"] != "INDEX_NOT_READY" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteSearchConnectionFailureGuidesUser(t *testing.T) {
	f := newFixture(t, nil)
	f.setConnection(f.server.URL, "key-1")
	f.server.Close()

	result := Execute(context.Background(), f.input(map[string]any{"query": "notes"}))

	requireFailure(t, result, "无法连接 Everything 应用")
	if !strings.Contains(result.Content, "请确认 Everything 应用正在运行") {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestExecuteSearchMissingConnectionFails(t *testing.T) {
	f := newFixture(t, nil)

	result := Execute(context.Background(), f.input(map[string]any{"query": "notes"}))

	requireFailure(t, result, "缺少访问地址")
}

func TestExecuteSearchCancelledContext(t *testing.T) {
	f := newFixture(t, nil)
	f.setConnection(f.server.URL, "key-1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := Execute(ctx, f.input(map[string]any{"query": "notes"}))

	requireFailure(t, result, "cancelled")
}

func TestFormatContentOmitsScopeWhenEmpty(t *testing.T) {
	content, truncated := formatContent(searchResultPayload{Query: "q", Results: []searchResult{}}, searchRequest{MaxOutputChars: 2000})
	if truncated {
		t.Fatal("must not truncate small content")
	}
	if strings.Contains(content, "Scope:") {
		t.Fatalf("content = %q", content)
	}
}

func TestFormatContentIncludesScopeWhenPresent(t *testing.T) {
	content, _ := formatContent(searchResultPayload{Query: "q", ScopePath: `D:\Projects`, Results: []searchResult{}}, searchRequest{MaxOutputChars: 2000})
	if !strings.Contains(content, "Scope: `D:\\Projects`") {
		t.Fatalf("content = %q", content)
	}
}

func TestWriteConfigFixtureFilesExist(t *testing.T) {
	f := newFixture(t, nil)
	if _, err := os.Stat(filepath.Join(f.dir, "config.json")); err != nil {
		t.Fatalf("config fixture missing: %v", err)
	}
}
