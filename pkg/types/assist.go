package types

import "time"

// DefaultAssistLowTemperature 是 AI 微服务「低温度」开关打开时使用的温度值。
const DefaultAssistLowTemperature = 0.2

// OptionalTemperature 把「温度开关 + 温度值」解析为模型请求温度：
// 仅当开关显式开启时返回温度值；未设置或关闭时返回 nil，表示不发送温度、走模型默认。
func OptionalTemperature(enabled *bool, value float64) *float64 {
	if enabled == nil || !*enabled {
		return nil
	}
	resolved := value
	return &resolved
}

// AssistTemperature 解析 AI 微服务的低温度开关。
func AssistTemperature(lowTemperature *bool) *float64 {
	return OptionalTemperature(lowTemperature, DefaultAssistLowTemperature)
}

type MermaidFixConfig struct {
	Enabled        bool            `json:"enabled"`
	ModelPick      string          `json:"modelPick,omitempty"`
	CustomModelID  string          `json:"customModelId,omitempty"`
	Coordinate     ModelCoordinate `json:"coordinate"`
	SystemPrompt   string          `json:"systemPrompt"`
	LowTemperature *bool           `json:"lowTemperature,omitempty"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type ChatTitleNamingConfig struct {
	Enabled        bool            `json:"enabled"`
	ModelPick      string          `json:"modelPick,omitempty"`
	CustomModelID  string          `json:"customModelId,omitempty"`
	Coordinate     ModelCoordinate `json:"coordinate"`
	SystemPrompt   string          `json:"systemPrompt"`
	LowTemperature *bool           `json:"lowTemperature,omitempty"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

const (
	DefaultContextCompressionRetainRecentMessages = 15
	ContextCompressionRetainRecentMessagesMin     = 1
	ContextCompressionRetainRecentMessagesMax     = 100
)

type ContextCompressionConfig struct {
	ModelPick            string          `json:"modelPick,omitempty"`
	CustomModelID        string          `json:"customModelId,omitempty"`
	Coordinate           ModelCoordinate `json:"coordinate"`
	RetainRecentMessages int             `json:"retainRecentMessages"`
	LowTemperature       *bool           `json:"lowTemperature,omitempty"`
	UpdatedAt            time.Time       `json:"updatedAt"`
}

const DefaultMermaidFixSystemPrompt = "你是 Mermaid 语法修复器。\n\n你会收到一段 Mermaid 源码（可能无法渲染）。你的任务：在尽量保持原意不变的前提下，修复语法/结构错误，让它可以被 Mermaid 渲染。\n\n输出要求：\n- 只输出修复后的 Mermaid 源码本体\n- 不要输出解释、不要输出 Markdown 代码块标记（不要输出 ```mermaid）"

const DefaultChatTitleNamingSystemPrompt = "你是“聊天标题生成器”。\n\n你会收到一段聊天记录。你的任务：为这段聊天生成一个简短、贴切的中文标题。\n\n输出要求：\n- 只输出标题本身（纯文本）\n- 不要输出引号、不要输出解释\n- 尽量不超过 20 个汉字"

const DefaultContextCompressionSystemPrompt = "You are an expert software development context summarizer.\n\nYour task is to compress an ongoing coding conversation into a concise handoff summary that preserves all information needed to continue the work.\n\nWrite the summary in the same language as the conversation when possible.\n\nUse exactly these sections:\n## Goal\n## Constraints & Preferences\n## Progress\n## Key Decisions\n## Next Steps\n## Critical Context\n## Relevant Files\n\nRules:\n- Preserve concrete requirements, decisions, blockers, and verification status.\n- Preserve file paths, commands, errors, and pending work when they matter.\n- Do not invent facts.\n- Do not include commentary about the summarization process.\n- Output only the updated summary, with no Markdown code fence."

type ChatTitleRequest struct {
	RoleID    string `json:"roleId"`
	SessionID string `json:"sessionId"`
}

type ChatTitleResult struct {
	Title string `json:"title"`
}

type MermaidFixRequest struct {
	RoleID         string `json:"roleId"`
	SessionID      string `json:"sessionId"`
	MessageID      string `json:"messageId"`
	MermaidSource  string `json:"mermaidSource"`
	RenderErrorMsg string `json:"renderErrorMsg,omitempty"`
}

type MermaidFixResult struct {
	MessageID      string `json:"messageId"`
	MermaidSource  string `json:"mermaidSource"`
	UpdatedContent string `json:"updatedContent"`
}
