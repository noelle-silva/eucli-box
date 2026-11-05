package datastorage

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// roleAvatarBaseName 是角色头像的文件基名：目录内只保留一个扩展名。
const roleAvatarBaseName = "avatar"

func (s *system) SaveRoleAvatar(ctx context.Context, roleID string, dataURL string) error {
	if err := ctx.Err(); err != nil {
		return storageWriteFailed("write cancelled", err)
	}
	if err := s.requireRoleExists(roleID); err != nil {
		return err
	}
	image, err := decodeImageDataURL(dataURL, entityIconImageFormats)
	if err != nil {
		return storageInvalid("avatar image must be a png, jpg, or webp data url", err)
	}
	dir, err := s.roleAvatarDir(roleID)
	if err != nil {
		return err
	}
	if err := ensureDirs(dir); err != nil {
		return storageWriteFailed("failed to create role avatar directory", err)
	}
	if err := writeEntityIconFile(ctx, dir, roleAvatarBaseName, image); err != nil {
		return storageWriteFailed("failed to write role avatar", err)
	}
	return nil
}

func (s *system) LoadRoleAvatar(ctx context.Context, roleID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", storageReadFailed("read cancelled", err)
	}
	if err := s.requireRoleExists(roleID); err != nil {
		return "", err
	}
	dir, err := s.roleAvatarDir(roleID)
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
			return "", storageReadFailed("role avatar does not exist", err)
		}
		return "", storageReadFailed("failed to read role avatar", err)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(payload), nil
}

func (s *system) DeleteRoleAvatar(ctx context.Context, roleID string) error {
	if err := ctx.Err(); err != nil {
		return storageDeleteFailed("delete cancelled", err)
	}
	if err := s.requireRoleExists(roleID); err != nil {
		return err
	}
	dir, err := s.roleAvatarDir(roleID)
	if err != nil {
		return err
	}
	if err := removeRoleAvatarFiles(dir); err != nil {
		return storageDeleteFailed("failed to delete role avatar", err)
	}
	return nil
}

func (s *system) requireRoleExists(roleID string) error {
	target, err := s.paths.roleDataFile(roleID)
	if err != nil {
		return err
	}
	if !dataFileExists(target) {
		return storageNotFound("role does not exist", nil)
	}
	return nil
}

func (s *system) roleAvatarDir(roleID string) (string, error) {
	dir, err := s.paths.roleDir(roleID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "attachments", "avatar"), nil
}

func findRoleAvatarFile(dir string) (string, string, error) {
	candidates := []struct {
		Name string
		Mime string
	}{
		{Name: "avatar.png", Mime: "image/png"},
		{Name: "avatar.jpg", Mime: "image/jpeg"},
		{Name: "avatar.webp", Mime: "image/webp"},
	}
	for _, candidate := range candidates {
		info, err := os.Stat(filepath.Join(dir, candidate.Name))
		if err == nil && !info.IsDir() {
			return candidate.Name, candidate.Mime, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", "", storageReadFailed("failed to inspect role avatar", err)
		}
	}
	return "", "", storageReadFailed("role avatar does not exist", os.ErrNotExist)
}

func removeRoleAvatarFiles(dir string) error {
	if err := removeEntityIconFiles(dir, roleAvatarBaseName); err != nil {
		return err
	}
	return removeRoleAvatarTempFiles(dir)
}

func removeRoleAvatarTempFiles(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	prefix := "." + roleAvatarBaseName + "-"
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		err := os.Remove(filepath.Join(dir, entry.Name()))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
