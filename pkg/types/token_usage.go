package types

// TokenUsage 是一次模型调用上游返回的用量真值，按服务商字段格式归一后的统一容器。
// 上游给什么就记什么，不做任何加减换算；上游没有给出的项一律留空，表示未知。
type TokenUsage struct {
	InputTokens  *int `json:"inputTokens,omitempty"`
	OutputTokens *int `json:"outputTokens,omitempty"`
	TotalTokens  *int `json:"totalTokens,omitempty"`
}

// IsEmpty 报告该用量是否没有任何真值：三项全空即视为未知。
func (usage *TokenUsage) IsEmpty() bool {
	if usage == nil {
		return true
	}
	return usage.InputTokens == nil && usage.OutputTokens == nil && usage.TotalTokens == nil
}
