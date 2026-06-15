package datastorage

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

const maxMessageAttachmentTextRunes = 10_000_000

var sessionAttachmentAllowedImageMIMEs = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp", "image/gif": "gif"}

func (s *system) SaveSessionMessageAttachment(ctx context.Context, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	return s.saveSessionMessageAttachment(ctx, roleSessionScope(roleID), sessionID, attachment)
}

func (s *system) SaveGroupSessionMessageAttachment(ctx context.Context, groupID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	return s.saveSessionMessageAttachment(ctx, groupSessionScope(groupID), sessionID, attachment)
}

func (s *system) saveSessionMessageAttachment(ctx context.Context, scope sessionScope, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	if err := ctx.Err(); err != nil {
		return types.MessageAttachment{}, storageWriteFailed("write cancelled", err)
	}
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return types.MessageAttachment{}, err
	}
	if _, err := cleanID(sessionID); err != nil {
		return types.MessageAttachment{}, err
	}
	if _, err := s.sessionDataFile(scope, sessionID); err != nil {
		return types.MessageAttachment{}, err
	}

	kind := normalizeMessageAttachmentKind(attachment.Kind)
	if kind == "image" {
		return s.saveSessionImageAttachment(ctx, scope, sessionID, attachment)
	}
	return normalizeTextMessageAttachment(attachment, kind)
}

func (s *system) saveSessionImageAttachment(ctx context.Context, scope sessionScope, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	image, err := decodeImageDataURL(attachment.DataURL, sessionAttachmentAllowedImageMIMEs)
	if err != nil {
		return types.MessageAttachment{}, storageInvalid("message image attachment must be a png, jpg, webp, or gif data url", err)
	}
	attachmentID := utils.NewID("att")
	dir, err := s.sessionAttachmentDir(scope, sessionID, attachmentID)
	if err != nil {
		return types.MessageAttachment{}, err
	}
	if err := ensureDirs(dir); err != nil {
		return types.MessageAttachment{}, storageWriteFailed("failed to create session attachment directory", err)
	}
	fileName := "image." + image.Ext
	if err := writeSingleImageFile(ctx, dir, fileName, image); err != nil {
		return types.MessageAttachment{}, storageWriteFailed("failed to write session image attachment", err)
	}
	return types.MessageAttachment{ID: attachmentID, Kind: "image", Name: normalizeAttachmentName(attachment.Name, "图片"), Mime: image.Mime, Path: sessionAttachmentRelPath(scope, sessionID, attachmentID, fileName)}, nil
}

func normalizeTextMessageAttachment(attachment types.RunAttachment, kind string) (types.MessageAttachment, error) {
	text := strings.TrimSpace(attachment.Text)
	if text == "" {
		return types.MessageAttachment{}, storageInvalid("message file attachment text is required", nil)
	}
	if utf8.RuneCountInString(text) > maxMessageAttachmentTextRunes {
		return types.MessageAttachment{}, storageInvalid("message file attachment text is too large", nil)
	}
	fullLen := attachment.FullLen
	if fullLen <= 0 {
		fullLen = utf8.RuneCountInString(text)
	}
	sendLen := attachment.SendLen
	if sendLen <= 0 || sendLen > fullLen {
		sendLen = utf8.RuneCountInString(text)
	}
	sendPct := attachment.SendPct
	if sendPct <= 0 {
		sendPct = 100
	}
	if sendPct > 100 {
		sendPct = 100
	}
	return types.MessageAttachment{ID: utils.NewID("att"), Kind: kind, Name: normalizeAttachmentName(attachment.Name, "文件"), Mime: strings.TrimSpace(attachment.Mime), Lang: normalizeAttachmentLang(attachment.Lang, kind), Text: text, FullLen: fullLen, SendLen: sendLen, SendPct: sendPct}, nil
}

func (s *system) LoadSessionAttachmentImage(ctx context.Context, relPath string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", storageReadFailed("read cancelled", err)
	}
	imagePath, mime, err := s.sessionAttachmentImagePath(relPath)
	if err != nil {
		return "", err
	}
	payload, err := os.ReadFile(imagePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", storageReadFailed("session attachment image does not exist", err)
		}
		return "", storageReadFailed("failed to read session attachment image", err)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(payload), nil
}

func (s *system) sessionAttachmentImagePath(relPath string) (string, string, error) {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	parts := strings.Split(relPath, "/")
	if len(parts) != 6 && len(parts) != 7 {
		return "", "", storageInvalid("session attachment image path is invalid", nil)
	}
	if parts[0] != "sessions" {
		return "", "", storageInvalid("session attachment image path is invalid", nil)
	}
	var scope sessionScope
	var sessionID string
	var attachmentID string
	var fileName string
	if len(parts) == 7 {
		if parts[1] != "groups" || parts[4] != "attachments" {
			return "", "", storageInvalid("session attachment image path is invalid", nil)
		}
		scope = groupSessionScope(parts[2])
		sessionID = parts[3]
		attachmentID = parts[5]
		fileName = strings.TrimSpace(parts[6])
	} else {
		if parts[3] != "attachments" {
			return "", "", storageInvalid("session attachment image path is invalid", nil)
		}
		scope = roleSessionScope(parts[1])
		sessionID = parts[2]
		attachmentID = parts[4]
		fileName = strings.TrimSpace(parts[5])
	}
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return "", "", err
	}
	if _, err := cleanID(sessionID); err != nil {
		return "", "", err
	}
	attachmentID, err = cleanID(attachmentID)
	if err != nil {
		return "", "", err
	}
	if !strings.HasPrefix(fileName, "image.") {
		return "", "", storageInvalid("session attachment image filename is invalid", nil)
	}
	mime, ok := imageMIMEFromExt(filepath.Ext(fileName))
	if !ok {
		return "", "", storageInvalid("session attachment image extension is unsupported", nil)
	}
	dir, err := s.sessionAttachmentDir(scope, sessionID, attachmentID)
	if err != nil {
		return "", "", err
	}
	joined := filepath.Join(dir, fileName)
	if !isWithin(dir, joined) {
		return "", "", storageInvalid("path escapes session attachment directory", nil)
	}
	return joined, mime, nil
}

func (s *system) sessionAttachmentDir(scope sessionScope, sessionID string, attachmentID string) (string, error) {
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return "", err
	}
	if scope.Kind == sessionScopeGroup {
		return s.paths.groupSessionAttachmentDir(scope.ID, sessionID, attachmentID)
	}
	return s.paths.sessionAttachmentDir(scope.ID, sessionID, attachmentID)
}

func sessionAttachmentRelPath(scope sessionScope, sessionID string, attachmentID string, fileName string) string {
	if scope.Kind == sessionScopeGroup {
		return filepath.ToSlash(filepath.Join("sessions", "groups", scope.ID, sessionID, "attachments", attachmentID, fileName))
	}
	return filepath.ToSlash(filepath.Join("sessions", scope.ID, sessionID, "attachments", attachmentID, fileName))
}

func normalizeMessageAttachmentKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "image":
		return "image"
	case "md", "markdown":
		return "md"
	case "pdf":
		return "pdf"
	case "docx":
		return "docx"
	case "ppt", "pptx":
		return "ppt"
	default:
		return "txt"
	}
}

func normalizeAttachmentName(name string, fallback string) string {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return fallback
	}
	runes := []rune(name)
	if len(runes) > 160 {
		return strings.TrimSpace(string(runes[:160]))
	}
	return name
}

func normalizeAttachmentLang(lang string, kind string) string {
	lang = strings.TrimSpace(lang)
	if lang != "" {
		return lang
	}
	if kind == "md" {
		return "markdown"
	}
	return "text"
}
