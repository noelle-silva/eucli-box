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
	GenerateChatTitle(ctx context.Context, request types.ChatTitleRequest) (types.ChatTitleResult, error)
	FixMermaidInMessage(ctx context.Context, request types.MermaidFixRequest) (types.MermaidFixResult, error)
}

type StickerStorage interface {
	LoadSession(ctx context.Context, roleID string, sessionID string) (types.Session, error)
	UpdateSessionTitle(ctx context.Context, roleID string, sessionID string, title string) (types.Session, error)
	UpdateSessionMessage(ctx context.Context, roleID string, sessionID string, messageID string, patch types.SessionMessagePatch) (types.Message, error)
	LoadMermaidFixConfig(ctx context.Context) (types.MermaidFixConfig, error)
	LoadChatTitleNamingConfig(ctx context.Context) (types.ChatTitleNamingConfig, error)
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
	if !types.HasCompleteModelCoordinate(config.Coordinate) {
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

func (s *system) GenerateChatTitle(ctx context.Context, request types.ChatTitleRequest) (types.ChatTitleResult, error) {
	roleID := strings.TrimSpace(request.RoleID)
	sessionID := strings.TrimSpace(request.SessionID)
	if roleID == "" || sessionID == "" {
		return types.ChatTitleResult{}, assistInvalid("roleId and sessionId are required", nil)
	}
	config, err := s.storage.LoadChatTitleNamingConfig(ctx)
	if err != nil {
		return types.ChatTitleResult{}, err
	}
	if !config.Enabled {
		return types.ChatTitleResult{}, assistInvalid("chat title naming is disabled", nil)
	}
	if !types.HasCompleteModelCoordinate(config.Coordinate) {
		return types.ChatTitleResult{}, assistInvalid("model coordinate is required", nil)
	}
	session, err := s.storage.LoadSession(ctx, roleID, sessionID)
	if err != nil {
		return types.ChatTitleResult{}, err
	}
	parts := make([]string, 0, len(session.Messages))
	for _, message := range session.Messages {
		role := "用户"
		if strings.TrimSpace(message.Type) == "assistant" {
			role = "助手"
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if len([]rune(content)) > 1800 {
			content = string([]rune(content)[:1800]) + "…"
		}
		parts = append(parts, role+"："+content)
	}
	transcript := buildChatTranscriptForTitle(parts)
	if transcript == "" {
		return types.ChatTitleResult{}, assistInvalid("chat transcript is empty", nil)
	}
	response, err := s.models.Complete(ctx, types.ModelRequest{Coordinate: config.Coordinate, Temperature: config.Temperature, Messages: []types.PromptMessage{{Role: "system", Content: config.SystemPrompt, Order: 0, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, {Role: "user", Content: transcript, Order: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}}})
	if err != nil {
		return types.ChatTitleResult{}, assistFailed("failed to generate chat title", err)
	}
	title := normalizeGeneratedChatTitle(response.Content)
	if title == "" {
		return types.ChatTitleResult{}, assistInvalid("generated chat title is empty", nil)
	}
	if _, err := s.storage.UpdateSessionTitle(ctx, roleID, sessionID, title); err != nil {
		return types.ChatTitleResult{}, err
	}
	return types.ChatTitleResult{Title: title}, nil
}

func (s *system) FixMermaidInMessage(ctx context.Context, request types.MermaidFixRequest) (types.MermaidFixResult, error) {
	roleID := strings.TrimSpace(request.RoleID)
	sessionID := strings.TrimSpace(request.SessionID)
	messageID := strings.TrimSpace(request.MessageID)
	oldCode := strings.TrimSpace(request.MermaidSource)
	if roleID == "" || sessionID == "" || messageID == "" || oldCode == "" {
		return types.MermaidFixResult{}, assistInvalid("roleId, sessionId, messageId, and mermaidSource are required", nil)
	}
	config, err := s.storage.LoadMermaidFixConfig(ctx)
	if err != nil {
		return types.MermaidFixResult{}, err
	}
	if !config.Enabled {
		return types.MermaidFixResult{}, assistInvalid("mermaid fix is disabled", nil)
	}
	if !types.HasCompleteModelCoordinate(config.Coordinate) {
		return types.MermaidFixResult{}, assistInvalid("model coordinate is required", nil)
	}
	session, err := s.storage.LoadSession(ctx, roleID, sessionID)
	if err != nil {
		return types.MermaidFixResult{}, err
	}
	message := types.Message{}
	found := false
	for _, item := range session.Messages {
		if item.ID == messageID {
			message = item
			found = true
			break
		}
	}
	if !found {
		return types.MermaidFixResult{}, assistInvalid("message does not exist", nil)
	}
	userContent := "请修复下面这段 Mermaid 源码，使其可以正确渲染。\n\n" + oldCode
	if text := strings.TrimSpace(request.RenderErrorMsg); text != "" {
		userContent += "\n\n渲染错误信息：\n" + text
	}
	response, err := s.models.Complete(ctx, types.ModelRequest{Coordinate: config.Coordinate, Temperature: config.Temperature, Messages: []types.PromptMessage{{Role: "system", Content: config.SystemPrompt, Order: 0, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, {Role: "user", Content: userContent, Order: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}}})
	if err != nil {
		return types.MermaidFixResult{}, assistFailed("failed to fix mermaid", err)
	}
	newCode := normalizeGeneratedMermaidCode(response.Content)
	if newCode == "" {
		return types.MermaidFixResult{}, assistInvalid("generated mermaid is empty", nil)
	}
	updatedContent, replaced := replaceMermaidFenceOnce(message.Content, oldCode, newCode)
	if !replaced {
		return types.MermaidFixResult{}, assistInvalid("target mermaid block was not found in message content", nil)
	}
	if _, err := s.storage.UpdateSessionMessage(ctx, roleID, sessionID, messageID, types.SessionMessagePatch{Content: &updatedContent}); err != nil {
		return types.MermaidFixResult{}, err
	}
	return types.MermaidFixResult{MessageID: messageID, MermaidSource: newCode, UpdatedContent: updatedContent}, nil
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
