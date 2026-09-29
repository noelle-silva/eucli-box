package datastorage

import (
	"context"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) LoadConversationImageConfig(ctx context.Context) (types.ConversationImageConfig, error) {
	return loadMetaConfig(ctx, s.paths.conversationImageConfigFile(), defaultConversationImageConfig(), normalizeConversationImageConfigForStorage)
}

func (s *system) SaveConversationImageConfig(ctx context.Context, config types.ConversationImageConfig) (types.ConversationImageConfig, error) {
	config = normalizeConversationImageConfigForStorage(config)
	config.UpdatedAt = time.Now().UTC()
	if err := writeJSON(ctx, s.paths.conversationImageConfigFile(), config); err != nil {
		return types.ConversationImageConfig{}, err
	}
	return config, nil
}

func defaultConversationImageConfig() types.ConversationImageConfig {
	return types.DefaultConversationImageConfig()
}

// normalizeConversationImageConfigForStorage 归一化落盘配置：张数缺省补默认值
// 并夹在合法范围内，不改变开关语义。
func normalizeConversationImageConfigForStorage(config types.ConversationImageConfig) types.ConversationImageConfig {
	if config.OriginalBudgetCount <= 0 {
		config.OriginalBudgetCount = types.ConversationImageOriginalBudgetDefault
	}
	if config.HistoryBudgetCount <= 0 {
		config.HistoryBudgetCount = types.ConversationImageHistoryBudgetDefault
	}
	return types.NormalizeConversationImageConfig(config)
}
