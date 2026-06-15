package datastorage

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) SaveChatGroup(ctx context.Context, group types.ChatGroup) error {
	group, err := normalizeChatGroupForStorage(group, time.Now().UTC())
	if err != nil {
		return err
	}
	if _, err := cleanID(group.ID); err != nil {
		return err
	}
	target, err := s.paths.groupDataFile(group.ID)
	if err != nil {
		return err
	}
	if err := writeJSON(ctx, target, group); err != nil {
		return err
	}
	groupDir, err := s.paths.groupDir(group.ID)
	if err != nil {
		return err
	}
	if err := ensureDirs(filepath.Join(groupDir, "attachments")); err != nil {
		return storageWriteFailed("failed to create group attachments directory", err)
	}
	return s.rebuildChatGroupIndex(ctx)
}

func (s *system) LoadChatGroup(ctx context.Context, groupID string) (types.ChatGroup, error) {
	target, err := s.paths.groupDataFile(groupID)
	if err != nil {
		return types.ChatGroup{}, err
	}
	group, err := readJSON[types.ChatGroup](ctx, target)
	if err != nil {
		return types.ChatGroup{}, err
	}
	return normalizeChatGroupForStorage(group, time.Now().UTC())
}

func (s *system) ListChatGroups(ctx context.Context) ([]types.ChatGroupSummary, error) {
	groups, err := readObjects[types.ChatGroup](ctx, s.paths.groupsRoot())
	if err != nil {
		return nil, err
	}
	summaries := make([]types.ChatGroupSummary, 0, len(groups))
	for _, group := range groups {
		group, err = normalizeChatGroupForStorage(group, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, types.ChatGroupSummary{ID: group.ID, Name: group.Name, Avatar: group.Avatar, UpdatedAt: group.UpdatedAt})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })
	return summaries, nil
}

func (s *system) DeleteChatGroup(ctx context.Context, groupID string) error {
	dir, err := s.paths.groupDir(groupID)
	if err != nil {
		return err
	}
	if err := moveToRecycle(ctx, s.paths, types.StorageItemGroup, groupID, dir); err != nil {
		return err
	}
	return s.rebuildChatGroupIndex(ctx)
}

func (s *system) rebuildChatGroupIndex(ctx context.Context) error {
	groups, err := s.ListChatGroups(ctx)
	if err != nil {
		return err
	}
	return writeIndex(ctx, filepath.Join(s.paths.groupsRoot(), "index.json"), rootIndex[types.ChatGroupSummary]{Items: groups})
}

func normalizeChatGroupForStorage(group types.ChatGroup, now time.Time) (types.ChatGroup, error) {
	group.ID = strings.TrimSpace(group.ID)
	if _, err := cleanID(group.ID); err != nil {
		return types.ChatGroup{}, err
	}
	group.Name = strings.Join(strings.Fields(group.Name), " ")
	if group.Name == "" {
		group.Name = "未命名群组"
	}
	group.Avatar = strings.TrimSpace(group.Avatar)
	if group.Avatar == "" {
		group.Avatar = "群"
	}
	group.Prompt = strings.TrimSpace(group.Prompt)
	if strings.TrimSpace(group.Mode) == "random" {
		group.Mode = "random"
	} else {
		group.Mode = "roundRobin"
	}
	var err error
	group.MemberRoleIDs, err = cleanUniqueIDs(group.MemberRoleIDs)
	if err != nil {
		return types.ChatGroup{}, err
	}
	group.RoundRobinOrder, err = cleanUniqueIDsInSet(group.RoundRobinOrder, group.MemberRoleIDs)
	if err != nil {
		return types.ChatGroup{}, err
	}
	if len(group.RoundRobinOrder) == 0 {
		group.RoundRobinOrder = append([]string(nil), group.MemberRoleIDs...)
	}
	group.Random, err = normalizeChatGroupRandomConfig(group.Random, group.MemberRoleIDs)
	if err != nil {
		return types.ChatGroup{}, err
	}
	baseline := firstNonZeroTime(group.CreatedAt, group.UpdatedAt, now)
	if group.CreatedAt.IsZero() {
		group.CreatedAt = baseline
	}
	if group.UpdatedAt.IsZero() || group.UpdatedAt.Before(group.CreatedAt) {
		group.UpdatedAt = baseline
	}
	return group, nil
}

func normalizeChatGroupRandomConfig(config types.ChatGroupRandomConfig, memberRoleIDs []string) (types.ChatGroupRandomConfig, error) {
	if config.MinCount <= 0 {
		config.MinCount = 1
	}
	if config.MaxCount <= 0 {
		config.MaxCount = config.MinCount
	}
	if config.MaxCount < config.MinCount {
		config.MaxCount = config.MinCount
	}
	memberSet := map[string]struct{}{}
	for _, roleID := range memberRoleIDs {
		memberSet[roleID] = struct{}{}
	}
	weights := map[string]float64{}
	for roleID, weight := range config.WeightsByRoleID {
		roleID = strings.TrimSpace(roleID)
		if _, ok := memberSet[roleID]; !ok {
			return types.ChatGroupRandomConfig{}, storageInvalid("group random weight references a role outside members", nil)
		}
		if weight < 0 {
			weight = 0
		}
		weights[roleID] = weight
	}
	for _, roleID := range memberRoleIDs {
		if _, ok := weights[roleID]; !ok {
			weights[roleID] = 1
		}
	}
	if len(weights) == 0 {
		weights = nil
	}
	config.WeightsByRoleID = weights
	return config, nil
}

func cleanUniqueIDs(values []string) ([]string, error) {
	result := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		id := strings.TrimSpace(value)
		if id == "" {
			continue
		}
		if _, err := cleanID(id); err != nil {
			return nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func cleanUniqueIDsInSet(values []string, allowed []string) ([]string, error) {
	allowedSet := map[string]struct{}{}
	for _, id := range allowed {
		allowedSet[id] = struct{}{}
	}
	result := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		id := strings.TrimSpace(value)
		if _, ok := allowedSet[id]; !ok {
			return nil, storageInvalid("group order references a role outside members", nil)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}
