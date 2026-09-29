package types

import "time"

type PromptMessage struct {
	ID      string        `json:"id"`
	Role    string        `json:"role"`
	Content string        `json:"content"`
	Parts   []MessagePart `json:"parts,omitempty"`
	Images  []PromptImage `json:"images,omitempty"`
	// ToolImages 是随工具结果一起发给模型的产物图片只读视图：
	// 只由提示词组装链路填充，不落盘；每张图与一次工具调用配对。
	ToolImages []PromptToolImage `json:"toolImages,omitempty"`
	Order      int               `json:"order"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

type PromptImage struct {
	DataURL string `json:"dataUrl"`
}

// PromptToolImage 是工具产物图片的提示词视图：调用标识用于配对，
// 附件标识与数据用于锚点文字与图片本体。
type PromptToolImage struct {
	CallID       string `json:"callId"`
	AttachmentID string `json:"attachmentId"`
	DataURL      string `json:"dataUrl"`
}

type ToolRunMode string

const (
	ToolRunDirect ToolRunMode = "direct"
	ToolRunAsk    ToolRunMode = "ask"
)

type ToolPolicy struct {
	Tools       []string               `json:"tools,omitempty"`
	NativeTools []string               `json:"nativeTools,omitempty"`
	RunModes    map[string]ToolRunMode `json:"runModes,omitempty"`
}

type ModelConfig struct {
	Coordinate  ModelCoordinate `json:"coordinate"`
	Temperature float64         `json:"temperature"`
}

type Role struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	Prompts            []PromptMessage `json:"prompts"`
	ModelConfig        ModelConfig     `json:"modelConfig"`
	ToolPolicy         ToolPolicy      `json:"toolPolicy"`
	HookPromptPresetID string          `json:"hookPromptPresetId,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

type RoleSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type RoleContext struct {
	RoleID             string           `json:"roleId"`
	RoleName           string           `json:"roleName"`
	Prompts            []PromptMessage  `json:"prompts"`
	ModelConfig        ModelConfig      `json:"modelConfig"`
	Messages           []Message        `json:"messages"`
	ToolPolicy         ToolPolicy       `json:"toolPolicy"`
	HookPromptPresetID string           `json:"hookPromptPresetId,omitempty"`
	Tools              []ToolDefinition `json:"tools,omitempty"`
	NativeTools        []ToolDefinition `json:"nativeTools,omitempty"`
}
