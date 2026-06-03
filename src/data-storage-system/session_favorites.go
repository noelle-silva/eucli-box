package datastorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) LoadSessionFavorites(ctx context.Context) (types.SessionFavorites, error) {
	target := s.paths.sessionFavoritesFile()
	if !dataFileExists(target) {
		return defaultSessionFavorites(), nil
	}
	favorites, err := readJSON[types.SessionFavorites](ctx, target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultSessionFavorites(), nil
		}
		return types.SessionFavorites{}, err
	}
	return normalizeSessionFavorites(favorites)
}

func (s *system) SaveSessionFavorites(ctx context.Context, favorites types.SessionFavorites) (types.SessionFavorites, error) {
	normalized, err := normalizeSessionFavorites(favorites)
	if err != nil {
		return types.SessionFavorites{}, err
	}
	if err := writeJSON(ctx, s.paths.sessionFavoritesFile(), normalized); err != nil {
		return types.SessionFavorites{}, err
	}
	return normalized, nil
}

func (s *system) ensureSessionFavoritesFile(ctx context.Context) error {
	if dataFileExists(s.paths.sessionFavoritesFile()) {
		return nil
	}
	if err := writeJSON(ctx, s.paths.sessionFavoritesFile(), defaultSessionFavorites()); err != nil {
		return storageInitFailed("failed to write session favorites", err)
	}
	return nil
}

func defaultSessionFavorites() types.SessionFavorites {
	return types.SessionFavorites{Folders: []types.SessionFavoriteFolder{}, ChatRefsByFolderID: map[string][]types.SessionFavoriteChatRef{}}
}

func normalizeSessionFavorites(favorites types.SessionFavorites) (types.SessionFavorites, error) {
	if len(favorites.Folders) > 1000 {
		return types.SessionFavorites{}, storageInvalid("too many favorite folders", nil)
	}
	out := defaultSessionFavorites()
	out.Folders = make([]types.SessionFavoriteFolder, 0, len(favorites.Folders))
	folderIDs := map[string]struct{}{}
	parentByID := map[string]string{}

	for _, folder := range favorites.Folders {
		id, err := cleanID(folder.ID)
		if err != nil {
			return types.SessionFavorites{}, storageInvalid("favorite folder id is invalid", err)
		}
		if _, exists := folderIDs[id]; exists {
			return types.SessionFavorites{}, storageInvalid("favorite folder id is duplicated", nil)
		}

		name := strings.Join(strings.Fields(folder.Name), " ")
		if err := validateSessionFavoriteFolderName(name); err != nil {
			return types.SessionFavorites{}, err
		}
		if folder.CreatedAt <= 0 || folder.UpdatedAt <= 0 {
			return types.SessionFavorites{}, storageInvalid("favorite folder timestamps are required", nil)
		}

		parentID := strings.TrimSpace(folder.ParentID)
		if parentID != "" {
			var parentErr error
			parentID, parentErr = cleanID(parentID)
			if parentErr != nil {
				return types.SessionFavorites{}, storageInvalid("favorite folder parent id is invalid", parentErr)
			}
			if parentID == id {
				return types.SessionFavorites{}, storageInvalid("favorite folder cannot parent itself", nil)
			}
		}

		folderIDs[id] = struct{}{}
		parentByID[id] = parentID
		out.Folders = append(out.Folders, types.SessionFavoriteFolder{ID: id, Name: name, ParentID: parentID, CreatedAt: folder.CreatedAt, UpdatedAt: folder.UpdatedAt})
	}

	for _, folder := range out.Folders {
		if folder.ParentID == "" {
			continue
		}
		if _, exists := folderIDs[folder.ParentID]; !exists {
			return types.SessionFavorites{}, storageInvalid("favorite folder parent does not exist", nil)
		}
	}
	for _, folder := range out.Folders {
		seen := map[string]struct{}{folder.ID: {}}
		for cur := parentByID[folder.ID]; cur != ""; cur = parentByID[cur] {
			if _, exists := seen[cur]; exists {
				return types.SessionFavorites{}, storageInvalid("favorite folder parent cycle detected", nil)
			}
			seen[cur] = struct{}{}
		}
	}

	refsByFolderID := favorites.ChatRefsByFolderID
	if refsByFolderID == nil {
		refsByFolderID = map[string][]types.SessionFavoriteChatRef{}
	}
	for folderID := range refsByFolderID {
		if _, exists := folderIDs[folderID]; !exists {
			return types.SessionFavorites{}, storageInvalid("favorite chat refs reference unknown folder", nil)
		}
	}
	for _, folder := range out.Folders {
		refs, err := normalizeSessionFavoriteChatRefs(refsByFolderID[folder.ID])
		if err != nil {
			return types.SessionFavorites{}, err
		}
		out.ChatRefsByFolderID[folder.ID] = refs
	}

	return out, nil
}

func validateSessionFavoriteFolderName(name string) error {
	if name == "" {
		return storageInvalid("favorite folder name is required", nil)
	}
	if len([]rune(name)) > 60 {
		return storageInvalid("favorite folder name is too long", nil)
	}
	if strings.ContainsAny(name, "/\\\n\r") {
		return storageInvalid("favorite folder name contains unsupported characters", nil)
	}
	return nil
}

func normalizeSessionFavoriteChatRefs(refs []types.SessionFavoriteChatRef) ([]types.SessionFavoriteChatRef, error) {
	if len(refs) > 5000 {
		return nil, storageInvalid("too many favorite chat refs", nil)
	}
	out := make([]types.SessionFavoriteChatRef, 0, len(refs))
	seen := map[string]struct{}{}
	for _, ref := range refs {
		kind := strings.TrimSpace(ref.TargetKind)
		if kind != "role" && kind != "group" {
			return nil, storageInvalid("favorite chat target kind is invalid", nil)
		}
		targetID, err := cleanID(ref.TargetID)
		if err != nil {
			return nil, storageInvalid("favorite chat target id is invalid", err)
		}
		chatID, err := cleanID(ref.ChatID)
		if err != nil {
			return nil, storageInvalid("favorite chat id is invalid", err)
		}
		if ref.AddedAt <= 0 {
			return nil, storageInvalid("favorite chat added timestamp is required", nil)
		}
		key := fmt.Sprintf("%s::%s::%s", kind, targetID, chatID)
		if _, exists := seen[key]; exists {
			return nil, storageInvalid("favorite chat ref is duplicated", nil)
		}
		seen[key] = struct{}{}
		out = append(out, types.SessionFavoriteChatRef{TargetKind: kind, TargetID: targetID, ChatID: chatID, AddedAt: ref.AddedAt})
	}
	return out, nil
}
