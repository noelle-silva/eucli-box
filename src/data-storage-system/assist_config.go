package datastorage

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) LoadMermaidFixConfig(ctx context.Context) (types.MermaidFixConfig, error) {
	return loadMetaConfig(ctx, s.paths.mermaidFixConfigFile(), defaultMermaidFixConfig(), normalizeMermaidFixConfig)
}

func (s *system) SaveMermaidFixConfig(ctx context.Context, config types.MermaidFixConfig) (types.MermaidFixConfig, error) {
	config = normalizeMermaidFixConfig(config)
	config.UpdatedAt = time.Now().UTC()
	if err := writeJSON(ctx, s.paths.mermaidFixConfigFile(), config); err != nil {
		return types.MermaidFixConfig{}, err
	}
	return config, nil
}

func (s *system) LoadChatTitleNamingConfig(ctx context.Context) (types.ChatTitleNamingConfig, error) {
	return loadMetaConfig(ctx, s.paths.chatTitleNamingConfigFile(), defaultChatTitleNamingConfig(), normalizeChatTitleNamingConfig)
}

func (s *system) SaveChatTitleNamingConfig(ctx context.Context, config types.ChatTitleNamingConfig) (types.ChatTitleNamingConfig, error) {
	config = normalizeChatTitleNamingConfig(config)
	config.UpdatedAt = time.Now().UTC()
	if err := writeJSON(ctx, s.paths.chatTitleNamingConfigFile(), config); err != nil {
		return types.ChatTitleNamingConfig{}, err
	}
	return config, nil
}

func (s *system) LoadContextCompressionConfig(ctx context.Context) (types.ContextCompressionConfig, error) {
	return loadMetaConfig(ctx, s.paths.contextCompressionConfigFile(), defaultContextCompressionConfig(), normalizeContextCompressionConfig)
}

func (s *system) SaveContextCompressionConfig(ctx context.Context, config types.ContextCompressionConfig) (types.ContextCompressionConfig, error) {
	config = normalizeContextCompressionConfig(config)
	config.UpdatedAt = time.Now().UTC()
	if err := writeJSON(ctx, s.paths.contextCompressionConfigFile(), config); err != nil {
		return types.ContextCompressionConfig{}, err
	}
	return config, nil
}

func loadMetaConfig[T any](ctx context.Context, target string, fallback T, normalize func(T) T) (T, error) {
	if !dataFileExists(target) {
		return normalize(fallback), nil
	}
	config, err := readJSON[T](ctx, target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return normalize(fallback), nil
		}
		var zero T
		return zero, err
	}
	return normalize(config), nil
}

func defaultMermaidFixConfig() types.MermaidFixConfig {
	return normalizeMermaidFixConfig(types.MermaidFixConfig{})
}

func defaultChatTitleNamingConfig() types.ChatTitleNamingConfig {
	return normalizeChatTitleNamingConfig(types.ChatTitleNamingConfig{})
}

func defaultContextCompressionConfig() types.ContextCompressionConfig {
	return normalizeContextCompressionConfig(types.ContextCompressionConfig{})
}

func normalizeMermaidFixConfig(config types.MermaidFixConfig) types.MermaidFixConfig {
	config.ModelPick, config.CustomModelID, config.Coordinate = normalizeAssistModelSelection(config.ModelPick, config.CustomModelID, config.Coordinate)
	config.SystemPrompt = strings.TrimSpace(config.SystemPrompt)
	if config.SystemPrompt == "" {
		config.SystemPrompt = types.DefaultMermaidFixSystemPrompt
	}
	if config.Temperature <= 0 {
		config.Temperature = 0.2
	}
	if config.UpdatedAt.IsZero() {
		config.UpdatedAt = time.Now().UTC()
	}
	return config
}

func normalizeChatTitleNamingConfig(config types.ChatTitleNamingConfig) types.ChatTitleNamingConfig {
	config.ModelPick, config.CustomModelID, config.Coordinate = normalizeAssistModelSelection(config.ModelPick, config.CustomModelID, config.Coordinate)
	config.SystemPrompt = strings.TrimSpace(config.SystemPrompt)
	if config.SystemPrompt == "" {
		config.SystemPrompt = types.DefaultChatTitleNamingSystemPrompt
	}
	if config.Temperature <= 0 {
		config.Temperature = 0.2
	}
	if config.UpdatedAt.IsZero() {
		config.UpdatedAt = time.Now().UTC()
	}
	return config
}

func normalizeContextCompressionConfig(config types.ContextCompressionConfig) types.ContextCompressionConfig {
	config.ModelPick, config.CustomModelID, config.Coordinate = normalizeAssistModelSelection(config.ModelPick, config.CustomModelID, config.Coordinate)
	if config.RetainRecentMessages <= 0 {
		config.RetainRecentMessages = types.DefaultContextCompressionRetainRecentMessages
	}
	if config.RetainRecentMessages < types.ContextCompressionRetainRecentMessagesMin {
		config.RetainRecentMessages = types.ContextCompressionRetainRecentMessagesMin
	}
	if config.RetainRecentMessages > types.ContextCompressionRetainRecentMessagesMax {
		config.RetainRecentMessages = types.ContextCompressionRetainRecentMessagesMax
	}
	if config.Temperature <= 0 {
		config.Temperature = 0.2
	}
	if config.UpdatedAt.IsZero() {
		config.UpdatedAt = time.Now().UTC()
	}
	return config
}

func normalizeAssistModelSelection(modelPick string, customModelID string, coordinate types.ModelCoordinate) (string, string, types.ModelCoordinate) {
	coordinate.ProviderID = strings.TrimSpace(coordinate.ProviderID)
	coordinate.ProviderName = strings.TrimSpace(coordinate.ProviderName)
	coordinate.ModelID = strings.TrimSpace(coordinate.ModelID)
	modelPick = strings.TrimSpace(modelPick)
	if coordinate.ModelID == "" && modelPick != "__custom__" {
		coordinate.ModelID = modelPick
	}
	modelPick = coordinate.ModelID
	return modelPick, "", coordinate
}
