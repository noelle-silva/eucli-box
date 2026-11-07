package types

// 提示词仓库的作用域：同一套读、写、删动作在这三个归属上运行。
// 作用域只决定文件落点，不代表跨作用域的继承、覆盖或优先级。
const (
	PromptScopeGlobal    = "global"
	PromptScopeRole      = "role"
	PromptScopeWorkspace = "workspace"
)

// PromptLibrary 是一个作用域下的提示词素材集合。
type PromptLibrary struct {
	Prompts []Prompt `json:"prompts"`
}

// Prompt 是一条可复用的提示词素材。
// Description 是给人看的备注：不参与提示词组装，也不会发给模型。
// 时间字段与 Hook 提示词预设同形（RFC3339 字符串、可省略），整库可原样导出。
type Prompt struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Content     string `json:"content"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}
