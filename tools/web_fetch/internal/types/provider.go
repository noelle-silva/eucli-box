package types

import "time"

type ProviderProtocol string

const (
	ProviderProtocolOpenAI    ProviderProtocol = "openai"
	ProviderProtocolAnthropic ProviderProtocol = "anthropic"
)

type Provider struct {
	ID               string                    `json:"id"`
	Name             string                    `json:"name"`
	BaseURL          string                    `json:"baseUrl"`
	Key              string                    `json:"key,omitempty"`
	Protocol         ProviderProtocol          `json:"protocol"`
	APIKeyStrategy   RotationStrategy          `json:"apiKeyStrategy"`
	APIKeys          []ProviderAPIKey          `json:"apiKeys,omitempty"`
	Models           []ModelInfo               `json:"models,omitempty"`
	RegisteredModels []ProviderRegisteredModel `json:"registeredModels,omitempty"`
	CreatedAt        time.Time                 `json:"createdAt"`
	UpdatedAt        time.Time                 `json:"updatedAt"`
}

type ProviderSummary struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	Protocol             ProviderProtocol `json:"protocol"`
	APIKeyCount          int              `json:"apiKeyCount"`
	EnabledAPIKeyCount   int              `json:"enabledApiKeyCount"`
	RegisteredModelCount int              `json:"registeredModelCount"`
	UpdatedAt            time.Time        `json:"updatedAt"`
}

type RotationStrategy string

const (
	RotationStrategySequential     RotationStrategy = "sequential"
	RotationStrategyWeightedRandom RotationStrategy = "weighted_random"
)

type ProviderAPIKey struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Key       string    `json:"key"`
	Enabled   bool      `json:"enabled"`
	Weight    int       `json:"weight"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ProviderRegisteredModel struct {
	ID                     string          `json:"id"`
	Name                   string          `json:"name"`
	SourceModelID          string          `json:"sourceModelId"`
	SupportsReasoning      bool            `json:"supportsReasoning,omitempty"`
	DefaultReasoningEffort ReasoningEffort `json:"defaultReasoningEffort,omitempty"`
	CreatedAt              time.Time       `json:"createdAt"`
	UpdatedAt              time.Time       `json:"updatedAt"`
}

type ModelInfo struct {
	ID                     string          `json:"id"`
	Name                   string          `json:"name"`
	SupportsReasoning      bool            `json:"supportsReasoning,omitempty"`
	DefaultReasoningEffort ReasoningEffort `json:"defaultReasoningEffort,omitempty"`
}

type ModelCoordinate struct {
	Kind         string `json:"kind,omitempty"`
	GroupID      string `json:"groupId,omitempty"`
	ProviderID   string `json:"providerId"`
	ProviderName string `json:"providerName"`
	ModelID      string `json:"modelId"`
}

type ModelGroup struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Models    []ModelGroupModel `json:"models"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

type ModelGroupModel struct {
	ID                     string             `json:"id"`
	Name                   string             `json:"name"`
	Strategy               RotationStrategy   `json:"strategy"`
	SupportsReasoning      bool               `json:"supportsReasoning,omitempty"`
	DefaultReasoningEffort ReasoningEffort    `json:"defaultReasoningEffort,omitempty"`
	Members                []ModelGroupMember `json:"members"`
	CreatedAt              time.Time          `json:"createdAt"`
	UpdatedAt              time.Time          `json:"updatedAt"`
}

type ModelGroupMember struct {
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelId"`
	Weight     int    `json:"weight"`
}

type ModelRequest struct {
	Coordinate      ModelCoordinate  `json:"coordinate"`
	Messages        []PromptMessage  `json:"messages"`
	Temperature     float64          `json:"temperature"`
	ReasoningEffort ReasoningEffort  `json:"reasoningEffort,omitempty"`
	Tools           []ToolDefinition `json:"tools,omitempty"`
	Stream          bool             `json:"stream,omitempty"`
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
	ID                 string       `json:"id"`
	Content            string       `json:"content"`
	Reasoning          string       `json:"reasoning,omitempty"`
	ReasoningSource    string       `json:"reasoningSource,omitempty"`
	ReasoningSignature string       `json:"reasoningSignature,omitempty"`
	ReasoningData      string       `json:"reasoningData,omitempty"`
	ToolIntents        []ToolIntent `json:"toolIntents,omitempty"`
	Raw                []byte       `json:"raw,omitempty"`
	CreatedAt          time.Time    `json:"createdAt"`
}

type ModelStreamEventType string

const (
	ModelStreamEventContentDelta   ModelStreamEventType = "content_delta"
	ModelStreamEventReasoningDelta ModelStreamEventType = "reasoning_delta"
)

type ModelStreamEvent struct {
	Type               ModelStreamEventType `json:"type"`
	ContentDelta       string               `json:"contentDelta,omitempty"`
	Content            string               `json:"content,omitempty"`
	ReasoningDelta     string               `json:"reasoningDelta,omitempty"`
	Reasoning          string               `json:"reasoning,omitempty"`
	ReasoningSource    string               `json:"reasoningSource,omitempty"`
	ReasoningSignature string               `json:"reasoningSignature,omitempty"`
	ReasoningData      string               `json:"reasoningData,omitempty"`
	CreatedAt          time.Time            `json:"createdAt"`
}

type ModelStreamHandler func(event ModelStreamEvent) error
