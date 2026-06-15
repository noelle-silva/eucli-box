package datastorage

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
)

func (s *system) SaveChatGroupAvatar(ctx context.Context, groupID string, dataURL string) error {
	if err := ctx.Err(); err != nil {
		return storageWriteFailed("write cancelled", err)
	}
	if err := s.requireChatGroupExists(groupID); err != nil {
		return err
	}
	image, err := decodeImageDataURL(dataURL, roleAvatarAllowedImageMIMEs)
	if err != nil {
		return storageInvalid("avatar image must be a png, jpg, or webp data url", err)
	}
	dir, err := s.chatGroupAvatarDir(groupID)
	if err != nil {
		return err
	}
	if err := ensureDirs(dir); err != nil {
		return storageWriteFailed("failed to create group avatar directory", err)
	}
	if err := writeRoleAvatarFile(ctx, dir, image); err != nil {
		return storageWriteFailed("failed to write group avatar", err)
	}
	return nil
}

func (s *system) LoadChatGroupAvatar(ctx context.Context, groupID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", storageReadFailed("read cancelled", err)
	}
	if err := s.requireChatGroupExists(groupID); err != nil {
		return "", err
	}
	dir, err := s.chatGroupAvatarDir(groupID)
	if err != nil {
		return "", err
	}
	fileName, mime, err := findRoleAvatarFile(dir)
	if err != nil {
		return "", err
	}
	payload, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", storageReadFailed("group avatar does not exist", err)
		}
		return "", storageReadFailed("failed to read group avatar", err)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(payload), nil
}

func (s *system) DeleteChatGroupAvatar(ctx context.Context, groupID string) error {
	if err := ctx.Err(); err != nil {
		return storageDeleteFailed("delete cancelled", err)
	}
	if err := s.requireChatGroupExists(groupID); err != nil {
		return err
	}
	dir, err := s.chatGroupAvatarDir(groupID)
	if err != nil {
		return err
	}
	if err := removeRoleAvatarFiles(dir); err != nil {
		return storageDeleteFailed("failed to delete group avatar", err)
	}
	return nil
}

func (s *system) requireChatGroupExists(groupID string) error {
	target, err := s.paths.groupDataFile(groupID)
	if err != nil {
		return err
	}
	if !dataFileExists(target) {
		return storageNotFound("group does not exist", nil)
	}
	return nil
}

func (s *system) chatGroupAvatarDir(groupID string) (string, error) {
	dir, err := s.paths.groupDir(groupID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "attachments", "avatar"), nil
}
