package datastorage

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var roleAvatarFiles = []string{"avatar.png", "avatar.jpg", "avatar.webp", "avatar.bin", "avatar.json"}

var roleAvatarAllowedImageMIMEs = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}

func (s *system) SaveRoleAvatar(ctx context.Context, roleID string, dataURL string) error {
	if err := ctx.Err(); err != nil {
		return storageWriteFailed("write cancelled", err)
	}
	if err := s.requireRoleExists(roleID); err != nil {
		return err
	}
	image, err := decodeImageDataURL(dataURL, roleAvatarAllowedImageMIMEs)
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
	if err := writeRoleAvatarFile(ctx, dir, image); err != nil {
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

func writeRoleAvatarFile(ctx context.Context, dir string, image storedImage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target := filepath.Join(dir, "avatar."+image.Ext)
	tmp, err := os.CreateTemp(dir, ".avatar-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(image.Payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := removeRoleAvatarNamedFiles(dir); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
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
	if err := removeRoleAvatarNamedFiles(dir); err != nil {
		return err
	}
	return removeRoleAvatarTempFiles(dir)
}

func removeRoleAvatarNamedFiles(dir string) error {
	for _, fileName := range roleAvatarFiles {
		err := os.Remove(filepath.Join(dir, fileName))
		if err == nil || errors.Is(err, os.ErrNotExist) {
			continue
		}
		return err
	}
	return nil
}

func removeRoleAvatarTempFiles(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".avatar-") || !strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		err := os.Remove(filepath.Join(dir, entry.Name()))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
