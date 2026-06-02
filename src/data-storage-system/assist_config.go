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
	return loadAssistConfig(ctx, s.paths.mermaidFixConfigFile(), defaultMermaidFixConfig(), normalizeMermaidFixConfig)
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
	return loadAssistConfig(ctx, s.paths.chatTitleNamingConfigFile(), defaultChatTitleNamingConfig(), normalizeChatTitleNamingConfig)
}

func (s *system) SaveChatTitleNamingConfig(ctx context.Context, config types.ChatTitleNamingConfig) (types.ChatTitleNamingConfig, error) {
	config = normalizeChatTitleNamingConfig(config)
	config.UpdatedAt = time.Now().UTC()
	if err := writeJSON(ctx, s.paths.chatTitleNamingConfigFile(), config); err != nil {
		return types.ChatTitleNamingConfig{}, err
	}
	return config, nil
}

func loadAssistConfig[T any](ctx context.Context, target string, fallback T, normalize func(T) T) (T, error) {
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

func normalizeAssistModelSelection(modelPick string, customModelID string, coordinate types.ModelCoordinate) (string, string, types.ModelCoordinate) {
	coordinate.ProviderID = strings.TrimSpace(coordinate.ProviderID)
	coordinate.ProviderName = strings.TrimSpace(coordinate.ProviderName)
	coordinate.ModelID = strings.TrimSpace(coordinate.ModelID)
	modelPick = strings.TrimSpace(modelPick)
	customModelID = strings.TrimSpace(customModelID)
	if modelPick == "" {
		if customModelID != "" {
			modelPick = "__custom__"
		} else {
			modelPick = coordinate.ModelID
		}
	}
	if modelPick == "__custom__" && customModelID == "" {
		customModelID = coordinate.ModelID
	}
	if modelPick != "__custom__" && coordinate.ModelID == "" {
		coordinate.ModelID = modelPick
	}
	return modelPick, customModelID, coordinate
}
