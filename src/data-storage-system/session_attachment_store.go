package datastorage

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

var sessionAttachmentAllowedImageMIMEs = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp", "image/gif": "gif"}

func (s *system) SaveSessionMessageAttachment(ctx context.Context, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	return s.saveSessionMessageAttachment(ctx, roleSessionScope(roleID), sessionID, attachment)
}

func (s *system) SaveGroupSessionMessageAttachment(ctx context.Context, groupID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	return s.saveSessionMessageAttachment(ctx, groupSessionScope(groupID), sessionID, attachment)
}

func (s *system) SaveWorkspaceSessionMessageAttachment(ctx context.Context, workspaceID string, roleID string, sessionID string, attachment types.RunAttachment) (types.MessageAttachment, error) {
	return s.saveSessionMessageAttachment(ctx, workspaceSessionScope(workspaceID, roleID), sessionID, attachment)
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

	if !isImageAttachmentKind(attachment.Kind) {
		return types.MessageAttachment{}, storageInvalid("message attachment must be an image", nil)
	}
	return s.saveSessionImageAttachment(ctx, scope, sessionID, attachment)
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
	if len(parts) != 7 && len(parts) != 8 {
		return "", "", storageInvalid("session attachment image path is invalid", nil)
	}
	if parts[0] != "sessions" {
		return "", "", storageInvalid("session attachment image path is invalid", nil)
	}
	var scope sessionScope
	var sessionID string
	var attachmentID string
	var fileName string
	if len(parts) == 8 {
		if parts[1] != "workspaces" || parts[5] != "attachments" {
			return "", "", storageInvalid("session attachment image path is invalid", nil)
		}
		scope = workspaceSessionScope(parts[2], parts[3])
		sessionID = parts[4]
		attachmentID = parts[6]
		fileName = strings.TrimSpace(parts[7])
	} else {
		if parts[4] != "attachments" {
			return "", "", storageInvalid("session attachment image path is invalid", nil)
		}
		switch parts[1] {
		case "roles":
			scope = roleSessionScope(parts[2])
		case "groups":
			scope = groupSessionScope(parts[2])
		default:
			return "", "", storageInvalid("session attachment image path is invalid", nil)
		}
		sessionID = parts[3]
		attachmentID = parts[5]
		fileName = strings.TrimSpace(parts[6])
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
	if scope.Kind == sessionScopeWorkspace {
		return s.paths.workspaceSessionAttachmentDir(scope.WorkspaceID, scope.RoleID, sessionID, attachmentID)
	}
	return s.paths.sessionAttachmentDir(scope.ID, sessionID, attachmentID)
}

func sessionAttachmentRelPath(scope sessionScope, sessionID string, attachmentID string, fileName string) string {
	if scope.Kind == sessionScopeGroup {
		return filepath.ToSlash(filepath.Join("sessions", "groups", scope.ID, sessionID, "attachments", attachmentID, fileName))
	}
	if scope.Kind == sessionScopeWorkspace {
		return filepath.ToSlash(filepath.Join("sessions", "workspaces", scope.WorkspaceID, scope.RoleID, sessionID, "attachments", attachmentID, fileName))
	}
	return filepath.ToSlash(filepath.Join("sessions", "roles", scope.ID, sessionID, "attachments", attachmentID, fileName))
}

func isImageAttachmentKind(kind string) bool {
	return strings.EqualFold(strings.TrimSpace(kind), "image")
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
