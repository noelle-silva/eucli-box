package hypercortexreader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"eucli-box/pkg/types"
)

type recordedCall struct {
	Method string
	Params map[string]any
}

// fixture 是测试用的假 HyperCortex 外部访问服务与工具配置目录。
type fixture struct {
	t          *testing.T
	dir        string
	server     *httptest.Server
	mu         sync.Mutex
	calls      []recordedCall
	authHeads  []string
	responses  map[string][]any
	failures   map[string]string
	userConfig map[string]any
}

func newFixture(t *testing.T, responses map[string][]any) *fixture {
	t.Helper()
	f := &fixture{t: t, responses: responses, failures: map[string]string{}}
	f.dir = t.TempDir()
	writeFixtureFile(t, filepath.Join(f.dir, "config.json"), `{"limits":{"maxOutputChars":50000}}`)
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
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	f.mu.Lock()
	message, failed := f.failures[frame.Method]
	queue := f.responses[frame.Method]
	hasResult := len(queue) > 0
	var result any
	if hasResult {
		result = queue[0]
		f.responses[frame.Method] = queue[1:]
	}
	f.mu.Unlock()
	if failed {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": message}})
		return
	}
	if !hasResult {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected method " + frame.Method}})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

// setRepos 在工具用户配置里安装仓库配置（与设置页保存的用户配置同形态）。
func (f *fixture) setRepos(payload string) {
	f.t.Helper()
	config := map[string]any{}
	if err := json.Unmarshal([]byte(payload), &config); err != nil {
		f.t.Fatalf("decode repos payload: %v", err)
	}
	f.userConfig = config
}

func (f *fixture) input(arguments map[string]any) types.ToolExecutionInput {
	return types.ToolExecutionInput{
		Arguments:         arguments,
		UserConfig:        cloneUserConfig(f.userConfig),
		ToolBodyDirectory: f.dir,
	}
}

func cloneUserConfig(source map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range source {
		out[key] = value
	}
	return out
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

func twoReposJSON(endpoint string) string {
	return fmt.Sprintf(`{"endpoint":%q,"defaultRepo":"notes","repos":[{"id":"notes","description":"主力知识库","key":"key-notes"},{"id":"work","description":"工作库","key":"key-work"}]}`, endpoint)
}

func writeFixtureFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func execute(t *testing.T, input types.ToolExecutionInput) types.ToolExecutionOutput {
	t.Helper()
	return Execute(context.Background(), input)
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

func TestExecuteListReposStaysLocalAndHidesKeys(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "list_repos"}))

	requireSuccess(t, result)
	for _, fragment := range []string{"notes（默认）", "主力知识库", "work", "工作库"} {
		if !strings.Contains(result.Content, fragment) {
			t.Fatalf("content %q missing %q", result.Content, fragment)
		}
	}
	for _, secret := range []string{"key-notes", "key-work"} {
		if strings.Contains(result.Content, secret) {
			t.Fatalf("content %q must not expose key", result.Content)
		}
	}
	if calls := f.callList(); len(calls) != 0 {
		t.Fatalf("list_repos must stay local, calls = %#v", calls)
	}
	if result.Metadata["defaultRepo"] != "notes" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteSearchNotesSendsParametersWithoutScope(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.search.query": {map[string]any{
			"kinds": []any{map[string]any{"kind": "markdown", "label": "文本"}},
			"items": []any{map[string]any{
				"noteId":      "note-1",
				"title":       "标题甲",
				"description": "简介甲",
				"dir":         "Notes/2026-09/note-1",
				"createdAtMs": 1000,
				"updatedAtMs": 2000,
				"noteFields":  []any{"title"},
				"faceHits":    []any{map[string]any{"faceId": "text", "kind": "markdown", "title": "文本", "snippet": "命中片段"}},
			}},
		}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action": "search_notes",
		"query":  "标题甲",
		"fields": []any{"title"},
		"limit":  1,
		"offset": 0,
	}))

	requireSuccess(t, result)
	for _, fragment := range []string{"note-1", "标题甲", "Notes/2026-09/note-1", "updatedAtMs=2000", "命中片段", "faceKinds=markdown", "nextOffset=1"} {
		if !strings.Contains(result.Content, fragment) {
			t.Fatalf("content %q missing %q", result.Content, fragment)
		}
	}
	calls := f.callList()
	if len(calls) != 1 || calls[0].Method != "hypercortex.search.query" {
		t.Fatalf("calls = %#v", calls)
	}
	params := calls[0].Params
	if params["query"] != "标题甲" || params["limit"] != float64(1) {
		t.Fatalf("params = %#v", params)
	}
	if _, ok := params["scope"]; ok {
		t.Fatalf("params must not carry scope: %#v", params)
	}
	if fields, ok := params["fields"].([]any); !ok || len(fields) != 1 || fields[0] != "title" {
		t.Fatalf("fields = %#v", params["fields"])
	}
	if auth := f.authList(); len(auth) != 1 || auth[0] != "Bearer key-notes" {
		t.Fatalf("auth = %#v", auth)
	}
	if result.Metadata["count"] != 1 {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteSearchNotesWithoutKeywordOmitsQuery(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.search.query": {map[string]any{"kinds": []any{}, "items": []any{}}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "search_notes"}))

	requireSuccess(t, result)
	if !strings.Contains(result.Content, "按更新时间倒序") {
		t.Fatalf("content = %q", result.Content)
	}
	calls := f.callList()
	if len(calls) != 1 {
		t.Fatalf("calls = %#v", calls)
	}
	if _, ok := calls[0].Params["query"]; ok {
		t.Fatalf("empty keyword must omit query: %#v", calls[0].Params)
	}
}

func TestExecuteReadNoteReadsManifestAndAllFaces(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.loadManifest": {map[string]any{
			"schemaVersion": 2,
			"id":            "note-1",
			"title":         "标题甲",
			"description":   "简介甲",
			"tags":          []any{"标签一", "标签二"},
			"createdAtMs":   1000,
			"updatedAtMs":   2000,
			"faceOrder":     []any{"text", "html"},
			"faces": map[string]any{
				"text": map[string]any{"id": "text", "kind": "markdown", "title": "文本", "file": "text.md"},
				"html": map[string]any{"id": "html", "kind": "html", "title": "网页", "file": "face.html"},
			},
		}},
		"hypercortex.notes.loadFace": {
			map[string]any{"id": "text", "noteId": "note-1", "face": map[string]any{"id": "text", "kind": "markdown", "title": "文本"}, "content": "文本正文", "exists": true, "updatedAtMs": 2000},
			map[string]any{"id": "html", "noteId": "note-1", "face": map[string]any{"id": "html", "kind": "html", "title": "网页"}, "content": "<p>网页正文</p>", "exists": true, "updatedAtMs": 2000},
		},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "read_note", "dir": "Notes/2026-09/note-1"}))

	requireSuccess(t, result)
	for _, fragment := range []string{"标题甲", "简介甲", "标签一、标签二", "版本（updatedAtMs）：2000", "文本正文", "<p>网页正文</p>", "面 1/2", "面 2/2"} {
		if !strings.Contains(result.Content, fragment) {
			t.Fatalf("content %q missing %q", result.Content, fragment)
		}
	}
	calls := f.callList()
	if len(calls) != 3 {
		t.Fatalf("calls = %#v", calls)
	}
	if calls[0].Method != "hypercortex.notes.loadManifest" || calls[0].Params["packageDir"] != "Notes/2026-09/note-1" {
		t.Fatalf("manifest call = %#v", calls[0])
	}
	if calls[1].Params["faceId"] != "text" || calls[2].Params["faceId"] != "html" {
		t.Fatalf("face calls = %#v", calls)
	}
	if result.Metadata["faceCount"] != 2 {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteNoteRelationsOmitsOptionalDefaults(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.refs.queryRelations": {map[string]any{
			"nodes": []any{map[string]any{"noteId": "note-1", "distance": 0}},
			"edges": []any{},
		}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "note_relations", "noteId": "note-1"}))

	requireSuccess(t, result)
	if !strings.Contains(result.Content, "半径：1（缺省）") || !strings.Contains(result.Content, "both（缺省）") {
		t.Fatalf("content = %q", result.Content)
	}
	calls := f.callList()
	if len(calls) != 1 || len(calls[0].Params) != 1 || calls[0].Params["noteId"] != "note-1" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestExecuteRepoSelectionUsesEntryKey(t *testing.T) {
	f := newFixture(t, map[string][]any{"hypercortex.assets.list": {[]any{}}})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "list_assets", "repo": "work"}))

	requireSuccess(t, result)
	if auth := f.authList(); len(auth) != 1 || auth[0] != "Bearer key-work" {
		t.Fatalf("auth = %#v", auth)
	}
	if result.Metadata["repo"] != "work" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteUnknownRepoFails(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "list_assets", "repo": "missing"}))

	requireFailure(t, result, "未注册的仓库")
	if calls := f.callList(); len(calls) != 0 {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestExecuteMissingRepoConfigFails(t *testing.T) {
	f := newFixture(t, nil)

	result := execute(t, f.input(map[string]any{"action": "list_assets"}))

	requireFailure(t, result, "缺少访问地址")
}

func TestExecuteBackendErrorFails(t *testing.T) {
	f := newFixture(t, map[string][]any{"hypercortex.assets.list": {}})
	f.failures["hypercortex.assets.list"] = "该访问密钥只能访问绑定仓库，不能访问其他仓库"
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "list_assets"}))

	requireFailure(t, result, "该访问密钥只能访问绑定仓库")
}

func TestExecuteUnsupportedActionFails(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "delete_note"}))

	requireFailure(t, result, "unsupported action")
}

func TestExecuteInvalidArgumentTypeFails(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "search_notes", "limit": "abc"}))

	requireFailure(t, result, "must be an integer")
}

func TestExecuteConnectionFailureFails(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))
	f.server.Close()

	result := execute(t, f.input(map[string]any{"action": "list_assets"}))

	requireFailure(t, result, "无法连接 HyperCortex 外部访问服务")
}

func TestExecuteTruncatesOutputKeepingEnvelope(t *testing.T) {
	longDescription := strings.Repeat("很长的简介内容", 80)
	f := newFixture(t, map[string][]any{
		"hypercortex.search.query": {map[string]any{
			"kinds": []any{},
			"items": []any{map[string]any{
				"noteId":      "note-1",
				"title":       "标题甲",
				"description": longDescription,
				"dir":         "Notes/2026-09/note-1",
				"updatedAtMs": 2000,
			}},
		}},
	})
	f.setRepos(twoReposJSON(f.server.URL))
	input := f.input(map[string]any{"action": "search_notes", "query": "标题甲"})
	input.UserConfig["maxOutputChars"] = 160

	result := execute(t, input)

	requireSuccess(t, result)
	if result.Metadata["truncated"] != true {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
	if !strings.Contains(result.Content, "[hypercortex_reader]") || !strings.Contains(result.Content, "truncated=true") {
		t.Fatalf("content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "action=search_notes") {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestExecuteArgumentLowersOutputLimit(t *testing.T) {
	longDescription := strings.Repeat("很长的简介内容", 80)
	f := newFixture(t, map[string][]any{
		"hypercortex.search.query": {map[string]any{
			"kinds": []any{},
			"items": []any{map[string]any{
				"noteId":      "note-1",
				"title":       "标题甲",
				"description": longDescription,
				"dir":         "Notes/2026-09/note-1",
				"updatedAtMs": 2000,
			}},
		}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "search_notes", "query": "标题甲", "maxOutputChars": 120}))

	requireSuccess(t, result)
	if result.Metadata["truncated"] != true {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
	if !strings.Contains(result.Content, "truncated=true") || !strings.Contains(result.Content, "action=search_notes") {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestExecuteRejectsArgumentAboveOutputLimit(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "search_notes", "maxOutputChars": 60000}))

	requireFailure(t, result, "between 1 and 50000")
	if calls := f.callList(); len(calls) != 0 {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestExecuteNoteRelationsRejectsNonPositiveRadius(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))

	for _, radius := range []any{0, -2} {
		result := execute(t, f.input(map[string]any{"action": "note_relations", "noteId": "note-1", "radius": radius}))
		requireFailure(t, result, "must be greater than zero")
	}
	if calls := f.callList(); len(calls) != 0 {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestExecuteListFavoritesRendersTree(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.favorites.tryLoad": {map[string]any{
			"version":        1,
			"rootFolderId":   "root",
			"updatedAtMs":    3000,
			"folders":        map[string]any{"root": map[string]any{"id": "root", "title": "根目录"}, "child": map[string]any{"id": "child", "title": "子夹", "description": "子夹说明"}},
			"refsByFolderId": map[string]any{"root": []any{map[string]any{"id": "r1", "folderId": "root", "kind": "folder", "targetId": "child"}, map[string]any{"id": "r2", "folderId": "root", "kind": "note", "targetId": "note-1"}}, "child": []any{map[string]any{"id": "r3", "folderId": "child", "kind": "asset", "targetId": "a.png"}}},
		}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "list_favorites"}))

	requireSuccess(t, result)
	for _, fragment := range []string{"版本（updatedAtMs）：3000", "### 根目录（root）", "收藏夹：子夹（child）", "笔记：note-1", "附件：a.png"} {
		if !strings.Contains(result.Content, fragment) {
			t.Fatalf("content %q missing %q", result.Content, fragment)
		}
	}
	if result.Metadata["folderCount"] != 2 || result.Metadata["itemCount"] != 2 {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteListFavoritesHandlesUninitialized(t *testing.T) {
	f := newFixture(t, map[string][]any{"hypercortex.favorites.tryLoad": {nil}})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "list_favorites"}))

	requireSuccess(t, result)
	if !strings.Contains(result.Content, "尚未初始化收藏夹") {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestExecuteSearchAssetsSendsFilters(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.search.queryAssets": {[]any{map[string]any{
			"relPath":     "Assets/images/2026-09/a.png",
			"name":        "a.png",
			"assetId":     "a",
			"ext":         "png",
			"kind":        "image",
			"mime":        "image/png",
			"displayName": "参考图",
			"tags":        []any{"设计"},
			"remark":      "备注",
			"size":        2048,
			"updatedAtMs": 4000,
		}}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action": "search_assets",
		"query":  "参考",
		"kind":   "image",
		"limit":  5,
	}))

	requireSuccess(t, result)
	for _, fragment := range []string{"参考图", "a.png", "2.0 KB", "Assets/images/2026-09/a.png", "标签：设计"} {
		if !strings.Contains(result.Content, fragment) {
			t.Fatalf("content %q missing %q", result.Content, fragment)
		}
	}
	calls := f.callList()
	if len(calls) != 1 || calls[0].Method != "hypercortex.search.queryAssets" {
		t.Fatalf("calls = %#v", calls)
	}
	if calls[0].Params["kind"] != "image" || calls[0].Params["query"] != "参考" {
		t.Fatalf("params = %#v", calls[0].Params)
	}
}

func TestExecuteListAssetsAndTrash(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.assets.list": {[]any{map[string]any{"name": "b.pdf", "assetId": "b", "ext": "pdf", "kind": "document", "size": 100, "updatedAtMs": 5000}}},
		"hypercortex.trash.list": {[]any{
			map[string]any{"kind": "note", "id": "note-9", "title": "已删笔记", "deletedAtMs": 6000, "originalDir": "Notes/2026-09/note-9"},
			map[string]any{"kind": "asset", "id": "c.png", "title": "已删附件", "deletedAtMs": 7000},
		}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	assets := execute(t, f.input(map[string]any{"action": "list_assets"}))
	requireSuccess(t, assets)
	if !strings.Contains(assets.Content, "b.pdf") || !strings.Contains(assets.Content, "document") {
		t.Fatalf("content = %q", assets.Content)
	}

	trash := execute(t, f.input(map[string]any{"action": "list_trash"}))
	requireSuccess(t, trash)
	for _, fragment := range []string{"[笔记] 已删笔记", "[附件] 已删附件", "原位置：Notes/2026-09/note-9"} {
		if !strings.Contains(trash.Content, fragment) {
			t.Fatalf("content %q missing %q", trash.Content, fragment)
		}
	}
	if trash.Metadata["count"] != 2 {
		t.Fatalf("metadata = %#v", trash.Metadata)
	}
}
