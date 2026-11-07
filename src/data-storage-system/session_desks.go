package datastorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) LoadSessionDesks(ctx context.Context) (types.SessionDesks, error) {
	target := s.paths.sessionDesksFile()
	if !dataFileExists(target) {
		return defaultSessionDesks(), nil
	}
	desks, err := readJSON[types.SessionDesks](ctx, target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultSessionDesks(), nil
		}
		return types.SessionDesks{}, err
	}
	return normalizeSessionDesks(desks)
}

func (s *system) SaveSessionDesks(ctx context.Context, desks types.SessionDesks) (types.SessionDesks, error) {
	normalized, err := normalizeSessionDesks(desks)
	if err != nil {
		return types.SessionDesks{}, err
	}
	if err := writeJSON(ctx, s.paths.sessionDesksFile(), normalized); err != nil {
		return types.SessionDesks{}, err
	}
	return normalized, nil
}

func (s *system) ensureSessionDesksFile(ctx context.Context) error {
	if dataFileExists(s.paths.sessionDesksFile()) {
		return nil
	}
	if err := writeJSON(ctx, s.paths.sessionDesksFile(), defaultSessionDesks()); err != nil {
		return storageInitFailed("failed to write session desks", err)
	}
	return nil
}

func defaultSessionDesks() types.SessionDesks {
	return types.SessionDesks{Folders: []types.SessionDeskFolder{}, ChatRefsByFolderID: map[string][]types.SessionDeskChatRef{}}
}

func normalizeSessionDesks(desks types.SessionDesks) (types.SessionDesks, error) {
	if len(desks.Folders) > 1000 {
		return types.SessionDesks{}, storageInvalid("too many desk folders", nil)
	}
	out := defaultSessionDesks()
	out.Folders = make([]types.SessionDeskFolder, 0, len(desks.Folders))
	folderIDs := map[string]struct{}{}
	parentByID := map[string]string{}

	for _, folder := range desks.Folders {
		id, err := cleanID(folder.ID)
		if err != nil {
			return types.SessionDesks{}, storageInvalid("desk folder id is invalid", err)
		}
		if _, exists := folderIDs[id]; exists {
			return types.SessionDesks{}, storageInvalid("desk folder id is duplicated", nil)
		}

		name := strings.Join(strings.Fields(folder.Name), " ")
		if err := validateSessionDeskFolderName(name); err != nil {
			return types.SessionDesks{}, err
		}
		if folder.CreatedAt <= 0 || folder.UpdatedAt <= 0 {
			return types.SessionDesks{}, storageInvalid("desk folder timestamps are required", nil)
		}

		parentID := strings.TrimSpace(folder.ParentID)
		if parentID != "" {
			var parentErr error
			parentID, parentErr = cleanID(parentID)
			if parentErr != nil {
				return types.SessionDesks{}, storageInvalid("desk folder parent id is invalid", parentErr)
			}
			if parentID == id {
				return types.SessionDesks{}, storageInvalid("desk folder cannot parent itself", nil)
			}
		}

		folderIDs[id] = struct{}{}
		parentByID[id] = parentID
		out.Folders = append(out.Folders, types.SessionDeskFolder{ID: id, Name: name, ParentID: parentID, CreatedAt: folder.CreatedAt, UpdatedAt: folder.UpdatedAt})
	}

	for _, folder := range out.Folders {
		if folder.ParentID == "" {
			continue
		}
		if _, exists := folderIDs[folder.ParentID]; !exists {
			return types.SessionDesks{}, storageInvalid("desk folder parent does not exist", nil)
		}
	}
	for _, folder := range out.Folders {
		seen := map[string]struct{}{folder.ID: {}}
		for cur := parentByID[folder.ID]; cur != ""; cur = parentByID[cur] {
			if _, exists := seen[cur]; exists {
				return types.SessionDesks{}, storageInvalid("desk folder parent cycle detected", nil)
			}
			seen[cur] = struct{}{}
		}
	}

	refsByFolderID := desks.ChatRefsByFolderID
	if refsByFolderID == nil {
		refsByFolderID = map[string][]types.SessionDeskChatRef{}
	}
	for folderID := range refsByFolderID {
		if _, exists := folderIDs[folderID]; !exists {
			return types.SessionDesks{}, storageInvalid("desk chat refs reference unknown folder", nil)
		}
	}
	for _, folder := range out.Folders {
		refs, err := normalizeSessionDeskChatRefs(refsByFolderID[folder.ID])
		if err != nil {
			return types.SessionDesks{}, err
		}
		out.ChatRefsByFolderID[folder.ID] = refs
	}

	return out, nil
}

func validateSessionDeskFolderName(name string) error {
	if name == "" {
		return storageInvalid("desk folder name is required", nil)
	}
	if len([]rune(name)) > 60 {
		return storageInvalid("desk folder name is too long", nil)
	}
	if strings.ContainsAny(name, "/\\\n\r") {
		return storageInvalid("desk folder name contains unsupported characters", nil)
	}
	return nil
}

func normalizeSessionDeskChatRefs(refs []types.SessionDeskChatRef) ([]types.SessionDeskChatRef, error) {
	if len(refs) > 5000 {
		return nil, storageInvalid("too many desk chat refs", nil)
	}
	out := make([]types.SessionDeskChatRef, 0, len(refs))
	seen := map[string]struct{}{}
	for _, ref := range refs {
		kind := strings.TrimSpace(ref.TargetKind)
		if kind != "role" && kind != "group" && kind != "workspace" {
			return nil, storageInvalid("desk chat target kind is invalid", nil)
		}
		targetID, err := cleanID(ref.TargetID)
		if err != nil {
			return nil, storageInvalid("desk chat target id is invalid", err)
		}
		chatID, err := cleanID(ref.ChatID)
		if err != nil {
			return nil, storageInvalid("desk chat id is invalid", err)
		}
		roleID := strings.TrimSpace(ref.RoleID)
		if kind == "workspace" {
			if roleID == "" {
				return nil, storageInvalid("desk chat role id is required", nil)
			}
			if roleID, err = cleanID(roleID); err != nil {
				return nil, storageInvalid("desk chat role id is invalid", err)
			}
		} else if roleID != "" {
			return nil, storageInvalid("desk chat role id is only allowed for workspace", nil)
		}
		if ref.AddedAt <= 0 {
			return nil, storageInvalid("desk chat added timestamp is required", nil)
		}
		key := fmt.Sprintf("%s::%s::%s::%s", kind, targetID, roleID, chatID)
		if _, exists := seen[key]; exists {
			return nil, storageInvalid("desk chat ref is duplicated", nil)
		}
		seen[key] = struct{}{}
		out = append(out, types.SessionDeskChatRef{TargetKind: kind, TargetID: targetID, RoleID: roleID, ChatID: chatID, AddedAt: ref.AddedAt})
	}
	return out, nil
}
