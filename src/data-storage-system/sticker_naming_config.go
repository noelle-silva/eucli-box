package datastorage

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) LoadStickerNamingConfig(ctx context.Context) (types.StickerNamingConfig, error) {
	target := s.paths.stickerNamingConfigFile()
	if !dataFileExists(target) {
		return defaultStickerNamingConfig(), nil
	}
	config, err := readJSON[types.StickerNamingConfig](ctx, target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultStickerNamingConfig(), nil
		}
		return types.StickerNamingConfig{}, err
	}
	return normalizeStickerNamingConfig(config), nil
}

func (s *system) SaveStickerNamingConfig(ctx context.Context, config types.StickerNamingConfig) (types.StickerNamingConfig, error) {
	config = normalizeStickerNamingConfig(config)
	config.UpdatedAt = time.Now().UTC()
	if err := writeJSON(ctx, s.paths.stickerNamingConfigFile(), config); err != nil {
		return types.StickerNamingConfig{}, err
	}
	return config, nil
}

func defaultStickerNamingConfig() types.StickerNamingConfig {
	return normalizeStickerNamingConfig(types.StickerNamingConfig{})
}

func normalizeStickerNamingConfig(config types.StickerNamingConfig) types.StickerNamingConfig {
	config.Coordinate.Kind = strings.TrimSpace(config.Coordinate.Kind)
	config.Coordinate.GroupID = strings.TrimSpace(config.Coordinate.GroupID)
	config.Coordinate.ProviderID = strings.TrimSpace(config.Coordinate.ProviderID)
	config.Coordinate.ProviderName = strings.TrimSpace(config.Coordinate.ProviderName)
	config.Coordinate.ModelID = strings.TrimSpace(config.Coordinate.ModelID)
	config.ModelPick = strings.TrimSpace(config.ModelPick)
	if config.Coordinate.ModelID == "" && config.ModelPick != "__custom__" {
		config.Coordinate.ModelID = config.ModelPick
	}
	if config.Coordinate.Kind == types.ModelCoordinateKindGroup || config.Coordinate.GroupID != "" {
		config.Coordinate.Kind = types.ModelCoordinateKindGroup
		config.Coordinate.ProviderID = ""
		config.Coordinate.ProviderName = ""
	} else if config.Coordinate.ProviderID != "" || config.Coordinate.ProviderName != "" {
		config.Coordinate.Kind = types.ModelCoordinateKindProvider
		config.Coordinate.GroupID = ""
	}
	config.ModelPick = config.Coordinate.ModelID
	config.CustomModelID = ""
	config.SystemPrompt = strings.TrimSpace(config.SystemPrompt)
	if config.SystemPrompt == "" {
		config.SystemPrompt = types.DefaultStickerNamingSystemPrompt
	}
	if config.Temperature <= 0 {
		config.Temperature = 0.2
	}
	if config.UpdatedAt.IsZero() {
		config.UpdatedAt = time.Now().UTC()
	}
	return config
}
