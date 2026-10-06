package types

import "time"

const DefaultStickerNamingSystemPrompt = `你是“表情包取名助手”。

你会收到一张表情包图片。你的任务：根据图片内容给它取一个简短、好记的中文名字。

输出要求：
- 只输出名字本身（纯文本）
- 不要输出引号、不要输出解释
- 不要包含 / 或 \ 或 ] 或换行
- 尽量不超过 12 个汉字`

type StickerItem struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	RelPath   string    `json:"relPath"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type StickerCategory struct {
	Name      string        `json:"name"`
	Items     []StickerItem `json:"items"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

type StickerCategorySummary struct {
	Name      string    `json:"name"`
	Count     int       `json:"count"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type StickerLibrary struct {
	Categories []StickerCategorySummary `json:"categories"`
	Map        map[string][]StickerItem `json:"map"`
	UpdatedAt  time.Time                `json:"updatedAt"`
}

type StickerNameRequest struct {
	CategoryName string `json:"categoryName"`
	StickerName  string `json:"stickerName"`
}

type StickerNamingConfig struct {
	Enabled       bool            `json:"enabled"`
	ModelPick     string          `json:"modelPick,omitempty"`
	CustomModelID string          `json:"customModelId,omitempty"`
	Coordinate    ModelCoordinate `json:"coordinate"`
	SystemPrompt  string          `json:"systemPrompt"`
	Temperature   float64         `json:"temperature"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

type StickerNameResult struct {
	Name    string      `json:"name"`
	Sticker StickerItem `json:"sticker"`
	Changed bool        `json:"changed"`
}
