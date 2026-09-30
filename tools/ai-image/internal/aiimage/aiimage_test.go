package aiimage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

// pngBase64 是一张 1x1 PNG 的 base64，用于校验「尺寸过小」的拒绝路径。
const pngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// validPngBase64 是一张 64x64 PNG 的 base64，满足参考图最短边下限。
const validPngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAEAAAABACAIAAAAlC+aJAAAAZElEQVR4nOzPUQnAUBTFsPNxhU/6ZJQHCTXQ277bXu72NAM1AzUDNQM1AzUDNQM1AzUDNQM1AzUDNQM1AzUDNQM1AzUDNQM1AzUDNQM1AzUDNQM1AzUDNQM1AzUDNQO1PwAA//+zRQN5bYZVRgAAAABJRU5ErkJggg=="

func pngDataURL() string {
	return "data:image/png;base64," + pngBase64
}

func validPngDataURL() string {
	return "data:image/png;base64," + validPngBase64
}

// fakeSession 是会话能力服务的测试替身：按能力与访问方式应答。
type fakeSession struct {
	images      []sessionImage
	written     []types.SessionAttachmentWriteRequest
	listDenied  bool
	writeDenied bool
	readDataURL string
}

func (f *fakeSession) Request(_ context.Context, capability string, access string, payload any) (toolcontrol.CapabilityResult, error) {
	switch {
	case capability == types.ToolCapabilitySessionAttachments && access == types.ToolCapabilityAccessRead:
		if f.listDenied {
			return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusDenied, Error: toolcontrol.CapabilityDeniedMessage}, nil
		}
		request, _ := payload.(types.SessionAttachmentsReadRequest)
		if request.Operation == types.SessionAttachmentOperationRead {
			dataURL := f.readDataURL
			if dataURL == "" {
				dataURL = validPngDataURL()
			}
			return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.SessionAttachmentData{ID: request.AttachmentID, Name: "图片", Mime: "image/png", DataURL: dataURL}}, nil
		}
		attachments := make([]types.SessionAttachmentInfo, 0, len(f.images))
		for _, image := range f.images {
			attachments = append(attachments, types.SessionAttachmentInfo{ID: image.ID, Name: image.Name, Mime: image.Mime})
		}
		return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.SessionAttachmentsListResult{Attachments: attachments}}, nil
	case capability == types.ToolCapabilitySessionAttachments && access == types.ToolCapabilityAccessWrite:
		if f.writeDenied {
			return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusDenied, Error: toolcontrol.CapabilityDeniedMessage}, nil
		}
		request, _ := payload.(types.SessionAttachmentWriteRequest)
		f.written = append(f.written, request)
		return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.SessionAttachmentInfo{ID: "att-generated", Name: request.Name, Mime: "image/png"}}, nil
	default:
		return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusFailed, Error: "unsupported capability"}, nil
	}
}

func newInput(t *testing.T, arguments map[string]any) types.ToolExecutionInput {
	t.Helper()
	return types.ToolExecutionInput{
		ActionID:          "test-action",
		ToolName:          "ai-image",
		Arguments:         arguments,
		ToolDataDirectory: t.TempDir(),
	}
}

func decodeOutput(t *testing.T, output types.ToolExecutionOutput) map[string]any {
	t.Helper()
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("expected success, got %s: %s", output.Status, output.Error)
	}
	return output.Metadata
}

func writeProviders(t *testing.T, dataDir string, content string) {
	t.Helper()
	target := filepath.Join(dataDir, types.ToolConfigDirName, providersFileName)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir providers dir: %v", err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}
}

func TestConfigActionsRoundTrip(t *testing.T) {
	dataDir := t.TempDir()
	withDataDir := func(arguments map[string]any) types.ToolExecutionInput {
		input := newInput(t, arguments)
		input.ToolDataDirectory = dataDir
		return input
	}
	input := withDataDir(map[string]any{"action": actionConfigWrite, "file": "providers.json", "content": `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"https://example.com","apiKey":"sk-secret","protocol":"images","defaultModel":"m1"}]}`})
	output := Execute(context.Background(), input, nil)
	decodeOutput(t, output)

	read := Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigRead, "file": "providers.json"}), nil)
	metadata := decodeOutput(t, read)
	if metadata["masked"] != true {
		t.Fatalf("providers read must be masked: %#v", metadata)
	}
	if strings.Contains(read.Content, "sk-secret") {
		t.Fatalf("read content leaked apiKey: %s", read.Content)
	}
	if !strings.Contains(read.Content, maskedAPIKey) {
		t.Fatalf("read content missing mask: %s", read.Content)
	}

	edit := Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigEdit, "file": "providers.json", "field": "providers.0.name", "value": "示例"}), nil)
	decodeOutput(t, edit)
	read = Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigRead, "file": "providers.json"}), nil)
	if !strings.Contains(read.Content, "示例") {
		t.Fatalf("edit not applied: %s", read.Content)
	}
	if strings.Contains(read.Content, "sk-secret") {
		t.Fatalf("edit leaked apiKey: %s", read.Content)
	}

	list := Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigList}), nil)
	decodeOutput(t, list)
	if !strings.Contains(list.Content, "providers.json") {
		t.Fatalf("list missing providers.json: %s", list.Content)
	}

	remove := Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigEdit, "file": "providers.json", "field": "providers.0.name", "remove": true}), nil)
	decodeOutput(t, remove)

	deleteOutput := Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigDelete, "file": "providers.json"}), nil)
	decodeOutput(t, deleteOutput)
}

func TestConfigWriteRejectsInvalidProviderConfig(t *testing.T) {
	output := Execute(context.Background(), newInput(t, map[string]any{"action": actionConfigWrite, "file": "providers.json", "content": `{"providers":[{"id":"demo","baseUrl":"not-a-url","protocol":"images"}]}`}), nil)
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failure, got %s", output.Status)
	}
}

func TestConfigWritePreservesMaskedAPIKey(t *testing.T) {
	dataDir := t.TempDir()
	withDataDir := func(arguments map[string]any) types.ToolExecutionInput {
		input := newInput(t, arguments)
		input.ToolDataDirectory = dataDir
		return input
	}
	original := `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"https://example.com","apiKey":"sk-real","protocol":"images","defaultModel":"m1"}]}`
	decodeOutput(t, Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigWrite, "file": "providers.json", "content": original}), nil))

	masked := `{"defaultProvider":"demo","providers":[{"id":"demo","name":"改名","baseUrl":"https://example.com","apiKey":"` + maskedAPIKey + `","protocol":"images","defaultModel":"m1"}]}`
	decodeOutput(t, Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigWrite, "file": "providers.json", "content": masked}), nil))

	read := Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigRead, "file": "providers.json"}), nil)
	decodeOutput(t, read)
	if !strings.Contains(read.Content, "改名") {
		t.Fatalf("write not applied: %s", read.Content)
	}
	if strings.Contains(read.Content, "sk-real") {
		t.Fatalf("read leaked apiKey: %s", read.Content)
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, types.ToolConfigDirName, providersFileName))
	if err != nil {
		t.Fatalf("read providers file: %v", err)
	}
	if !strings.Contains(string(raw), "sk-real") {
		t.Fatalf("masked write must preserve original apiKey: %s", string(raw))
	}
}

func TestConfigWriteRejectsMaskedAPIKeyWithoutOriginal(t *testing.T) {
	masked := `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"https://example.com","apiKey":"` + maskedAPIKey + `","protocol":"images","defaultModel":"m1"}]}`
	output := Execute(context.Background(), newInput(t, map[string]any{"action": actionConfigWrite, "file": "providers.json", "content": masked}), nil)
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("masked write without original must fail, got %s", output.Status)
	}
	if !strings.Contains(output.Error, "明文") {
		t.Fatalf("error must ask for plaintext key: %s", output.Error)
	}
}

func TestConfigWriteRejectsMaskedAPIKeyForNewProvider(t *testing.T) {
	dataDir := t.TempDir()
	withDataDir := func(arguments map[string]any) types.ToolExecutionInput {
		input := newInput(t, arguments)
		input.ToolDataDirectory = dataDir
		return input
	}
	original := `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"https://example.com","apiKey":"sk-real","protocol":"images","defaultModel":"m1"}]}`
	decodeOutput(t, Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigWrite, "file": "providers.json", "content": original}), nil))

	added := `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"https://example.com","apiKey":"sk-real","protocol":"images","defaultModel":"m1"},{"id":"other","baseUrl":"https://example.com","apiKey":"` + maskedAPIKey + `","protocol":"images","defaultModel":"m1"}]}`
	output := Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigWrite, "file": "providers.json", "content": added}), nil)
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("masked key for new provider must fail, got %s", output.Status)
	}
	if !strings.Contains(output.Error, "other") {
		t.Fatalf("error must name the provider: %s", output.Error)
	}
}

func TestConfigWriteKeepsPlainContentVerbatim(t *testing.T) {
	dataDir := t.TempDir()
	input := newInput(t, map[string]any{"action": actionConfigWrite, "file": "providers.json", "content": `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"https://example.com","apiKey":"sk-plain","protocol":"images","defaultModel":"m1"}]}`})
	input.ToolDataDirectory = dataDir
	decodeOutput(t, Execute(context.Background(), input, nil))
	raw, err := os.ReadFile(filepath.Join(dataDir, types.ToolConfigDirName, providersFileName))
	if err != nil {
		t.Fatalf("read providers file: %v", err)
	}
	if !strings.Contains(string(raw), "sk-plain") {
		t.Fatalf("plain write must be stored verbatim: %s", string(raw))
	}
}

func TestConfigEditRejectsMaskedAPIKeyForNewProvider(t *testing.T) {
	dataDir := t.TempDir()
	withDataDir := func(arguments map[string]any) types.ToolExecutionInput {
		input := newInput(t, arguments)
		input.ToolDataDirectory = dataDir
		return input
	}
	original := `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"https://example.com","apiKey":"sk-real","protocol":"images","defaultModel":"m1"}]}`
	decodeOutput(t, Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigWrite, "file": "providers.json", "content": original}), nil))

	output := Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigEdit, "file": "providers.json", "field": "providers.0.id", "value": "renamed"}), nil)
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("editing id while masked key exists must fail, got %s", output.Status)
	}
}

func TestConfigActionsRequireFile(t *testing.T) {
	actions := []string{actionConfigRead, actionConfigWrite, actionConfigDelete, actionConfigEdit}
	for _, action := range actions {
		arguments := map[string]any{"action": action}
		switch action {
		case actionConfigWrite:
			arguments["content"] = `{}`
		case actionConfigEdit:
			arguments["field"] = "defaultProvider"
			arguments["value"] = "demo"
		}
		output := Execute(context.Background(), newInput(t, arguments), nil)
		if output.Status != types.ToolStatusFailed {
			t.Fatalf("action %s without file must fail, got %s", action, output.Status)
		}
		if !strings.Contains(output.Error, "缺少 file 参数") {
			t.Fatalf("action %s must explain the missing file argument: %s", action, output.Error)
		}
	}
}

func TestResolveModelValidatesModelsList(t *testing.T) {
	provider := providerEntry{ID: "demo", Models: []string{"m1", "m2"}, DefaultModel: "m1"}
	if _, err := resolveModel(provider, "m2"); err != nil {
		t.Fatalf("listed model must pass: %v", err)
	}
	if _, err := resolveModel(provider, "m9"); err == nil || !strings.Contains(err.Error(), "m1, m2") {
		t.Fatalf("unlisted model must fail with available models, got: %v", err)
	}
	open := providerEntry{ID: "demo", DefaultModel: "anything"}
	if _, err := resolveModel(open, ""); err != nil {
		t.Fatalf("provider without models list must pass: %v", err)
	}
}

func TestRequireSuccessCarriesStatusCode(t *testing.T) {
	plain := requireSuccess(types.HTTPResponse{StatusCode: 503, Body: []byte("gateway down")}, false)
	if plain == nil || !strings.Contains(plain.Error(), "HTTP 503") || !strings.Contains(plain.Error(), "gateway down") {
		t.Fatalf("status code and upstream message must be kept: %v", plain)
	}
	var classified *classifiedError
	if !errors.As(plain, &classified) || !classified.retryable {
		t.Fatalf("503 must be classified retryable: %v", plain)
	}
	empty := requireSuccess(types.HTTPResponse{StatusCode: 400}, false)
	if empty == nil || !strings.Contains(empty.Error(), "HTTP 400") {
		t.Fatalf("status code must be kept without upstream message: %v", empty)
	}
	if !errors.As(empty, &classified) || classified.retryable {
		t.Fatalf("400 must be classified non-retryable: %v", empty)
	}
	retried := requireSuccess(types.HTTPResponse{StatusCode: 502}, true)
	if retried == nil || !strings.Contains(retried.Error(), "已自动重试 1 次") {
		t.Fatalf("retried failure must be labelled: %v", retried)
	}
	if requireSuccess(types.HTTPResponse{StatusCode: 200}, false) != nil {
		t.Fatal("success status must pass")
	}
}

func TestGenerateIgnoresTinyTimeoutForLocalActions(t *testing.T) {
	input := newInput(t, map[string]any{"action": actionConfigList, "timeoutMs": 1})
	output := Execute(context.Background(), input, nil)
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("local action must ignore request timeout: %s", output.Error)
	}
}

func TestConfigWriteRejectsEscapingPath(t *testing.T) {
	for _, path := range []string{"../escape.json", "a/../../b.json", "C:/abs.json", "/abs.json"} {
		output := Execute(context.Background(), newInput(t, map[string]any{"action": actionConfigWrite, "file": path, "content": `{}`}), nil)
		if output.Status != types.ToolStatusFailed {
			t.Fatalf("path %q must fail, got %s", path, output.Status)
		}
	}
}

func TestAdapterValidationAndRendering(t *testing.T) {
	valid := `{"id":"my-adapter","request":{"method":"POST","path":"/v1/generate","headers":{"Authorization":"Bearer {{apiKey}}"},"json":{"model":"{{model}}","prompt":"{{prompt}}","images":"{{images}}"}},"response":{"imagePath":"data.0.url"}}`
	adapter, err := parseAdapterFile("my-adapter", valid)
	if err != nil {
		t.Fatalf("parse adapter: %v", err)
	}
	renderCtx := adapterRenderContext{APIKey: "k", Model: "m", Prompt: "p", Images: []promptImage{{DataURL: "data:image/png;base64,AA==", Base64: "AA=="}}}
	rendered, err := renderAdapterJSON(adapter.Request.JSON, renderCtx)
	if err != nil {
		t.Fatalf("render adapter: %v", err)
	}
	text := string(rendered)
	if !strings.Contains(text, `"m"`) || !strings.Contains(text, `"p"`) || !strings.Contains(text, "data:image/png;base64,AA==") {
		t.Fatalf("rendered body wrong: %s", text)
	}
	if header := renderAdapterString(adapter.Request.Headers["Authorization"], renderCtx); header != "Bearer k" {
		t.Fatalf("header placeholder not rendered: %s", header)
	}

	unknown := `{"id":"my-adapter","request":{"path":"/x","json":{"prompt":"{{unknown}}"}},"response":{"imagePath":"data.0.url"}}`
	if _, err := parseAdapterFile("my-adapter", unknown); err == nil {
		t.Fatal("unknown placeholder must fail")
	}

	mismatch := `{"id":"other","request":{"path":"/x","json":{}},"response":{"imagePath":"data.0.url"}}`
	if _, err := parseAdapterFile("my-adapter", mismatch); err == nil {
		t.Fatal("adapter id mismatch must fail")
	}
}

func TestParseImageSourceVariants(t *testing.T) {
	cases := []string{
		`{"data":[{"url":"https://example.com/a.png"}]}`,
		`{"images":["` + pngDataURL() + `"]}`,
		`{"choices":[{"message":{"content":"` + pngDataURL() + `"}}]}`,
		`{"data":[{"b64_json":"` + pngBase64 + `"}]}`,
	}
	for _, body := range cases {
		source := extractImageFromResponse([]byte(body))
		if source == "" {
			t.Fatalf("no image source for %s", body)
		}
	}
}

func TestGenerateClosesLoopWithFakeProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/generations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer sk-test" {
			t.Fatalf("unexpected auth: %s", auth)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"url": pngDataURL()}}})
	}))
	defer server.Close()

	input := newInput(t, map[string]any{"action": actionGenerate, "prompt": "一只猫"})
	writeProviders(t, input.ToolDataDirectory, `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"`+server.URL+`","apiKey":"sk-test","protocol":"images","defaultModel":"m1"}]}`)
	session := &fakeSession{}
	output := Execute(context.Background(), input, session)
	metadata := decodeOutput(t, output)
	if metadata["provider"] != "demo" || metadata["protocol"] != protocolImages {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	if len(session.written) != 1 {
		t.Fatalf("expected one session write, got %d", len(session.written))
	}
	if !strings.HasPrefix(session.written[0].DataURL, "data:image/png;base64,") {
		t.Fatalf("written image is not a data url: %s", session.written[0].DataURL)
	}
}

func TestGenerateUsesSessionReferenceImages(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		received <- body
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"url": pngDataURL()}}})
	}))
	defer server.Close()

	input := newInput(t, map[string]any{"action": actionGenerate, "prompt": "改成蓝色", "referenceImages": []any{"1"}})
	writeProviders(t, input.ToolDataDirectory, `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"`+server.URL+`","apiKey":"sk-test","protocol":"chat","defaultModel":"m1"}]}`)
	session := &fakeSession{images: []sessionImage{{ID: "att-1", Name: "原图"}}}
	output := Execute(context.Background(), input, session)
	decodeOutput(t, output)
	body := <-received
	if !strings.Contains(body, "image_url") {
		t.Fatalf("chat request must carry reference image: %s", body)
	}
}

func TestGenerateDeniedCapabilityPassesThrough(t *testing.T) {
	input := newInput(t, map[string]any{"action": actionGenerate, "prompt": "一只猫", "referenceImages": []any{"1"}})
	writeProviders(t, input.ToolDataDirectory, `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"https://example.com","apiKey":"sk-test","protocol":"images","defaultModel":"m1"}]}`)
	session := &fakeSession{images: []sessionImage{{ID: "att-1", Name: "原图"}}, listDenied: true}
	output := Execute(context.Background(), input, session)
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failure, got %s", output.Status)
	}
	if !strings.Contains(output.Error, toolcontrol.CapabilityDeniedMessage) {
		t.Fatalf("denied message must pass through: %s", output.Error)
	}
}

func TestSessionImagesListsAttachments(t *testing.T) {
	session := &fakeSession{images: []sessionImage{{ID: "att-1", Name: "图一"}, {ID: "att-2", Name: "图二"}}}
	output := Execute(context.Background(), newInput(t, map[string]any{"action": actionSessionImages}), session)
	metadata := decodeOutput(t, output)
	if metadata["count"] != 2 {
		t.Fatalf("unexpected count: %#v", metadata)
	}
	if !strings.Contains(output.Content, "1. 图一") || !strings.Contains(output.Content, "2. 图二") {
		t.Fatalf("unexpected content: %s", output.Content)
	}
}

func TestSessionActionsFailWithoutHost(t *testing.T) {
	for _, action := range []string{actionSessionImages, actionGenerate} {
		arguments := map[string]any{"action": action}
		if action == actionGenerate {
			arguments["prompt"] = "x"
		}
		output := Execute(context.Background(), newInput(t, arguments), nil)
		if output.Status != types.ToolStatusFailed {
			t.Fatalf("action %s without host must fail", action)
		}
	}
}

func TestUnknownActionFails(t *testing.T) {
	output := Execute(context.Background(), newInput(t, map[string]any{"action": "nope"}), nil)
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("unknown action must fail, got %s", output.Status)
	}
}

func TestAdapterProtocolEndToEnd(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		received <- body
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"image_b64": pngBase64}})
	}))
	defer server.Close()

	input := newInput(t, map[string]any{"action": actionConfigWrite, "file": "adapters/custom.json", "content": `{"id":"custom","request":{"path":"/custom","json":{"model":"{{model}}","prompt":"{{prompt}}"}},"response":{"imagePath":"result.image_b64"}}`})
	decodeOutput(t, Execute(context.Background(), input, nil))
	writeProviders(t, input.ToolDataDirectory, `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"`+server.URL+`","apiKey":"sk-test","protocol":"adapter","adapter":"custom","defaultModel":"m1"}]}`)

	generate := newInput(t, map[string]any{"action": actionGenerate, "prompt": "一只猫"})
	generate.ToolDataDirectory = input.ToolDataDirectory
	session := &fakeSession{}
	output := Execute(context.Background(), generate, session)
	metadata := decodeOutput(t, output)
	if metadata["protocol"] != protocolAdapter {
		t.Fatalf("unexpected protocol: %#v", metadata)
	}
	body := <-received
	if !strings.Contains(body, `"m1"`) || !strings.Contains(body, "一只猫") {
		t.Fatalf("adapter request not rendered: %s", body)
	}
	if len(session.written) != 1 {
		t.Fatalf("expected one session write")
	}
}

func TestNormalizeImageInputAcceptsBareBase64(t *testing.T) {
	image, err := normalizeImageInput(pngBase64)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if image.Mime != "image/png" || image.DataURL != pngDataURL() {
		t.Fatalf("unexpected image: %#v", image)
	}
	decoded, err := base64.StdEncoding.DecodeString(image.Base64)
	if err != nil || len(decoded) == 0 {
		t.Fatalf("base64 invalid: %v", err)
	}
}

func TestGenerateRejectsTinyReferenceImageLocally(t *testing.T) {
	var hits int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"url": validPngDataURL()}}})
	}))
	defer server.Close()

	input := newInput(t, map[string]any{"action": actionGenerate, "prompt": "验证", "referenceImages": []any{pngDataURL()}})
	writeProviders(t, input.ToolDataDirectory, `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"`+server.URL+`","apiKey":"sk-test","protocol":"chat","defaultModel":"m1"}]}`)
	output := Execute(context.Background(), input, &fakeSession{})
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("tiny reference must fail, got %s", output.Status)
	}
	if !strings.Contains(output.Error, "尺寸过小") {
		t.Fatalf("error must name the size problem: %s", output.Error)
	}
	if output.Metadata["retryable"] != false {
		t.Fatalf("size failure must be non-retryable: %#v", output.Metadata)
	}
	if atomic.LoadInt64(&hits) != 0 {
		t.Fatalf("tiny reference must not reach upstream, hits = %d", hits)
	}
}

func TestGenerateRejectsTimeoutOutOfRange(t *testing.T) {
	for _, timeoutMs := range []int64{1, 1000, 4000000} {
		input := newInput(t, map[string]any{"action": actionGenerate, "prompt": "验证", "timeoutMs": timeoutMs})
		writeProviders(t, input.ToolDataDirectory, `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"https://example.com","apiKey":"sk-test","protocol":"images","defaultModel":"m1"}]}`)
		output := Execute(context.Background(), input, &fakeSession{})
		if output.Status != types.ToolStatusFailed {
			t.Fatalf("timeoutMs=%d must fail, got %s", timeoutMs, output.Status)
		}
		if !strings.Contains(output.Error, "5000-3600000") {
			t.Fatalf("error must state the range: %s", output.Error)
		}
	}
}

func TestGenerateRetriesTransientUpstreamFailure(t *testing.T) {
	var hits int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&hits, 1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":{"message":"upstream busy"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"url": validPngDataURL()}}})
	}))
	defer server.Close()

	input := newInput(t, map[string]any{"action": actionGenerate, "prompt": "验证"})
	writeProviders(t, input.ToolDataDirectory, `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"`+server.URL+`","apiKey":"sk-test","protocol":"images","defaultModel":"m1"}]}`)
	session := &fakeSession{}
	output := Execute(context.Background(), input, session)
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("transient 502 must succeed after retry, got %s: %s", output.Status, output.Error)
	}
	if atomic.LoadInt64(&hits) != 2 {
		t.Fatalf("expected exactly one retry, hits = %d", hits)
	}
	if len(session.written) != 1 {
		t.Fatalf("expected one session write, got %d", len(session.written))
	}
}

func TestGenerateLabelsPersistentUpstreamFailureAsRetryable(t *testing.T) {
	var hits int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"still busy"}}`))
	}))
	defer server.Close()

	input := newInput(t, map[string]any{"action": actionGenerate, "prompt": "验证"})
	writeProviders(t, input.ToolDataDirectory, `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"`+server.URL+`","apiKey":"sk-test","protocol":"images","defaultModel":"m1"}]}`)
	output := Execute(context.Background(), input, &fakeSession{})
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("persistent 503 must fail, got %s", output.Status)
	}
	if output.Metadata["retryable"] != true {
		t.Fatalf("503 failure must be retryable: %#v", output.Metadata)
	}
	if !strings.Contains(output.Error, "已自动重试 1 次") {
		t.Fatalf("failure must note the retry: %s", output.Error)
	}
	if atomic.LoadInt64(&hits) != 2 {
		t.Fatalf("expected two attempts, hits = %d", hits)
	}
}

func TestConfigErrorsDoNotLeakPaths(t *testing.T) {
	dataDir := t.TempDir()
	input := newInput(t, map[string]any{"action": actionConfigRead, "file": "missing.json"})
	input.ToolDataDirectory = dataDir
	output := Execute(context.Background(), input, nil)
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("missing file must fail, got %s", output.Status)
	}
	if strings.Contains(output.Error, dataDir) || strings.Contains(output.Error, ".tmp-") {
		t.Fatalf("error leaked internal path: %s", output.Error)
	}
	if !strings.Contains(output.Error, "missing.json") {
		t.Fatalf("error must keep logical path: %s", output.Error)
	}
}

func TestConfigWriteLockSerializesConcurrentEdits(t *testing.T) {
	dataDir := t.TempDir()
	withDataDir := func(arguments map[string]any) types.ToolExecutionInput {
		input := newInput(t, arguments)
		input.ToolDataDirectory = dataDir
		return input
	}
	original := `{"defaultProvider":"demo","providers":[{"id":"demo","baseUrl":"https://example.com","apiKey":"sk-real","protocol":"images","defaultModel":"flare"}]}`
	decodeOutput(t, Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigWrite, "file": "providers.json", "content": original}), nil))

	edits := []map[string]any{
		{"action": actionConfigEdit, "file": "providers.json", "field": "providers.0.defaultModel", "value": "sunburst"},
		{"action": actionConfigEdit, "file": "providers.json", "field": "providers.0.name", "value": "并发一"},
		{"action": actionConfigEdit, "file": "providers.json", "field": "providers.0.baseUrl", "value": "https://example.org"},
	}
	results := make(chan types.ToolExecutionOutput, len(edits))
	start := make(chan struct{})
	for _, edit := range edits {
		go func(arguments map[string]any) {
			<-start
			results <- Execute(context.Background(), withDataDir(arguments), nil)
		}(edit)
	}
	close(start)
	failures := 0
	for range edits {
		if output := <-results; output.Status != types.ToolStatusSuccess {
			failures++
		}
	}
	if failures != 0 {
		t.Fatalf("all concurrent edits must succeed, failures = %d", failures)
	}
	read := Execute(context.Background(), withDataDir(map[string]any{"action": actionConfigRead, "file": "providers.json"}), nil)
	decodeOutput(t, read)
	for _, expected := range []string{"sunburst", "并发一", "https://example.org"} {
		if !strings.Contains(read.Content, expected) {
			t.Fatalf("concurrent edit lost %q: %s", expected, read.Content)
		}
	}
}
