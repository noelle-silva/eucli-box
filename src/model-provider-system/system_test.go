package modelprovider

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

func TestNewSystemRejectsMissingDependencies(t *testing.T) {
	storage := newFakeProviderStorage()
	if _, err := NewSystem(Config{}, nil, storage); err == nil {
		t.Fatalf("expected missing network error")
	}
	network := &fakeNetwork{}
	if _, err := NewSystem(Config{}, network, nil); err == nil {
		t.Fatalf("expected missing storage error")
	}
}

func TestSaveProviderValidatesProvider(t *testing.T) {
	system := newTestProviderSystem(t, &fakeNetwork{}, newFakeProviderStorage())
	err := system.SaveProvider(context.Background(), types.Provider{ID: "bad", Name: "Bad", BaseURL: "https://example.com", Key: "key", Protocol: "bad"})
	assertAppErrorCode(t, err, "provider.unsupported_protocol")
}

func TestRefreshModelsUsesNetworkSystemAndSavesModels(t *testing.T) {
	storage := newFakeProviderStorage()
	provider := testOpenAIProvider()
	provider.BaseURL = "https://api.example.test/v1/"
	storage.providers[provider.ID] = provider
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`{"data":[{"id":"gpt-4.1"}]}`)}}
	system := newTestProviderSystem(t, network, storage)

	models, err := system.RefreshModels(context.Background(), provider.ID)
	if err != nil {
		t.Fatalf("RefreshModels() error = %v", err)
	}
	if len(models) != 1 || models[0].ID != "gpt-4.1" {
		t.Fatalf("models = %#v", models)
	}
	if network.lastRequest.URL != "https://api.example.test/v1/models" {
		t.Fatalf("url = %s", network.lastRequest.URL)
	}
	if network.lastRequest.Headers["Authorization"] != "Bearer secret" {
		t.Fatalf("authorization header = %#v", network.lastRequest.Headers)
	}
	if storage.providers[provider.ID].Models[0].ID != "gpt-4.1" {
		t.Fatalf("saved provider = %#v", storage.providers[provider.ID])
	}
}

func TestRefreshModelsUsesConfiguredTimeout(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.modelRequestConfig = types.ModelRequestConfig{ListModelsTimeoutMs: 45_000, CompletionTimeoutMs: types.ModelRequestCompletionTimeoutDefaultMs, StreamIdleTimeoutMs: types.ModelRequestStreamIdleTimeoutDefaultMs}
	storage.providers["openai-main"] = testOpenAIProvider()
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`{"data":[{"id":"gpt-4.1"}]}`)}}
	system := newTestProviderSystem(t, network, storage)

	_, err := system.RefreshModels(context.Background(), "openai-main")
	if err != nil {
		t.Fatalf("RefreshModels() error = %v", err)
	}
	if network.lastRequest.Timeout != 45*time.Second {
		t.Fatalf("timeout = %s", network.lastRequest.Timeout)
	}
}

func TestResolveModelFailsWhenModelMissing(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["openai-main"] = testOpenAIProvider()
	system := newTestProviderSystem(t, &fakeNetwork{}, storage)
	_, _, err := system.ResolveModel(context.Background(), types.ModelCoordinate{ProviderID: "openai-main", ModelID: "missing"})
	assertAppErrorCode(t, err, "provider.model_not_found")
}

func TestResolveModelMapsRegisteredModelToSourceModel(t *testing.T) {
	storage := newFakeProviderStorage()
	provider := testOpenAIProvider()
	provider.RegisteredModels = []types.ProviderRegisteredModel{{ID: "fast", Name: "Fast", SourceModelID: "gpt-4.1"}}
	storage.providers["openai-main"] = provider
	system := newTestProviderSystem(t, &fakeNetwork{}, storage)

	_, model, err := system.ResolveModel(context.Background(), types.ModelCoordinate{ProviderID: "openai-main", ModelID: "fast"})
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	if model.ID != "gpt-4.1" || model.Name != "Fast" {
		t.Fatalf("model = %#v", model)
	}
}

func TestResolveModelGroupUsesRegisteredMemberModel(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["openai-main"] = testOpenAIProvider()
	storage.modelGroups = []types.ModelGroup{{ID: "main-group", Name: "Main Group", Models: []types.ModelGroupModel{{ID: "balanced", Name: "Balanced", Strategy: types.RotationStrategySequential, Members: []types.ModelGroupMember{{ProviderID: "openai-main", ModelID: "gpt-4.1", Weight: 1}}}}}}
	system := newTestProviderSystem(t, &fakeNetwork{}, storage)

	provider, model, err := system.ResolveModel(context.Background(), types.ModelCoordinate{Kind: "model_group", GroupID: "main-group", ModelID: "balanced"})
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	if provider.ID != "openai-main" || model.ID != "gpt-4.1" {
		t.Fatalf("provider = %#v model = %#v", provider, model)
	}
}

func TestCompleteOpenAIWritesToolsIntoRequest(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["openai-main"] = testOpenAIProvider()
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`{"id":"chatcmpl-1","choices":[{"message":{"content":"hello","tool_calls":[{"id":"call-1","function":{"name":"file-reader","arguments":"{\"path\":\"README.md\"}"}}]}}]}`)}}
	system := newTestProviderSystem(t, network, storage)

	response, err := system.Complete(context.Background(), types.ModelRequest{
		Coordinate:  types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"},
		Messages:    []types.PromptMessage{{Role: "user", Content: "hi"}},
		Temperature: 0.7,
		Tools:       []types.ToolDefinition{{Name: "file-reader", Description: "Read file", InputSchema: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Content != "hello" || len(response.ToolIntents) != 1 || response.ToolIntents[0].ToolName != "file-reader" || response.ToolIntents[0].Source != types.ToolCallSourceNative {
		t.Fatalf("response = %#v", response)
	}
	var body map[string]any
	if err := json.Unmarshal(network.lastRequest.Body, &body); err != nil {
		t.Fatalf("request body is invalid json: %v", err)
	}
	if _, ok := body["tools"]; !ok {
		t.Fatalf("request body missing tools: %s", string(network.lastRequest.Body))
	}
}

func TestCompleteUsesConfiguredTimeout(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.modelRequestConfig = types.ModelRequestConfig{ListModelsTimeoutMs: types.ModelRequestListModelsTimeoutDefaultMs, CompletionTimeoutMs: 180_000, StreamIdleTimeoutMs: types.ModelRequestStreamIdleTimeoutDefaultMs}
	storage.providers["openai-main"] = testOpenAIProvider()
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`{"id":"chatcmpl-1","choices":[{"message":{"content":"done"}}]}`)}}
	system := newTestProviderSystem(t, network, storage)

	_, err := system.Complete(context.Background(), types.ModelRequest{Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"}, Messages: []types.PromptMessage{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if network.lastRequest.Timeout != 180*time.Second {
		t.Fatalf("timeout = %s", network.lastRequest.Timeout)
	}
}

func TestModelToolDescriptionPrefersPromptDescription(t *testing.T) {
	tool := types.ToolDefinition{Description: "Short description", PromptDescription: "Detailed prompt usage"}
	if got := modelToolDescription(tool); got != "Detailed prompt usage" {
		t.Fatalf("description = %q", got)
	}
	tool.PromptDescription = ""
	if got := modelToolDescription(tool); got != "Short description" {
		t.Fatalf("fallback description = %q", got)
	}
}

func TestCompleteOpenAISendsStructuredToolHistory(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["openai-main"] = testOpenAIProvider()
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`{"id":"chatcmpl-1","choices":[{"message":{"content":"done"}}]}`)}}
	system := newTestProviderSystem(t, network, storage)

	_, err := system.Complete(context.Background(), types.ModelRequest{
		Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"},
		Messages:   []types.PromptMessage{{Role: "assistant", Content: "checking", Parts: []types.MessagePart{{Type: "text", Text: "checking"}, {Type: "tool", CallID: "call-1", ToolName: "shell_command", Input: map[string]any{"command": "pwd"}, Result: &types.ToolPartResult{Status: types.ToolStatusSuccess, Content: "ok"}}}}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(network.lastRequest.Body, &body); err != nil {
		t.Fatalf("request body is invalid json: %v", err)
	}
	messages := body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v body=%s", messages, string(network.lastRequest.Body))
	}
	assistant := messages[0].(map[string]any)
	if _, ok := assistant["tool_calls"]; !ok {
		t.Fatalf("assistant missing tool_calls: %#v", assistant)
	}
	toolMessage := messages[1].(map[string]any)
	if toolMessage["role"] != "tool" || toolMessage["tool_call_id"] != "call-1" {
		t.Fatalf("tool message = %#v", toolMessage)
	}
}

func TestCompletePreservesUpstreamErrorDetails(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["openai-main"] = testOpenAIProvider()
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 400, Headers: map[string][]string{"X-Request-Id": {"req-1"}}, Body: []byte(`{"error":{"message":"upstream says no","type":"invalid_request_error"}}`)}}
	system := newTestProviderSystem(t, network, storage)

	_, err := system.Complete(context.Background(), types.ModelRequest{Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"}, Messages: []types.PromptMessage{{Role: "user", Content: "hi"}}})
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not AppError", err)
	}
	if appErr.Code != "provider.service_failed" || appErr.Message != "upstream says no" {
		t.Fatalf("app error = %#v", appErr)
	}
	details, ok := appErr.Details.(map[string]any)
	if !ok {
		t.Fatalf("details = %#v", appErr.Details)
	}
	if details["statusCode"] != 400 || details["body"] != `{"error":{"message":"upstream says no","type":"invalid_request_error"}}` {
		t.Fatalf("details = %#v", details)
	}
}

func TestCompleteOpenAIRejectsToolHistoryWithoutResult(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["openai-main"] = testOpenAIProvider()
	system := newTestProviderSystem(t, &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`{"id":"chatcmpl-1","choices":[{"message":{"content":"done"}}]}`)}}, storage)

	_, err := system.Complete(context.Background(), types.ModelRequest{Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"}, Messages: []types.PromptMessage{{Role: "assistant", Parts: []types.MessagePart{{Type: "tool", CallID: "call-1", ToolName: "shell_command", Input: map[string]any{"command": "pwd"}}}}}})
	assertAppErrorCode(t, err, "provider.invalid_request")
}

func TestCompleteOpenAISkipsHiddenToolHistory(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["openai-main"] = testOpenAIProvider()
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`{"id":"chatcmpl-1","choices":[{"message":{"content":"done"}}]}`)}}
	system := newTestProviderSystem(t, network, storage)

	_, err := system.Complete(context.Background(), types.ModelRequest{
		Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"},
		Messages:   []types.PromptMessage{{Role: "assistant", Content: "checking", Parts: []types.MessagePart{{Type: "tool", CallID: "call-1", ToolName: "shell_command", Input: map[string]any{"command": "pwd"}, Result: &types.ToolPartResult{Status: types.ToolStatusSuccess, Content: "ok"}, Display: map[string]any{"hideResult": true}}}}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if strings.Contains(string(network.lastRequest.Body), "tool_calls") || strings.Contains(string(network.lastRequest.Body), "tool_call_id") {
		t.Fatalf("hidden tool history leaked into request: %s", string(network.lastRequest.Body))
	}
}

func TestCompleteAnthropicSeparatesSystemPrompt(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["anthropic-main"] = testAnthropicProvider()
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`{"id":"msg-1","content":[{"type":"text","text":"hello"},{"type":"tool_use","id":"toolu-1","name":"search","input":{"query":"eucli"}}]}`)}}
	system := newTestProviderSystem(t, network, storage)

	response, err := system.Complete(context.Background(), types.ModelRequest{
		Coordinate: types.ModelCoordinate{ProviderID: "anthropic-main", ModelID: "claude-3-5-sonnet"},
		Messages: []types.PromptMessage{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "hi"},
		},
		Temperature: 0.2,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Content != "hello" || len(response.ToolIntents) != 1 || response.ToolIntents[0].ToolName != "search" || response.ToolIntents[0].Source != types.ToolCallSourceNative {
		t.Fatalf("response = %#v", response)
	}
	var body map[string]any
	if err := json.Unmarshal(network.lastRequest.Body, &body); err != nil {
		t.Fatalf("request body is invalid json: %v", err)
	}
	if body["system"] != "You are helpful" {
		t.Fatalf("system = %#v", body["system"])
	}
	messages := body["messages"].([]any)
	if len(messages) != 1 || !strings.Contains(string(network.lastRequest.Body), `"role":"user"`) {
		t.Fatalf("messages = %#v body=%s", messages, string(network.lastRequest.Body))
	}
}

func TestCompleteAnthropicSendsStructuredToolHistory(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["anthropic-main"] = testAnthropicProvider()
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`{"id":"msg-1","content":[{"type":"text","text":"done"}]}`)}}
	system := newTestProviderSystem(t, network, storage)

	_, err := system.Complete(context.Background(), types.ModelRequest{
		Coordinate: types.ModelCoordinate{ProviderID: "anthropic-main", ModelID: "claude-3-5-sonnet"},
		Messages:   []types.PromptMessage{{Role: "assistant", Content: "checking", Parts: []types.MessagePart{{Type: "tool", CallID: "toolu-1", ToolName: "shell_command", Input: map[string]any{"command": "pwd"}, Result: &types.ToolPartResult{Status: types.ToolStatusSuccess, Content: "ok"}}}}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(network.lastRequest.Body, &body); err != nil {
		t.Fatalf("request body is invalid json: %v", err)
	}
	messages := body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v body=%s", messages, string(network.lastRequest.Body))
	}
	assistant := messages[0].(map[string]any)
	if assistant["role"] != "assistant" || !strings.Contains(string(network.lastRequest.Body), `"type":"tool_use"`) {
		t.Fatalf("assistant message = %#v body=%s", assistant, string(network.lastRequest.Body))
	}
	toolResult := messages[1].(map[string]any)
	if toolResult["role"] != "user" || !strings.Contains(string(network.lastRequest.Body), `"type":"tool_result"`) {
		t.Fatalf("tool result message = %#v body=%s", toolResult, string(network.lastRequest.Body))
	}
}

func TestCompleteAnthropicRejectsToolHistoryWithoutResult(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["anthropic-main"] = testAnthropicProvider()
	system := newTestProviderSystem(t, &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`{"id":"msg-1","content":[{"type":"text","text":"done"}]}`)}}, storage)

	_, err := system.Complete(context.Background(), types.ModelRequest{Coordinate: types.ModelCoordinate{ProviderID: "anthropic-main", ModelID: "claude-3-5-sonnet"}, Messages: []types.PromptMessage{{Role: "assistant", Parts: []types.MessagePart{{Type: "tool", CallID: "toolu-1", ToolName: "shell_command", Input: map[string]any{"command": "pwd"}}}}}})
	assertAppErrorCode(t, err, "provider.invalid_request")
}

func TestCompleteStreamOpenAIEmitsDeltasAndAssemblesResponse(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["openai-main"] = testOpenAIProvider()
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`data: {"id":"chatcmpl-stream","choices":[{"delta":{"content":"he"}}]}

data: {"id":"chatcmpl-stream","choices":[{"delta":{"content":"llo"}}]}

data: {"id":"chatcmpl-stream","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","function":{"name":"file-reader","arguments":"{\"path\":"}}]}}]}

data: {"id":"chatcmpl-stream","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"README.md\"}"}}]}}]}

data: [DONE]

`)}}
	system := newTestProviderSystem(t, network, storage)

	events := []string{}
	response, err := system.CompleteStream(context.Background(), types.ModelRequest{Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"}, Messages: []types.PromptMessage{{Role: "user", Content: "hi"}}, Temperature: 0.7, Tools: []types.ToolDefinition{{Name: "file-reader", Description: "Read file", InputSchema: map[string]any{"type": "object"}}}}, func(event types.ModelStreamEvent) error {
		events = append(events, event.Content)
		return nil
	})
	if err != nil {
		t.Fatalf("CompleteStream() error = %v", err)
	}
	if response.ID != "chatcmpl-stream" || response.Content != "hello" {
		t.Fatalf("response = %#v", response)
	}
	if len(events) != 2 || events[0] != "he" || events[1] != "hello" {
		t.Fatalf("events = %#v", events)
	}
	if len(response.ToolIntents) != 1 || response.ToolIntents[0].ToolName != "file-reader" || response.ToolIntents[0].Source != types.ToolCallSourceNative || response.ToolIntents[0].Arguments["path"] != "README.md" {
		t.Fatalf("tool intents = %#v", response.ToolIntents)
	}
	var body map[string]any
	if err := json.Unmarshal(network.lastRequest.Body, &body); err != nil {
		t.Fatalf("request body is invalid json: %v", err)
	}
	if body["stream"] != true {
		t.Fatalf("stream flag = %#v body=%s", body["stream"], string(network.lastRequest.Body))
	}
}

func TestCompleteStreamUsesConfiguredIdleTimeout(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.modelRequestConfig = types.ModelRequestConfig{ListModelsTimeoutMs: types.ModelRequestListModelsTimeoutDefaultMs, CompletionTimeoutMs: types.ModelRequestCompletionTimeoutDefaultMs, StreamIdleTimeoutMs: 90_000}
	storage.providers["openai-main"] = testOpenAIProvider()
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte("data: {\"id\":\"chatcmpl-stream\",\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\ndata: [DONE]\n\n")}}
	system := newTestProviderSystem(t, network, storage)

	_, err := system.CompleteStream(context.Background(), types.ModelRequest{Coordinate: types.ModelCoordinate{ProviderID: "openai-main", ModelID: "gpt-4.1"}, Messages: []types.PromptMessage{{Role: "user", Content: "hi"}}}, nil)
	if err != nil {
		t.Fatalf("CompleteStream() error = %v", err)
	}
	if network.lastRequest.Timeout != 90*time.Second {
		t.Fatalf("timeout = %s", network.lastRequest.Timeout)
	}
}

func TestSaveModelRequestConfigRejectsOutOfRangeTimeout(t *testing.T) {
	system := newTestProviderSystem(t, &fakeNetwork{}, newFakeProviderStorage())
	_, err := system.SaveModelRequestConfig(context.Background(), types.ModelRequestConfig{ListModelsTimeoutMs: 1, CompletionTimeoutMs: types.ModelRequestCompletionTimeoutDefaultMs, StreamIdleTimeoutMs: types.ModelRequestStreamIdleTimeoutDefaultMs})
	assertAppErrorCode(t, err, "provider.invalid_request")
}

func TestCompleteStreamAnthropicEmitsDeltas(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["anthropic-main"] = testAnthropicProvider()
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`data: {"type":"message_start","message":{"id":"msg-stream"}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"he"}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"llo"}}

`)}}
	system := newTestProviderSystem(t, network, storage)

	events := []string{}
	response, err := system.CompleteStream(context.Background(), types.ModelRequest{Coordinate: types.ModelCoordinate{ProviderID: "anthropic-main", ModelID: "claude-3-5-sonnet"}, Messages: []types.PromptMessage{{Role: "user", Content: "hi"}}, Temperature: 0.2}, func(event types.ModelStreamEvent) error {
		events = append(events, event.Content)
		return nil
	})
	if err != nil {
		t.Fatalf("CompleteStream() error = %v", err)
	}
	if response.ID != "msg-stream" || response.Content != "hello" {
		t.Fatalf("response = %#v", response)
	}
	if len(events) != 2 || events[0] != "he" || events[1] != "hello" {
		t.Fatalf("events = %#v", events)
	}
	var body map[string]any
	if err := json.Unmarshal(network.lastRequest.Body, &body); err != nil {
		t.Fatalf("request body is invalid json: %v", err)
	}
	if body["stream"] != true {
		t.Fatalf("stream flag = %#v body=%s", body["stream"], string(network.lastRequest.Body))
	}
}

func newTestProviderSystem(t *testing.T, network NetworkSystem, storage StorageSystem) System {
	t.Helper()
	system, err := NewSystem(Config{RequestTimeout: time.Second}, network, storage)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	return system
}

func testOpenAIProvider() types.Provider {
	return types.Provider{
		ID:       "openai-main",
		Name:     "OpenAI",
		BaseURL:  "https://api.example.test/v1",
		Key:      "secret",
		Protocol: types.ProviderProtocolOpenAI,
		Models:   []types.ModelInfo{{ID: "gpt-4.1", Name: "gpt-4.1"}},
		RegisteredModels: []types.ProviderRegisteredModel{{
			ID:            "gpt-4.1",
			Name:          "gpt-4.1",
			SourceModelID: "gpt-4.1",
		}},
	}
}

func testAnthropicProvider() types.Provider {
	return types.Provider{
		ID:       "anthropic-main",
		Name:     "Anthropic",
		BaseURL:  "https://api.anthropic.test/v1",
		Key:      "secret",
		Protocol: types.ProviderProtocolAnthropic,
		Models:   []types.ModelInfo{{ID: "claude-3-5-sonnet", Name: "claude-3-5-sonnet"}},
		RegisteredModels: []types.ProviderRegisteredModel{{
			ID:            "claude-3-5-sonnet",
			Name:          "claude-3-5-sonnet",
			SourceModelID: "claude-3-5-sonnet",
		}},
	}
}

type fakeNetwork struct {
	lastRequest types.HTTPRequest
	response    types.HTTPResponse
	err         error
}

func (f *fakeNetwork) Do(ctx context.Context, req types.HTTPRequest) (types.HTTPResponse, error) {
	f.lastRequest = req
	if f.err != nil {
		return types.HTTPResponse{}, f.err
	}
	return f.response, nil
}

func (f *fakeNetwork) DoStream(ctx context.Context, req types.HTTPRequest, onChunk types.HTTPStreamHandler) (types.HTTPResponse, error) {
	f.lastRequest = req
	if f.err != nil {
		return types.HTTPResponse{}, f.err
	}
	if onChunk != nil && len(f.response.Body) > 0 {
		if err := onChunk(types.HTTPStreamChunk{Data: f.response.Body}); err != nil {
			return types.HTTPResponse{}, err
		}
	}
	return f.response, nil
}

type fakeProviderStorage struct {
	providers          map[string]types.Provider
	modelGroups        []types.ModelGroup
	callRecords        []types.CallRecord
	modelRequestConfig types.ModelRequestConfig
}

func newFakeProviderStorage() *fakeProviderStorage {
	return &fakeProviderStorage{providers: map[string]types.Provider{}}
}

func (f *fakeProviderStorage) SaveProvider(ctx context.Context, provider types.Provider) error {
	f.providers[provider.ID] = provider
	return nil
}

func (f *fakeProviderStorage) LoadProvider(ctx context.Context, providerID string) (types.Provider, error) {
	provider, ok := f.providers[providerID]
	if !ok {
		return types.Provider{}, errors.New("provider missing")
	}
	return provider, nil
}

func (f *fakeProviderStorage) ListProviders(ctx context.Context) ([]types.ProviderSummary, error) {
	summaries := make([]types.ProviderSummary, 0, len(f.providers))
	for _, provider := range f.providers {
		provider = normalizeProvider(provider)
		enabledKeyCount := 0
		for _, key := range provider.APIKeys {
			if key.Enabled {
				enabledKeyCount++
			}
		}
		summaries = append(summaries, types.ProviderSummary{ID: provider.ID, Name: provider.Name, Protocol: provider.Protocol, APIKeyCount: len(provider.APIKeys), EnabledAPIKeyCount: enabledKeyCount, RegisteredModelCount: len(provider.RegisteredModels), UpdatedAt: provider.UpdatedAt})
	}
	return summaries, nil
}

func (f *fakeProviderStorage) DeleteProvider(ctx context.Context, providerID string) error {
	delete(f.providers, providerID)
	return nil
}

func (f *fakeProviderStorage) LoadModelRequestConfig(ctx context.Context) (types.ModelRequestConfig, error) {
	return f.modelRequestConfig, nil
}

func (f *fakeProviderStorage) SaveModelRequestConfig(ctx context.Context, config types.ModelRequestConfig) (types.ModelRequestConfig, error) {
	f.modelRequestConfig = config
	return f.modelRequestConfig, nil
}

func (f *fakeProviderStorage) LoadModelGroups(ctx context.Context) ([]types.ModelGroup, error) {
	return f.modelGroups, nil
}

func (f *fakeProviderStorage) SaveModelGroups(ctx context.Context, groups []types.ModelGroup) ([]types.ModelGroup, error) {
	f.modelGroups = groups
	return f.modelGroups, nil
}

func (f *fakeProviderStorage) SaveCallRecord(ctx context.Context, record types.CallRecord) error {
	f.callRecords = append(f.callRecords, record)
	return nil
}

func assertAppErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not AppError", err)
	}
	if appErr.Code != code {
		t.Fatalf("code = %s, want %s", appErr.Code, code)
	}
}
