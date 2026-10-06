package hypercortexwriter

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

	"eucli-box/tools/hypercortex_writer/internal/types"
)

type recordedCall struct {
	Method string
	Params map[string]any
}

// fixture 是测试用的假 HyperCortex 外部访问服务与工具配置目录。
type fixture struct {
	t            *testing.T
	dir          string
	server       *httptest.Server
	mu           sync.Mutex
	calls        []recordedCall
	authHeads    []string
	responses    map[string][]any
	failures     map[string]string
	failureCodes map[string]string
	userConfig   map[string]any
}

func newFixture(t *testing.T, responses map[string][]any) *fixture {
	t.Helper()
	f := &fixture{t: t, responses: responses, failures: map[string]string{}, failureCodes: map[string]string{}}
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
	code := f.failureCodes[frame.Method]
	queue := f.responses[frame.Method]
	hasResult := len(queue) > 0
	var result any
	if hasResult {
		result = queue[0]
		f.responses[frame.Method] = queue[1:]
	}
	f.mu.Unlock()
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

// noteSavePayload 构造后端笔记写入接口的响应形态。
func noteSavePayload(noteID string, dir string, version float64) map[string]any {
	return map[string]any{
		"meta": map[string]any{
			"id":          noteID,
			"title":       "标题",
			"description": "",
			"dir":         dir,
			"createdAtMs": 1000,
			"updatedAtMs": version,
		},
		"manifest": map[string]any{
			"schemaVersion": 2,
			"id":            noteID,
			"title":         "标题",
			"description":   "",
			"tags":          []any{},
			"createdAtMs":   1000,
			"updatedAtMs":   version,
			"faceOrder":     []any{"text"},
			"faces": map[string]any{
				"text": map[string]any{"id": "text", "kind": "markdown", "title": "文本", "file": "text.md"},
			},
		},
		"version": version,
	}
}

func TestExecuteListFaceKindsRendersKinds(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.listFacePlugins": {[]any{
			map[string]any{"kind": "markdown", "label": "文本", "defaultFaceId": "text", "defaultFileName": "text.md"},
			map[string]any{"kind": "html", "label": "HTML", "defaultFaceId": "html", "defaultFileName": "html.html", "settings": []any{
				map[string]any{"key": "displayMode", "kind": "enum", "label": "HTML 面显示方式", "default": "fixed-fit", "options": []any{
					map[string]any{"value": "natural", "label": "自然撑开"},
					map[string]any{"value": "fit-window", "label": "随窗口自适应"},
					map[string]any{"value": "fixed-fit", "label": "固定视口缩放"},
				}},
				map[string]any{"key": "fixedScale", "kind": "number", "label": "HTML 面缩放比例", "default": 0.95, "min": 0.25, "max": 2, "step": 0.01},
			}},
		}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "list_face_kinds"}))

	requireSuccess(t, result)
	// 面类型的设置声明必须随清单给出：键名、类型、默认值与可选值，调用方无需猜测。
	for _, fragment := range []string{
		"文本（markdown）", "HTML（html）", "默认面 id：text",
		"设置：无",
		"设置项 displayMode（enum）", "默认 fixed-fit", "可选 natural/fit-window/fixed-fit",
		"设置项 fixedScale（number）", "默认 0.95", "范围 0.25~2",
	} {
		if !strings.Contains(result.Content, fragment) {
			t.Fatalf("content %q missing %q", result.Content, fragment)
		}
	}
	calls := f.callList()
	if len(calls) != 1 || calls[0].Method != "hypercortex.notes.listFacePlugins" {
		t.Fatalf("calls = %#v", calls)
	}
	if auth := f.authList(); len(auth) != 1 || auth[0] != "Bearer key-notes" {
		t.Fatalf("auth = %#v", auth)
	}
}

func TestExecuteCreateNoteSendsInputAndRendersVersion(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.create": {noteSavePayload("note-1", "Notes/2026-09/note-1", 1700)},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action":          "create_note",
		"title":           "新笔记",
		"noteDescription": "简介",
		"tags":            []any{"甲", "乙"},
		"faceKinds":       []any{"markdown"},
	}))

	requireSuccess(t, result)
	calls := f.callList()
	if len(calls) != 1 {
		t.Fatalf("calls = %#v", calls)
	}
	noteInput, _ := calls[0].Params["input"].(map[string]any)
	if noteInput["title"] != "新笔记" || noteInput["description"] != "简介" {
		t.Fatalf("note input = %#v", noteInput)
	}
	tags, _ := noteInput["tags"].([]any)
	if len(tags) != 2 || tags[0] != "甲" {
		t.Fatalf("tags = %#v", noteInput["tags"])
	}
	if _, ok := calls[0].Params["scope"]; ok {
		t.Fatalf("params must not carry scope: %#v", calls[0].Params)
	}
	for _, fragment := range []string{"noteId：note-1", "dir：Notes/2026-09/note-1", "version=1700"} {
		if !strings.Contains(result.Content, fragment) {
			t.Fatalf("content %q missing %q", result.Content, fragment)
		}
	}
}

func TestExecuteWriteNoteSendsFacesAndExpectedVersion(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.tryReadManifest": {noteSavePayload("note-1", "Notes/2026-09/note-1", 1700)["manifest"]},
		"hypercortex.notes.saveFaces":       {noteSavePayload("note-1", "Notes/2026-09/note-1", 1800)},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action":          "write_note",
		"dir":             "Notes/2026-09/note-1",
		"noteId":          "note-1",
		"title":           "标题",
		"noteDescription": "简介",
		"expectedVersion": float64(1700),
		"faces": []any{
			map[string]any{"faceId": "text", "kind": "markdown", "content": "正文"},
		},
	}))

	requireSuccess(t, result)
	calls := f.callList()
	if len(calls) != 2 || calls[0].Method != "hypercortex.notes.tryReadManifest" || calls[1].Method != "hypercortex.notes.saveFaces" {
		t.Fatalf("calls = %#v", calls)
	}
	params := calls[1].Params
	if params["expectedVersion"] != float64(1700) {
		t.Fatalf("expectedVersion = %#v", params["expectedVersion"])
	}
	noteInput, _ := params["input"].(map[string]any)
	if noteInput["id"] != "note-1" || noteInput["packageDir"] != "Notes/2026-09/note-1" || noteInput["title"] != "标题" {
		t.Fatalf("note input = %#v", noteInput)
	}
	faces, _ := noteInput["faces"].([]any)
	if len(faces) != 1 {
		t.Fatalf("faces = %#v", noteInput["faces"])
	}
	face, _ := faces[0].(map[string]any)
	if face["kind"] != "markdown" || face["content"] != "正文" {
		t.Fatalf("face = %#v", face)
	}
	if !strings.Contains(result.Content, "version=1800") {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestExecuteWriteNoteRejectsUnknownFaceFields(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action": "write_note",
		"dir":    "d",
		"noteId": "n",
		"title":  "t",
		"faces": []any{
			map[string]any{"kind": "markdown", "content": "x", "oops": 1},
		},
	}))

	requireFailure(t, result, "unknown field")
	if calls := f.callList(); len(calls) != 0 {
		t.Fatalf("calls = %#v", calls)
	}
}

// write_note 只更新已有笔记：目录不存在时快速失败，绝不静默创建新笔记。
func TestExecuteWriteNoteRejectsMissingNote(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.tryReadManifest": {nil},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action": "write_note",
		"dir":    "Notes/2026-09/ghost",
		"noteId": "ghost",
		"title":  "标题",
	}))

	requireFailure(t, result, "笔记不存在")
	if !strings.Contains(result.Content, "create_note") {
		t.Fatalf("failure should point to create_note: %q", result.Content)
	}
	calls := f.callList()
	if len(calls) != 1 || calls[0].Method != "hypercortex.notes.tryReadManifest" {
		t.Fatalf("save must not be called: %#v", calls)
	}
}

// write_note 提交的 noteId 与目录归属不一致时快速失败。
func TestExecuteWriteNoteRejectsOwnerMismatch(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.tryReadManifest": {noteSavePayload("owner-a", "Notes/2026-09/owner-a", 1700)["manifest"]},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action": "write_note",
		"dir":    "Notes/2026-09/owner-a",
		"noteId": "owner-b",
		"title":  "标题",
	}))

	requireFailure(t, result, "归属不匹配")
}

func TestExecutePatchFaceKeepsRawText(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.patchFace": {noteSavePayload("note-1", "Notes/2026-09/note-1", 1900)},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action":          "patch_face",
		"dir":             "Notes/2026-09/note-1",
		"faceId":          "text",
		"oldString":       "  spaced old  ",
		"newString":       "  spaced new  ",
		"replaceAll":      true,
		"expectedVersion": float64(1800),
	}))

	requireSuccess(t, result)
	params := f.callList()[0].Params
	if params["oldString"] != "  spaced old  " || params["newString"] != "  spaced new  " {
		t.Fatalf("raw text not preserved: %#v", params)
	}
	if params["replaceAll"] != true || params["expectedVersion"] != float64(1800) {
		t.Fatalf("params = %#v", params)
	}
}

func TestExecutePatchFaceRequiresNewString(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action":    "patch_face",
		"dir":       "d",
		"faceId":    "text",
		"oldString": "old",
	}))

	requireFailure(t, result, "newString")
}

func TestExecuteSaveFaceOrderAndSettings(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.saveFaceOrder":    {noteSavePayload("note-1", "d", 2000)},
		"hypercortex.notes.saveFaceSettings": {noteSavePayload("note-1", "d", 2100)},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	order := execute(t, f.input(map[string]any{
		"action":          "save_face_order",
		"dir":             "d",
		"faceOrder":       []any{"html", "text"},
		"expectedVersion": float64(1900),
	}))
	requireSuccess(t, order)
	orderParams := f.callList()[0].Params
	faceOrder, _ := orderParams["faceOrder"].([]any)
	if len(faceOrder) != 2 || faceOrder[0] != "html" || orderParams["expectedVersion"] != float64(1900) {
		t.Fatalf("order params = %#v", orderParams)
	}

	settings := execute(t, f.input(map[string]any{
		"action":          "save_face_settings",
		"dir":             "d",
		"faceId":          "html",
		"settings":        map[string]any{"fixedScale": nil, "displayMode": "natural"},
		"expectedVersion": float64(2000),
	}))
	requireSuccess(t, settings)
	settingsParams := f.callList()[1].Params
	patch, _ := settingsParams["settings"].(map[string]any)
	if _, ok := patch["fixedScale"]; !ok || patch["displayMode"] != "natural" {
		t.Fatalf("settings = %#v", patch)
	}
}

func TestExecuteSaveFaceOrderRequiresNonEmptyList(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action":    "save_face_order",
		"dir":       "d",
		"faceOrder": []any{},
	}))

	requireFailure(t, result, "faceOrder")
}

func TestExecuteDeleteFaceModes(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.deleteFace": {noteSavePayload("note-1", "d", 2200)},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action": "delete_face",
		"dir":    "d",
		"faceId": "html",
		"mode":   "trash",
	}))
	requireSuccess(t, result)
	params := f.callList()[0].Params
	if params["mode"] != "trash" || params["faceId"] != "html" {
		t.Fatalf("params = %#v", params)
	}
	if _, ok := params["expectedVersion"]; ok {
		t.Fatalf("delete_face must not send expectedVersion: %#v", params)
	}

	bad := execute(t, f.input(map[string]any{
		"action": "delete_face",
		"dir":    "d",
		"faceId": "html",
		"mode":   "weird",
	}))
	requireFailure(t, bad, "mode")
}

func TestExecutePublishVersion(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.versions.publish": {map[string]any{
			"versionId":   "v_20260930_1",
			"commitName":  "初稿",
			"createdAtMs": 1700,
			"title":       "标题",
		}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action":     "publish_version",
		"dir":        "Notes/2026-09/note-1",
		"commitName": "初稿",
	}))

	requireSuccess(t, result)
	for _, fragment := range []string{"版本 id：v_20260930_1", "提交名：初稿"} {
		if !strings.Contains(result.Content, fragment) {
			t.Fatalf("content %q missing %q", result.Content, fragment)
		}
	}
}

func TestExecuteUpdateNoteMetadataPatchSemantics(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.updateMetadata": {map[string]any{
			"version": 2300,
			"changed": true,
			"meta": map[string]any{
				"id": "note-1", "title": "新标题", "description": "", "dir": "d", "createdAtMs": 1000, "updatedAtMs": 2300,
			},
		}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action":          "update_note_metadata",
		"dir":             "d",
		"title":           "新标题",
		"expectedVersion": float64(2200),
	}))

	requireSuccess(t, result)
	params := f.callList()[0].Params
	metadata, _ := params["metadata"].(map[string]any)
	if len(metadata) != 1 || metadata["title"] != "新标题" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if params["expectedVersion"] != float64(2200) {
		t.Fatalf("expectedVersion = %#v", params["expectedVersion"])
	}

	empty := execute(t, f.input(map[string]any{"action": "update_note_metadata", "dir": "d"}))
	requireFailure(t, empty, "at least one")
}

func TestExecuteUpdateNoteMetadataAllowsClearingDescription(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.updateMetadata": {map[string]any{"version": 2400, "changed": true}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action":          "update_note_metadata",
		"dir":             "d",
		"noteDescription": "",
	}))

	requireSuccess(t, result)
	metadata, _ := f.callList()[0].Params["metadata"].(map[string]any)
	value, ok := metadata["description"]
	if !ok || value != "" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestExecuteUploadAssetsWaitsAndRendersMarkers(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.assets.upload.sync": {[]any{
			map[string]any{"assetId": "abc123", "ext": "png", "kind": "image", "name": "图", "marker": "{{asset:abc123.png||320}}"},
		}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action": "upload_assets",
		"files": []any{
			map[string]any{"path": `C:\tmp\图.png`, "displayName": "图"},
		},
	}))

	requireSuccess(t, result)
	for _, fragment := range []string{"assetId：abc123", "引用标记：{{asset:abc123.png||320}}"} {
		if !strings.Contains(result.Content, fragment) {
			t.Fatalf("content %q missing %q", result.Content, fragment)
		}
	}
	files, _ := f.callList()[0].Params["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("files = %#v", files)
	}
	entry, _ := files[0].(map[string]any)
	if entry["path"] != `C:\tmp\图.png` || entry["displayName"] != "图" {
		t.Fatalf("file entry = %#v", entry)
	}
}

func TestExecuteUploadAssetsRejectsMissingPath(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action": "upload_assets",
		"files":  []any{map[string]any{"displayName": "无路径"}},
	}))

	requireFailure(t, result, "path")
}

func TestExecuteUpdateAssetMetadata(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.assets.updateMetadata": {map[string]any{
			"assetId": "abc", "ext": "png", "name": "abc.png", "displayName": "新名字", "tags": []any{"甲"}, "updatedAtMs": 1700,
		}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action":      "update_asset_metadata",
		"assetId":     "abc",
		"ext":         "png",
		"displayName": "新名字",
		"tags":        []any{"甲"},
	}))

	requireSuccess(t, result)
	params := f.callList()[0].Params
	metadata, _ := params["metadata"].(map[string]any)
	if metadata["displayName"] != "新名字" || params["ext"] != "png" {
		t.Fatalf("params = %#v", params)
	}
	// 全量提交语义：未提供的字段显式按空提交（与后端接口一致），不静默保留。
	if remark, ok := metadata["remark"]; !ok || remark != "" {
		t.Fatalf("remark must be submitted as empty string: %#v", metadata)
	}
	if tags, ok := metadata["tags"]; !ok || tags == nil {
		t.Fatalf("tags must be submitted (possibly empty): %#v", metadata)
	}
	if !strings.Contains(result.Content, "新名字") {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestExecuteCreateFavoriteFolder(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.favorites.createFolder": {map[string]any{
			"version": 3000, "folderId": "f_new", "parentId": "root",
		}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action":            "create_favorite_folder",
		"title":             "新夹",
		"folderDescription": "说明",
		"expectedVersion":   float64(2900),
	}))

	requireSuccess(t, result)
	params := f.callList()[0].Params
	if params["title"] != "新夹" || params["description"] != "说明" || params["expectedVersion"] != float64(2900) {
		t.Fatalf("params = %#v", params)
	}
	if _, ok := params["parentId"]; ok {
		t.Fatalf("empty parent must not be sent: %#v", params)
	}
	if !strings.Contains(result.Content, "folderId=f_new") {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestExecuteUpdateFavoriteFolder(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.favorites.updateFolder": {map[string]any{"version": 3100, "folderId": "f_1", "changed": false}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action":   "update_favorite_folder",
		"folderId": "f_1",
		"title":    "改名",
	}))

	requireSuccess(t, result)
	patch, _ := f.callList()[0].Params["patch"].(map[string]any)
	if len(patch) != 1 || patch["title"] != "改名" {
		t.Fatalf("patch = %#v", patch)
	}
	if !strings.Contains(result.Content, "changed=false") {
		t.Fatalf("content = %q", result.Content)
	}

	empty := execute(t, f.input(map[string]any{"action": "update_favorite_folder", "folderId": "f_1"}))
	requireFailure(t, empty, "at least one")
}

func TestExecuteAddRemoveMoveFavoriteItem(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.favorites.addItem":    {map[string]any{"version": 3200, "refId": "ref_1", "folderId": "f_1"}},
		"hypercortex.favorites.removeItem": {map[string]any{"version": 3300, "folderId": "f_1"}},
		"hypercortex.favorites.moveItem":   {map[string]any{"version": 3400, "refId": "ref_2", "fromFolderId": "f_1", "toFolderId": "f_2"}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	add := execute(t, f.input(map[string]any{
		"action":   "add_favorite_item",
		"folderId": "f_1",
		"kind":     "note",
		"targetId": "note-1",
	}))
	requireSuccess(t, add)
	addParams := f.callList()[0].Params
	if addParams["kind"] != "note" || addParams["targetId"] != "note-1" {
		t.Fatalf("add params = %#v", addParams)
	}

	remove := execute(t, f.input(map[string]any{
		"action":   "remove_favorite_item",
		"folderId": "f_1",
		"kind":     "asset",
		"targetId": "abc.png",
	}))
	requireSuccess(t, remove)

	move := execute(t, f.input(map[string]any{
		"action":       "move_favorite_item",
		"fromFolderId": "f_1",
		"toFolderId":   "f_2",
		"kind":         "folder",
		"targetId":     "f_3",
	}))
	requireSuccess(t, move)
	moveParams := f.callList()[2].Params
	if moveParams["fromFolderId"] != "f_1" || moveParams["toFolderId"] != "f_2" {
		t.Fatalf("move params = %#v", moveParams)
	}

	bad := execute(t, f.input(map[string]any{
		"action":   "add_favorite_item",
		"folderId": "f_1",
		"kind":     "weird",
		"targetId": "x",
	}))
	requireFailure(t, bad, "kind")
}

func TestExecuteUnknownActionFailsFast(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "nope"}))

	requireFailure(t, result, "unsupported action")
}

// 未知参数快速失败：不再静默接受（框架惯例的 description 属合法公共参数，不在此列）。
func TestExecuteRejectsUnknownArguments(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{"action": "update_note_metadata", "dir": "d", "summary": "未知参数"}))

	requireFailure(t, result, "unknown argument")
	if calls := f.callList(); len(calls) != 0 {
		t.Fatalf("calls = %#v", calls)
	}
}

// 框架惯例的「调用原因」description 被接受并记入结果元数据（与 shell_command 同款）。
func TestExecuteAcceptsDescriptionConvention(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.updateMetadata": {map[string]any{"version": 2300, "changed": true}},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action":      "update_note_metadata",
		"dir":         "d",
		"title":       "新标题",
		"description": "用户要求更新标题",
	}))

	requireSuccess(t, result)
	if result.Metadata["description"] != "用户要求更新标题" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

// save_face_settings 回显保存后的实际设置，写入是否生效当场可验证。
func TestExecuteSaveFaceSettingsEchoesSavedSettings(t *testing.T) {
	payload := noteSavePayload("note-1", "d", 2100)
	payload["manifest"].(map[string]any)["faces"].(map[string]any)["html"] = map[string]any{
		"id": "html", "kind": "html", "title": "HTML", "file": "html-view.html",
		"settings": map[string]any{"displayMode": "natural", "fixedScale": 0.8},
	}
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.saveFaceSettings": {payload},
	})
	f.setRepos(twoReposJSON(f.server.URL))

	result := execute(t, f.input(map[string]any{
		"action":   "save_face_settings",
		"dir":      "d",
		"faceId":   "html",
		"settings": map[string]any{"displayMode": "natural", "fixedScale": 0.8},
	}))

	requireSuccess(t, result)
	for _, fragment := range []string{"当前设置：displayMode=natural，fixedScale=0.8", "settings=displayMode=natural"} {
		if !strings.Contains(result.Content, fragment) {
			t.Fatalf("content %q missing %q", result.Content, fragment)
		}
	}
}

// 失败输出同样带信息条（动作、仓库与错误码）。
func TestExecuteFailureCarriesEnvelope(t *testing.T) {
	f := newFixture(t, map[string][]any{
		"hypercortex.notes.tryReadManifest": {noteSavePayload("note-1", "Notes/2026-09/note-1", 2)["manifest"]},
	})
	f.setRepos(twoReposJSON(f.server.URL))
	f.mu.Lock()
	f.failures["hypercortex.notes.saveFaces"] = "笔记版本不匹配：期望版本 1，当前版本 2"
	f.failureCodes["hypercortex.notes.saveFaces"] = "VERSION_CONFLICT"
	f.mu.Unlock()

	result := execute(t, f.input(map[string]any{
		"action": "write_note",
		"dir":    "Notes/2026-09/note-1",
		"noteId": "note-1",
		"title":  "标题",
	}))

	requireFailure(t, result, "版本不匹配")
	for _, fragment := range []string{"[hypercortex_writer]", "action=write_note", "repo=notes", "dir=Notes/2026-09/note-1", "code=VERSION_CONFLICT"} {
		if !strings.Contains(result.Content, fragment) {
			t.Fatalf("content %q missing %q", result.Content, fragment)
		}
	}
}

func TestExecuteSurfacesBackendErrors(t *testing.T) {
	f := newFixture(t, nil)
	f.setRepos(twoReposJSON(f.server.URL))
	f.mu.Lock()
	f.failures["hypercortex.notes.create"] = "笔记版本不匹配：期望版本 1，当前版本 2"
	f.mu.Unlock()

	result := execute(t, f.input(map[string]any{"action": "create_note", "title": "t"}))

	requireFailure(t, result, "版本不匹配")
}
