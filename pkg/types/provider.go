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
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelId"`
}

type ModelRequest struct {
	Coordinate  ModelCoordinate  `json:"coordinate"`
	Messages    []PromptMessage  `json:"messages"`
	Temperature float64          `json:"temperature"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
}

type ModelResponse struct {
	ID          string       `json:"id"`
	Content     string       `json:"content"`
	ToolIntents []ToolIntent `json:"toolIntents,omitempty"`
	Raw         []byte       `json:"raw,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
}
