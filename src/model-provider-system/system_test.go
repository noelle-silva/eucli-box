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
	provider := types.Provider{ID: "openai-main", Name: "OpenAI", BaseURL: "https://api.example.test/v1/", Key: "secret", Protocol: types.ProviderProtocolOpenAI}
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

func TestResolveModelFailsWhenModelMissing(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["openai-main"] = types.Provider{ID: "openai-main", Name: "OpenAI", BaseURL: "https://api.example.test/v1", Key: "secret", Protocol: types.ProviderProtocolOpenAI, Models: []types.ModelInfo{{ID: "gpt-4.1"}}}
	system := newTestProviderSystem(t, &fakeNetwork{}, storage)
	_, _, err := system.ResolveModel(context.Background(), types.ModelCoordinate{ProviderID: "openai-main", ModelID: "missing"})
	assertAppErrorCode(t, err, "provider.model_not_found")
}

func TestCompleteOpenAIWritesToolsIntoRequest(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["openai-main"] = types.Provider{ID: "openai-main", Name: "OpenAI", BaseURL: "https://api.example.test/v1", Key: "secret", Protocol: types.ProviderProtocolOpenAI, Models: []types.ModelInfo{{ID: "gpt-4.1"}}}
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
	if response.Content != "hello" || len(response.ToolIntents) != 1 || response.ToolIntents[0].ToolName != "file-reader" {
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

func TestCompleteAnthropicSeparatesSystemPrompt(t *testing.T) {
	storage := newFakeProviderStorage()
	storage.providers["anthropic-main"] = types.Provider{ID: "anthropic-main", Name: "Anthropic", BaseURL: "https://api.anthropic.test/v1", Key: "secret", Protocol: types.ProviderProtocolAnthropic, Models: []types.ModelInfo{{ID: "claude-3-5-sonnet"}}}
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
	if response.Content != "hello" || len(response.ToolIntents) != 1 || response.ToolIntents[0].ToolName != "search" {
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

func newTestProviderSystem(t *testing.T, network NetworkSystem, storage StorageSystem) System {
	t.Helper()
	system, err := NewSystem(Config{RequestTimeout: time.Second}, network, storage)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	return system
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

type fakeProviderStorage struct {
	providers map[string]types.Provider
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
		summaries = append(summaries, types.ProviderSummary{ID: provider.ID, Name: provider.Name, Protocol: provider.Protocol, UpdatedAt: provider.UpdatedAt})
	}
	return summaries, nil
}

func (f *fakeProviderStorage) DeleteProvider(ctx context.Context, providerID string) error {
	delete(f.providers, providerID)
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
