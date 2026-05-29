package datastorage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func ensureDirs(paths ...string) error {
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(ctx context.Context, target string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return storageWriteFailed("failed to create parent directory", err)
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return storageWriteFailed("failed to marshal json", err)
	}
	payload = append(payload, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(target), ".tmp-*.json")
	if err != nil {
		return storageWriteFailed("failed to create temporary file", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return storageWriteFailed("failed to write temporary file", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return storageWriteFailed("failed to sync temporary file", err)
	}
	if err := tmp.Close(); err != nil {
		return storageWriteFailed("failed to close temporary file", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return storageWriteFailed("failed to replace json file", err)
	}
	return nil
}

func readJSON[T any](ctx context.Context, target string) (T, error) {
	var value T
	if err := ctx.Err(); err != nil {
		return value, storageReadFailed("read cancelled", err)
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return value, storageReadFailed("json file does not exist", err)
		}
		return value, storageReadFailed("failed to read json file", err)
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return value, storageReadFailed("failed to decode json file", err)
	}
	return value, nil
}

func dataFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
