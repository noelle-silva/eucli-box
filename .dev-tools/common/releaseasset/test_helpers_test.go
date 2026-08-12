package releaseasset

import (
	"os"
	"path/filepath"
)

func writeTestAssetTree(root string, recipe Recipe) error {
	for _, file := range recipe.RequiredFiles {
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		payload := make([]byte, file.Size)
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func appendFile(root string, name string) error {
	file, err := os.OpenFile(filepath.Join(root, filepath.FromSlash(name)), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write([]byte("changed")); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
