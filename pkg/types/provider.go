package types

import "time"

type ProviderProtocol string

const (
	ProviderProtocolOpenAI    ProviderProtocol = "openai"
	ProviderProtocolAnthropic ProviderProtocol = "anthropic"
)

type Provider struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	BaseURL   string           `json:"baseUrl"`
	Key       string           `json:"key"`
	Protocol  ProviderProtocol `json:"protocol"`
	Models    []ModelInfo      `json:"models,omitempty"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
}

type ProviderSummary struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Protocol  ProviderProtocol `json:"protocol"`
	UpdatedAt time.Time        `json:"updatedAt"`
}

type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ModelCoordinate struct {
	ProviderID   string `json:"providerId"`
	ProviderName string `json:"providerName"`
	ModelID      string `json:"modelId"`
}

type ModelRequest struct {
	Coordinate  ModelCoordinate  `json:"coordinate"`
	Messages    []PromptMessage  `json:"messages"`
	Temperature float64          `json:"temperature"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
}

const (
	ModelRequestListModelsTimeoutDefaultMs = 30_000
	ModelRequestListModelsTimeoutMinMs     = 5_000
	ModelRequestListModelsTimeoutMaxMs     = 120_000

	ModelRequestCompletionTimeoutDefaultMs = 300_000
	ModelRequestCompletionTimeoutMinMs     = 30_000
	ModelRequestCompletionTimeoutMaxMs     = 600_000

	ModelRequestStreamIdleTimeoutDefaultMs = 120_000
	ModelRequestStreamIdleTimeoutMinMs     = 15_000
	ModelRequestStreamIdleTimeoutMaxMs     = 300_000
)

type ModelRequestConfig struct {
	ListModelsTimeoutMs int       `json:"listModelsTimeoutMs"`
	CompletionTimeoutMs int       `json:"completionTimeoutMs"`
	StreamIdleTimeoutMs int       `json:"streamIdleTimeoutMs"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type ModelResponse struct {
	ID          string       `json:"id"`
	Content     string       `json:"content"`
	ToolIntents []ToolIntent `json:"toolIntents,omitempty"`
	Raw         []byte       `json:"raw,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
}

type ModelStreamEventType string

const (
	ModelStreamEventContentDelta ModelStreamEventType = "content_delta"
)

type ModelStreamEvent struct {
	Type         ModelStreamEventType `json:"type"`
	ContentDelta string               `json:"contentDelta,omitempty"`
	Content      string               `json:"content,omitempty"`
	CreatedAt    time.Time            `json:"createdAt"`
}

type ModelStreamHandler func(event ModelStreamEvent) error

type CallRecord struct {
	ID         string    `json:"id"`
	ProviderID string    `json:"providerId"`
	ModelID    string    `json:"modelId"`
	Success    bool      `json:"success"`
	ErrorCode  string    `json:"errorCode,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}
