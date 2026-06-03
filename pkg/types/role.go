package types

import "time"

type PromptMessage struct {
	ID        string        `json:"id"`
	Role      string        `json:"role"`
	Content   string        `json:"content"`
	Images    []PromptImage `json:"images,omitempty"`
	Order     int           `json:"order"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

type PromptImage struct {
	DataURL string `json:"dataUrl"`
}

type ToolRunMode string

const (
	ToolRunDirect ToolRunMode = "direct"
	ToolRunAsk    ToolRunMode = "ask"
)

type ToolPolicy struct {
	Tools    []string               `json:"tools,omitempty"`
	RunModes map[string]ToolRunMode `json:"runModes,omitempty"`
}

type ModelConfig struct {
	Coordinate  ModelCoordinate `json:"coordinate"`
	Temperature float64         `json:"temperature"`
}

type Role struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Avatar      string          `json:"avatar"`
	Description string          `json:"description"`
	Prompts     []PromptMessage `json:"prompts"`
	ModelConfig ModelConfig     `json:"modelConfig"`
	ToolPolicy  ToolPolicy      `json:"toolPolicy"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type RoleSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Avatar    string    `json:"avatar"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type RoleContext struct {
	RoleID      string           `json:"roleId"`
	RoleName    string           `json:"roleName"`
	Avatar      string           `json:"avatar"`
	Prompts     []PromptMessage  `json:"prompts"`
	ModelConfig ModelConfig      `json:"modelConfig"`
	Messages    []Message        `json:"messages"`
	ToolPolicy  ToolPolicy       `json:"toolPolicy"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
}
