package datastorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"eucli-box/pkg/types"
)

const (
	maxSessionDeskItems      = 5000
	maxSessionDeskGroups     = 1000
	maxSessionDeskGroupChats = 5000
	maxSessionDeskGroupRunes = 60
	maxSessionDeskColorBytes = 40
	// legacyDeskRootFolderID 是旧版工作桌面承载散装条目的固定根收藏夹。
	legacyDeskRootFolderID = "desk-root"
)

// 分组颜色是受限的样式标记：十六进制或 css 颜色函数，不接受任意字符串。
var sessionDeskGroupColorPattern = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|(hsl|hsla|rgb|rgba)\(\s*[0-9.,%\s]+\))$`)

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
	return types.SessionDesks{Items: []types.SessionDeskItem{}}
}

// normalizeSessionDesks 是工作桌面唯一的写入闸门。
//
// 新版只有 Items 一份表达；旧版两层结构在这里一次性摊平成 Items。
// 无论输入是哪一版，输出永远是同一形态，文件里不会再出现两份数据。
func normalizeSessionDesks(desks types.SessionDesks) (types.SessionDesks, error) {
	items := desks.Items
	if len(items) == 0 {
		migrated, err := migrateLegacySessionDeskItems(desks)
		if err != nil {
			return types.SessionDesks{}, err
		}
		items = migrated
	}
	normalized, err := normalizeSessionDeskItems(items)
	if err != nil {
		return types.SessionDesks{}, err
	}
	return types.SessionDesks{Items: normalized}, nil
}

// migrateLegacySessionDeskItems 把旧版两层结构摊平成有序条目。
//
// 旧形态里承载散装条目的根收藏夹（desk-root）所持条目直接摊到顶层，
// 其余 folder 原样变成一个分组；folders 的先后顺序就是迁移后的顶层顺序。
func migrateLegacySessionDeskItems(desks types.SessionDesks) ([]types.SessionDeskItem, error) {
	if len(desks.LegacyFolders) == 0 && len(desks.LegacyChatRefsByFolderID) == 0 {
		return []types.SessionDeskItem{}, nil
	}

	knownFolderIDs := make(map[string]struct{}, len(desks.LegacyFolders))
	for _, folder := range desks.LegacyFolders {
		knownFolderIDs[strings.TrimSpace(folder.ID)] = struct{}{}
	}
	// 旧版对「引用了不存在的 folder」是错误；迁移期同样快速失败，不替人猜归属。
	for folderID := range desks.LegacyChatRefsByFolderID {
		if _, exists := knownFolderIDs[strings.TrimSpace(folderID)]; !exists {
			return nil, storageInvalid("desk chat refs reference unknown folder", nil)
		}
	}

	items := make([]types.SessionDeskItem, 0, len(desks.LegacyFolders)+len(desks.LegacyChatRefsByFolderID))
	for _, folder := range desks.LegacyFolders {
		if strings.TrimSpace(folder.ID) == legacyDeskRootFolderID {
			for _, ref := range desks.LegacyChatRefsByFolderID[folder.ID] {
				chat := ref
				items = append(items, types.SessionDeskItem{Type: types.SessionDeskItemChat, Chat: &chat})
			}
			continue
		}
		group := types.SessionDeskGroup{
			ID:        folder.ID,
			Name:      folder.Name,
			Chats:     append([]types.SessionDeskChatRef{}, desks.LegacyChatRefsByFolderID[folder.ID]...),
			CreatedAt: folder.CreatedAt,
			UpdatedAt: folder.UpdatedAt,
		}
		items = append(items, types.SessionDeskItem{Type: types.SessionDeskItemGroup, Group: &group})
	}
	return items, nil
}

func normalizeSessionDeskItems(items []types.SessionDeskItem) ([]types.SessionDeskItem, error) {
	if len(items) > maxSessionDeskItems {
		return nil, storageInvalid("too many desk items", nil)
	}

	out := make([]types.SessionDeskItem, 0, len(items))
	// 桌面是一份手边清单：同一条会话在任何位置都只能出现一次。
	seenChats := map[string]struct{}{}
	seenGroups := map[string]struct{}{}
	groupCount := 0

	for _, item := range items {
		switch strings.TrimSpace(item.Type) {
		case types.SessionDeskItemChat:
			if item.Chat == nil {
				return nil, storageInvalid("desk chat item is missing its reference", nil)
			}
			if item.Group != nil {
				return nil, storageInvalid("desk chat item cannot carry a group", nil)
			}
			ref, err := normalizeSessionDeskChatRef(*item.Chat)
			if err != nil {
				return nil, err
			}
			key := sessionDeskChatRefKey(ref)
			if _, exists := seenChats[key]; exists {
				return nil, storageInvalid("desk chat ref is duplicated", nil)
			}
			seenChats[key] = struct{}{}
			out = append(out, types.SessionDeskItem{Type: types.SessionDeskItemChat, Chat: &ref})

		case types.SessionDeskItemGroup:
			if item.Group == nil {
				return nil, storageInvalid("desk group item is missing its group", nil)
			}
			if item.Chat != nil {
				return nil, storageInvalid("desk group item cannot carry a chat", nil)
			}
			groupCount++
			if groupCount > maxSessionDeskGroups {
				return nil, storageInvalid("too many desk groups", nil)
			}
			group, err := normalizeSessionDeskGroup(*item.Group, seenChats)
			if err != nil {
				return nil, err
			}
			if _, exists := seenGroups[group.ID]; exists {
				return nil, storageInvalid("desk group id is duplicated", nil)
			}
			seenGroups[group.ID] = struct{}{}
			out = append(out, types.SessionDeskItem{Type: types.SessionDeskItemGroup, Group: &group})

		default:
			return nil, storageInvalid("desk item type is invalid", nil)
		}
	}
	return out, nil
}

func normalizeSessionDeskGroup(group types.SessionDeskGroup, seenChats map[string]struct{}) (types.SessionDeskGroup, error) {
	id, err := cleanID(group.ID)
	if err != nil {
		return types.SessionDeskGroup{}, storageInvalid("desk group id is invalid", err)
	}
	name := strings.Join(strings.Fields(group.Name), " ")
	if err := validateSessionDeskGroupName(name); err != nil {
		return types.SessionDeskGroup{}, err
	}
	color, err := normalizeSessionDeskGroupColor(group.Color)
	if err != nil {
		return types.SessionDeskGroup{}, err
	}
	if group.CreatedAt <= 0 || group.UpdatedAt <= 0 {
		return types.SessionDeskGroup{}, storageInvalid("desk group timestamps are required", nil)
	}
	if len(group.Chats) > maxSessionDeskGroupChats {
		return types.SessionDeskGroup{}, storageInvalid("too many desk group chats", nil)
	}

	chats := make([]types.SessionDeskChatRef, 0, len(group.Chats))
	for _, ref := range group.Chats {
		normalized, err := normalizeSessionDeskChatRef(ref)
		if err != nil {
			return types.SessionDeskGroup{}, err
		}
		key := sessionDeskChatRefKey(normalized)
		if _, exists := seenChats[key]; exists {
			return types.SessionDeskGroup{}, storageInvalid("desk chat ref is duplicated", nil)
		}
		seenChats[key] = struct{}{}
		chats = append(chats, normalized)
	}

	return types.SessionDeskGroup{
		ID:        id,
		Name:      name,
		Color:     color,
		Collapsed: group.Collapsed,
		Chats:     chats,
		CreatedAt: group.CreatedAt,
		UpdatedAt: group.UpdatedAt,
	}, nil
}

func normalizeSessionDeskGroupColor(value string) (string, error) {
	color := strings.TrimSpace(value)
	if color == "" {
		return "", nil
	}
	if len(color) > maxSessionDeskColorBytes || !sessionDeskGroupColorPattern.MatchString(color) {
		return "", storageInvalid("desk group color is invalid", nil)
	}
	return color, nil
}

func validateSessionDeskGroupName(name string) error {
	if name == "" {
		return storageInvalid("desk group name is required", nil)
	}
	if len([]rune(name)) > maxSessionDeskGroupRunes {
		return storageInvalid("desk group name is too long", nil)
	}
	if strings.ContainsAny(name, "/\\\n\r") {
		return storageInvalid("desk group name contains unsupported characters", nil)
	}
	return nil
}

func normalizeSessionDeskChatRef(ref types.SessionDeskChatRef) (types.SessionDeskChatRef, error) {
	kind := strings.TrimSpace(ref.TargetKind)
	if kind != "role" && kind != "group" && kind != "workspace" {
		return types.SessionDeskChatRef{}, storageInvalid("desk chat target kind is invalid", nil)
	}
	targetID, err := cleanID(ref.TargetID)
	if err != nil {
		return types.SessionDeskChatRef{}, storageInvalid("desk chat target id is invalid", err)
	}
	chatID, err := cleanID(ref.ChatID)
	if err != nil {
		return types.SessionDeskChatRef{}, storageInvalid("desk chat id is invalid", err)
	}
	roleID := strings.TrimSpace(ref.RoleID)
	if kind == "workspace" {
		if roleID == "" {
			return types.SessionDeskChatRef{}, storageInvalid("desk chat role id is required", nil)
		}
		if roleID, err = cleanID(roleID); err != nil {
			return types.SessionDeskChatRef{}, storageInvalid("desk chat role id is invalid", err)
		}
	} else if roleID != "" {
		return types.SessionDeskChatRef{}, storageInvalid("desk chat role id is only allowed for workspace", nil)
	}
	if ref.AddedAt <= 0 {
		return types.SessionDeskChatRef{}, storageInvalid("desk chat added timestamp is required", nil)
	}
	return types.SessionDeskChatRef{TargetKind: kind, TargetID: targetID, RoleID: roleID, ChatID: chatID, AddedAt: ref.AddedAt}, nil
}

func sessionDeskChatRefKey(ref types.SessionDeskChatRef) string {
	return fmt.Sprintf("%s::%s::%s::%s", ref.TargetKind, ref.TargetID, ref.RoleID, ref.ChatID)
}
