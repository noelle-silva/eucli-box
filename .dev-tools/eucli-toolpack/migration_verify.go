package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"eucli-box/pkg/types"
)

func copyLegacyBody(ctx context.Context, sourceDir string, targetDir string, bodyPaths []string) error {
	for _, bodyPath := range bodyPaths {
		if err := ctx.Err(); err != nil {
			return err
		}
		source := filepath.Join(sourceDir, bodyPath)
		if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		target := filepath.Join(targetDir, bodyPath)
		if err := copyPath(source, target); err != nil {
			return err
		}
		if err := verifyCopiedPath(source, target); err != nil {
			return err
		}
	}
	return nil
}

func pathCoveredBy(path string, roots []string) bool {
	for _, root := range roots {
		if path == root || pathWithin(root, path) {
			return true
		}
	}
	return false
}

func copyPath(source string, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(source, target)
	}
	return copyFile(source, target, info.Mode())
}

type fileSnapshot struct {
	Path string
	Hash [32]byte
}

func verifyCopiedPath(source string, target string) error {
	sourceSnapshot, err := snapshotPath(source)
	if err != nil {
		return err
	}
	targetSnapshot, err := snapshotPath(target)
	if err != nil {
		return err
	}
	if len(sourceSnapshot) != len(targetSnapshot) {
		return fmt.Errorf("file count differs: source=%d target=%d", len(sourceSnapshot), len(targetSnapshot))
	}
	for index := range sourceSnapshot {
		if sourceSnapshot[index] != targetSnapshot[index] {
			return fmt.Errorf("copied file differs at %s", sourceSnapshot[index].Path)
		}
	}
	return nil
}

func snapshotPath(root string) ([]fileSnapshot, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		hash, err := hashFile(root)
		if err != nil {
			return nil, err
		}
		return []fileSnapshot{{Path: ".", Hash: hash}}, nil
	}
	files := []fileSnapshot{}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		hash, err := hashFile(path)
		if err != nil {
			return err
		}
		files = append(files, fileSnapshot{Path: filepath.ToSlash(relative), Hash: hash})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func hashFile(path string) ([32]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return [32]byte{}, err
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result, nil
}

func verifyStagedTool(bodyDir string, dataDir string, toolID string, expectedSettings types.ToolUserSettings) error {
	definition, err := readJSONValue[types.ToolDefinition](filepath.Join(bodyDir, "definition.json"))
	if err != nil {
		return fmt.Errorf("verify tool %s definition: %w", toolID, err)
	}
	if definition.ID != toolID || definition.BodyDirectory != "." {
		return fmt.Errorf("verify tool %s definition identity and body directory", toolID)
	}
	if err := validateReleaseMetadata(definition.Version, definition.EucliBoxCompatibility); err != nil {
		return fmt.Errorf("verify tool %s release metadata: %w", toolID, err)
	}
	settings, err := readJSONValue[types.ToolUserSettings](filepath.Join(dataDir, "settings.json"))
	if err != nil {
		return fmt.Errorf("verify tool %s settings: %w", toolID, err)
	}
	expectedPayload, _ := json.Marshal(expectedSettings)
	actualPayload, _ := json.Marshal(settings)
	if string(expectedPayload) != string(actualPayload) {
		return fmt.Errorf("verify tool %s settings: values differ", toolID)
	}
	return nil
}

func readJSONValue[T any](path string) (T, error) {
	var value T
	payload, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return value, err
	}
	return value, nil
}
