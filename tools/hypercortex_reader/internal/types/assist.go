package types

import "time"

type MermaidFixConfig struct {
	Enabled       bool            `json:"enabled"`
	ModelPick     string          `json:"modelPick,omitempty"`
	CustomModelID string          `json:"customModelId,omitempty"`
	Coordinate    ModelCoordinate `json:"coordinate"`
	SystemPrompt  string          `json:"systemPrompt"`
	Temperature   float64         `json:"temperature"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

type ChatTitleNamingConfig struct {
	Enabled       bool            `json:"enabled"`
	ModelPick     string          `json:"modelPick,omitempty"`
	CustomModelID string          `json:"customModelId,omitempty"`
	Coordinate    ModelCoordinate `json:"coordinate"`
	SystemPrompt  string          `json:"systemPrompt"`
	Temperature   float64         `json:"temperature"`
	UpdatedAt     time.Time       `json:"updatedAt"`
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
	Temperature          float64         `json:"temperature"`
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
