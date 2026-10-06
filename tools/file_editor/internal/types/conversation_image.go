package types

import "time"

const (
	// ConversationImageOriginalBudgetDefault 是原图预算的默认张数：
	// 最近的若干张图片以原图形式发送给模型。
	ConversationImageOriginalBudgetDefault = 2
	ConversationImageOriginalBudgetMin     = 1
	ConversationImageOriginalBudgetMax     = 100

	// ConversationImageHistoryBudgetDefault 是历史图片预算的默认张数：
	// 发往模型的历史图片最多保留最近若干张，超出的以占位文字替代。
	ConversationImageHistoryBudgetDefault = 6
	ConversationImageHistoryBudgetMin     = 1
	ConversationImageHistoryBudgetMax     = 1000
)

// ConversationImageConfig 是会话图片机制的全局配置：多版本存图与两项
// 发送预算。它只作用于发往模型的请求体，不改变会话中的图片事实。
type ConversationImageConfig struct {
	// MultiVersionEnabled 控制存图时是否另存一张压缩小副本；
	// 关闭时发往模型的图片全部使用原图，两项预算不生效。
	MultiVersionEnabled bool `json:"multiVersionEnabled"`
	// OriginalBudgetEnabled 控制最近若干张图片是否以原图发送；
	// 依赖 MultiVersionEnabled 开启。
	OriginalBudgetEnabled bool `json:"originalBudgetEnabled"`
	// OriginalBudgetCount 是原图预算张数。
	OriginalBudgetCount int `json:"originalBudgetCount"`
	// HistoryBudgetEnabled 控制发往模型的图片张数是否受限；
	// 依赖 MultiVersionEnabled 开启。
	HistoryBudgetEnabled bool `json:"historyBudgetEnabled"`
	// HistoryBudgetCount 是历史图片预算张数。
	HistoryBudgetCount int       `json:"historyBudgetCount"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// NormalizeConversationImageConfig 归一化会话图片配置：零值视为缺省开启，
// 张数夹在合法范围内；关闭多版本存图时两项预算整体失效。
func NormalizeConversationImageConfig(config ConversationImageConfig) ConversationImageConfig {
	config.OriginalBudgetCount = clampInt(config.OriginalBudgetCount, ConversationImageOriginalBudgetMin, ConversationImageOriginalBudgetMax)
	config.HistoryBudgetCount = clampInt(config.HistoryBudgetCount, ConversationImageHistoryBudgetMin, ConversationImageHistoryBudgetMax)
	if config.UpdatedAt.IsZero() {
		config.UpdatedAt = time.Now().UTC()
	}
	return config
}

// DefaultConversationImageConfig 返回默认配置：三项全开，
// 原图预算 2 张、历史图片预算 6 张。
func DefaultConversationImageConfig() ConversationImageConfig {
	return ConversationImageConfig{
		MultiVersionEnabled:   true,
		OriginalBudgetEnabled: true,
		OriginalBudgetCount:   ConversationImageOriginalBudgetDefault,
		HistoryBudgetEnabled:  true,
		HistoryBudgetCount:    ConversationImageHistoryBudgetDefault,
	}
}

// EffectiveOriginalBudgetCount 返回实际生效的原图预算张数：
// 多版本存图或原图预算关闭时为 0（不替换原图）。
func (config ConversationImageConfig) EffectiveOriginalBudgetCount() int {
	if !config.MultiVersionEnabled || !config.OriginalBudgetEnabled {
		return 0
	}
	return config.OriginalBudgetCount
}

// EffectiveHistoryBudgetCount 返回实际生效的历史图片预算张数：
// 多版本存图或历史预算关闭时为 0（不限制张数）。
func (config ConversationImageConfig) EffectiveHistoryBudgetCount() int {
	if !config.MultiVersionEnabled || !config.HistoryBudgetEnabled {
		return 0
	}
	return config.HistoryBudgetCount
}

func clampInt(value int, min int, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
