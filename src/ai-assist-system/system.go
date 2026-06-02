package aiassist

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"eucli-box/pkg/types"
)

type System interface {
	GenerateStickerName(ctx context.Context, request types.StickerNameRequest) (types.StickerNameResult, error)
}

type StickerStorage interface {
	LoadStickerCategory(ctx context.Context, categoryName string) (types.StickerCategory, error)
	LoadStickerImage(ctx context.Context, relPath string) (string, error)
	RenameSticker(ctx context.Context, categoryName string, oldStickerName string, newStickerName string) (types.StickerItem, error)
	LoadStickerNamingConfig(ctx context.Context) (types.StickerNamingConfig, error)
}

type ModelSystem interface {
	Complete(ctx context.Context, request types.ModelRequest) (types.ModelResponse, error)
}

type Config struct{}

type system struct {
	storage StickerStorage
	models  ModelSystem
}

func NewSystem(config Config, storage StickerStorage, models ModelSystem) (System, error) {
	if storage == nil {
		return nil, assistInvalid("sticker storage dependency is required", nil)
	}
	if models == nil {
		return nil, assistInvalid("model system dependency is required", nil)
	}
	return &system{storage: storage, models: models}, nil
}

func (s *system) GenerateStickerName(ctx context.Context, request types.StickerNameRequest) (types.StickerNameResult, error) {
	categoryName := strings.TrimSpace(request.CategoryName)
	stickerName := strings.TrimSpace(request.StickerName)
	if categoryName == "" {
		return types.StickerNameResult{}, assistInvalid("categoryName is required", nil)
	}
	if stickerName == "" {
		return types.StickerNameResult{}, assistInvalid("stickerName is required", nil)
	}
	config, err := s.storage.LoadStickerNamingConfig(ctx)
	if err != nil {
		return types.StickerNameResult{}, err
	}
	if !config.Enabled {
		return types.StickerNameResult{}, assistInvalid("sticker naming is disabled", nil)
	}
	if config.ModelPick == "__custom__" && strings.TrimSpace(config.CustomModelID) != "" {
		config.Coordinate.ModelID = strings.TrimSpace(config.CustomModelID)
	}
	if strings.TrimSpace(config.Coordinate.ProviderID) == "" || strings.TrimSpace(config.Coordinate.ModelID) == "" {
		return types.StickerNameResult{}, assistInvalid("model coordinate is required", nil)
	}

	sticker, err := s.loadStickerByName(ctx, categoryName, stickerName)
	if err != nil {
		return types.StickerNameResult{}, err
	}
	imageDataURL, err := s.storage.LoadStickerImage(ctx, sticker.RelPath)
	if err != nil {
		return types.StickerNameResult{}, err
	}

	prompt := strings.TrimSpace(config.SystemPrompt)
	if prompt == "" {
		prompt = types.DefaultStickerNamingSystemPrompt
	}
	temperature := config.Temperature
	if temperature <= 0 {
		temperature = 0.2
	}
	response, err := s.models.Complete(ctx, types.ModelRequest{
		Coordinate:  config.Coordinate,
		Temperature: temperature,
		Messages: []types.PromptMessage{
			{Role: "system", Content: prompt, Order: 0, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
			{Role: "user", Content: "请根据这张表情包图片取一个名字。", Images: []types.PromptImage{{DataURL: imageDataURL}}, Order: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		},
	})
	if err != nil {
		return types.StickerNameResult{}, assistFailed("failed to generate sticker name", err)
	}

	generated, err := normalizeStickerName(response.Content)
	if err != nil {
		return types.StickerNameResult{}, err
	}
	if generated == sticker.Name {
		return types.StickerNameResult{Name: generated, Sticker: sticker, Changed: false}, nil
	}
	renamed, err := s.storage.RenameSticker(ctx, categoryName, sticker.Name, generated)
	if err != nil {
		return types.StickerNameResult{}, err
	}
	return types.StickerNameResult{Name: generated, Sticker: renamed, Changed: true}, nil
}

func (s *system) loadStickerByName(ctx context.Context, categoryName string, stickerName string) (types.StickerItem, error) {
	category, err := s.storage.LoadStickerCategory(ctx, categoryName)
	if err != nil {
		return types.StickerItem{}, err
	}
	for _, item := range category.Items {
		if item.Name == stickerName {
			return item, nil
		}
	}
	return types.StickerItem{}, assistInvalid("sticker does not exist", nil)
}

func normalizeStickerName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	name = strings.Trim(name, "`'\"")
	name = strings.TrimSpace(strings.Split(strings.ReplaceAll(name, "\r", "\n"), "\n")[0])
	name = strings.Trim(name, "`'\"")
	name = strings.TrimSpace(name)
	if name == "" {
		return "", assistInvalid("generated sticker name is empty", nil)
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "]") || strings.ContainsAny(name, "\n\r") {
		return "", assistInvalid("generated sticker name contains unsupported characters", nil)
	}
	if utf8.RuneCountInString(name) > 80 {
		return "", assistInvalid("generated sticker name is too long", nil)
	}
	return name, nil
}
