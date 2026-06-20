package placeholder

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) LoadPlaceholderLibrary(ctx context.Context) (types.PlaceholderLibrary, error) {
	if err := ctx.Err(); err != nil {
		return types.PlaceholderLibrary{}, placeholderReadFailed("read cancelled", err)
	}
	if !dataFileExists(s.libraryFile) {
		return normalizePlaceholderLibrary(types.PlaceholderLibrary{}), nil
	}
	payload, err := os.ReadFile(s.libraryFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return normalizePlaceholderLibrary(types.PlaceholderLibrary{}), nil
		}
		return types.PlaceholderLibrary{}, placeholderReadFailed("failed to read placeholder library", err)
	}
	var library types.PlaceholderLibrary
	if err := json.Unmarshal(payload, &library); err != nil {
		return types.PlaceholderLibrary{}, placeholderReadFailed("failed to decode placeholder library", err)
	}
	return normalizePlaceholderLibrary(library), nil
}

func (s *system) SavePlaceholderLibrary(ctx context.Context, library types.PlaceholderLibrary) (types.PlaceholderLibrary, error) {
	if err := ctx.Err(); err != nil {
		return types.PlaceholderLibrary{}, placeholderWriteFailed("write cancelled", err)
	}
	library = normalizePlaceholderLibrary(library)
	if err := validatePlaceholderNamesUnique(library.Placeholders); err != nil {
		return types.PlaceholderLibrary{}, err
	}
	if err := writeJSON(ctx, s.libraryFile, library); err != nil {
		return types.PlaceholderLibrary{}, err
	}
	return library, nil
}

func writeJSON(ctx context.Context, target string, value any) error {
	if err := ctx.Err(); err != nil {
		return placeholderWriteFailed("write cancelled", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return placeholderWriteFailed("failed to create parent directory", err)
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return placeholderWriteFailed("failed to marshal placeholder library", err)
	}
	payload = append(payload, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(target), ".tmp-*.json")
	if err != nil {
		return placeholderWriteFailed("failed to create temporary file", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return placeholderWriteFailed("failed to write temporary file", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return placeholderWriteFailed("failed to sync temporary file", err)
	}
	if err := tmp.Close(); err != nil {
		return placeholderWriteFailed("failed to close temporary file", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return placeholderWriteFailed("failed to replace placeholder library", err)
	}
	return nil
}

func dataFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func normalizePlaceholderLibrary(library types.PlaceholderLibrary) types.PlaceholderLibrary {
	placeholders := normalizePlaceholderItems(library.Placeholders)
	folders := normalizePlaceholderFolders(library.Folders, placeholders)
	return types.PlaceholderLibrary{Placeholders: placeholders, Folders: folders}
}

func normalizePlaceholderItems(items []types.PlaceholderItem) []types.PlaceholderItem {
	out := make([]types.PlaceholderItem, 0, len(items))
	for _, item := range items {
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			continue
		}
		if item.CreatedAt.IsZero() {
			item.CreatedAt = time.Now().UTC()
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func normalizePlaceholderFolders(folders []types.PlaceholderFolder, placeholders []types.PlaceholderItem) []types.PlaceholderFolder {
	knownNames := map[string]struct{}{}
	for _, item := range placeholders {
		knownNames[item.Name] = struct{}{}
	}
	out := make([]types.PlaceholderFolder, 0, len(folders))
	seenIDs := map[string]struct{}{}
	now := time.Now().UTC()
	for _, folder := range folders {
		folder.ID = strings.TrimSpace(folder.ID)
		folder.Name = strings.TrimSpace(folder.Name)
		folder.ParentID = strings.TrimSpace(folder.ParentID)
		if folder.ID == "" || folder.Name == "" {
			continue
		}
		if _, ok := seenIDs[folder.ID]; ok {
			continue
		}
		seenIDs[folder.ID] = struct{}{}
		if folder.CreatedAt.IsZero() {
			folder.CreatedAt = now
		}
		if folder.UpdatedAt.IsZero() {
			folder.UpdatedAt = folder.CreatedAt
		}
		folder.PlaceholderNames = normalizeFolderPlaceholderNames(folder.PlaceholderNames, knownNames)
		out = append(out, folder)
	}
	validIDs := map[string]struct{}{}
	for _, folder := range out {
		validIDs[folder.ID] = struct{}{}
	}
	for index := range out {
		if out[index].ParentID == out[index].ID {
			out[index].ParentID = ""
		}
		if out[index].ParentID != "" {
			if _, ok := validIDs[out[index].ParentID]; !ok {
				out[index].ParentID = ""
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ParentID != out[j].ParentID {
			return out[i].ParentID < out[j].ParentID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func normalizeFolderPlaceholderNames(names []string, knownNames map[string]struct{}) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := knownNames[name]; !ok {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func validatePlaceholderNamesUnique(items []types.PlaceholderItem) error {
	seen := map[string]struct{}{}
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			return placeholderInvalid("placeholder name already exists", nil)
		}
		seen[name] = struct{}{}
	}
	return nil
}
