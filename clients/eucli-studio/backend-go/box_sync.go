package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const boxSchemaVersion = 1
const boxDataVersion = 7

func (svc *service) syncBoxCatalog(ctx context.Context) error {
	roles, err := svc.box.listRoles(ctx)
	if err != nil {
		return fmt.Errorf("sync eucli-box roles: %w", err)
	}
	providers, err := svc.box.listProviders(ctx)
	if err != nil {
		return fmt.Errorf("sync eucli-box providers: %w", err)
	}

	var errs []string

	existing, loadErr := svc.loadObject(splitMetaKey)
	var meta map[string]any
	if loadErr == nil && existing != nil {
		meta = existing
	} else if recovered := svc.loadRawMetaAll(); recovered != nil {
		meta = recovered
	} else {
		meta = map[string]any{
			"schemaVersion": boxSchemaVersion,
			"dataVersion":   boxDataVersion,
			"updatedAt":     nowMs(),
		}
	}
	if asMap(meta["ui"]) == nil {
		meta["ui"] = map[string]any{}
	}
	if asMap(meta["settings"]) == nil {
		meta["settings"] = map[string]any{}
	}
	if asMap(meta["favorites"]) == nil {
		meta["favorites"] = map[string]any{"folders": []any{}, "chatRefsByFolderId": map[string]any{}}
	}

	roleOrder := make([]string, 0, len(roles))
	roleFolders := map[string]string{}
	providerOrder := make([]string, 0, len(providers))
	providerFolders := map[string]string{}

	for _, role := range roles {
		roleID := strings.TrimSpace(asString(role["id"]))
		if roleID == "" {
			continue
		}
		folder := asString(roleFolders[roleID])
		if folder == "" {
			folder = boxSafeDirName(asString(role["name"]), roleID)
			roleFolders[roleID] = folder
		}
		name := strings.TrimSpace(asString(role["name"]))
		avatar := strings.TrimSpace(asString(role["avatar"]))
		description := strings.TrimSpace(asString(role["description"]))
		systemPrompt := strings.TrimSpace(boxFirstPromptText(role))
		modelConfig := asMap(role["modelConfig"])
		temperature := asFloat64(modelConfig["temperature"], 0.7)

		clientRole := map[string]any{
			"id":           roleID,
			"name":         name,
			"avatar":       avatar,
			"description":  description,
			"systemPrompt": systemPrompt,
			"temperature":  temperature,
			"modelRef":     boxModelRef(role),
			"createdAt":    boxTimeOrDefault(role, "createdAt"),
			"updatedAt":    boxTimeOrDefault(role, "updatedAt"),
		}
		if err := svc.storageSetByKey(splitRoleKeyGo(folder), clientRole); err != nil {
			errs = append(errs, fmt.Sprintf("write role %s: %v", roleID, err))
			continue
		}
		if _, err := svc.ensureChatIndexForRoleFolder(roleID, folder); err != nil {
			errs = append(errs, fmt.Sprintf("ensure chat index for role %s: %v", roleID, err))
			continue
		}
		if !containsString(roleOrder, roleID) {
			roleOrder = append(roleOrder, roleID)
		}
	}

	for _, provider := range providers {
		providerID := strings.TrimSpace(asString(provider["id"]))
		if providerID == "" {
			continue
		}
		folder := asString(providerFolders[providerID])
		if folder == "" {
			folder = boxSafeDirName(asString(provider["name"]), providerID)
			providerFolders[providerID] = folder
		}
		clientProvider := map[string]any{
			"id":       providerID,
			"name":     strings.TrimSpace(asString(provider["name"])),
			"baseUrl":  strings.TrimSpace(asString(provider["baseUrl"])),
			"apiKey":   "",
			"protocol": strings.TrimSpace(asString(provider["protocol"])),
			"modelsCache": map[string]any{
				"items":     boxModels(provider["models"]),
				"fetchedAt": nowMs(),
			},
			"createdAt": boxTimeOrDefault(provider, "createdAt"),
			"updatedAt": boxTimeOrDefault(provider, "updatedAt"),
		}
		if err := svc.storageSetByKey(splitProviderKeyGo(folder), clientProvider); err != nil {
			errs = append(errs, fmt.Sprintf("write provider %s: %v", providerID, err))
			continue
		}
		if !containsString(providerOrder, providerID) {
			providerOrder = append(providerOrder, providerID)
		}
	}

	now := nowMs()
	meta["roleOrder"] = roleOrder
	meta["roleFolders"] = roleFolders
	meta["updatedAt"] = now
	meta["catalogSyncedAt"] = now
	meta["catalogVersion"] = asInt64(meta["catalogVersion"], 0) + 1
	if err := svc.storageSetByKey(splitMetaKey, meta); err != nil {
		return fmt.Errorf("write meta/index: %w", err)
	}

	chatsIndex := map[string]any{
		"schemaVersion": boxSchemaVersion,
		"roleOrder":     roleOrder,
		"roleFolders":   roleFolders,
		"updatedAt":     now,
	}
	if err := svc.storageSetByKey(splitChatsIndexKeyGo(), chatsIndex); err != nil {
		return fmt.Errorf("write chats/index: %w", err)
	}

	providersIndex := map[string]any{
		"schemaVersion":  boxSchemaVersion,
		"updatedAt":      now,
		"providerOrder":  providerOrder,
		"providerFolders": providerFolders,
	}
	if err := svc.storageSetByKey(splitProvidersIndexKeyGo(), providersIndex); err != nil {
		return fmt.Errorf("write providers/index: %w", err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("部分同步失败: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (svc *service) ensureChatIndexForRoleFolder(roleID string, folder string) (map[string]any, error) {
	existing, _ := svc.loadObject(splitRoleChatIndexKeyGo(folder))
	if existing != nil {
		return existing, nil
	}
	index := map[string]any{
		"schemaVersion": boxSchemaVersion,
		"roleId":        roleID,
		"roleFolder":    folder,
		"activeChatId":  "",
		"chatIds":       []any{},
		"chatUpdatedAt": map[string]any{},
		"chatMetas":     []any{},
		"updatedAt":     nowMs(),
	}
	return index, svc.storageSetByKey(splitRoleChatIndexKeyGo(folder), index)
}

func boxFirstPromptText(role map[string]any) string {
	prompts := asSlice(role["prompts"])
	if len(prompts) == 0 {
		return strings.TrimSpace(asString(role["description"]))
	}
	var texts []string
	for _, p := range prompts {
		prompt := asMap(p)
		text := strings.TrimSpace(asString(prompt["content"]))
		if text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

func boxModelRef(role map[string]any) map[string]any {
	modelConfig := asMap(role["modelConfig"])
	return map[string]any{
		"providerId": strings.TrimSpace(asString(modelConfig["providerId"])),
		"modelId":    strings.TrimSpace(asString(modelConfig["model"])),
	}
}

func boxModels(raw any) []map[string]any {
	list := asSlice(raw)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		model := asMap(item)
		out = append(out, map[string]any{
			"id":   strings.TrimSpace(asString(model["id"])),
			"name": strings.TrimSpace(asString(model["name"])),
		})
	}
	return out
}

func boxTimeOrDefault(obj map[string]any, key string) int64 {
	if raw, ok := obj[key]; ok {
		if t, err := parseTime(raw); err == nil && t > 0 {
			return t
		}
	}
	return nowMs()
}

func parseTime(raw any) (int64, error) {
	switch v := raw.(type) {
	case float64:
		if v < 0 {
			return 0, errors.New("negative timestamp")
		}
		return int64(v), nil
	case string:
		t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(v))
		if err != nil {
			return 0, err
		}
		return t.UnixMilli(), nil
	default:
		return 0, fmt.Errorf("unsupported time type %T", raw)
	}
}

func containsString(items []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func boxSafeDirName(name string, id string) string {
	const fallback = "未命名"
	raw := strings.TrimSpace(name)
	if raw == "" {
		raw = strings.TrimSpace(id)
	}
	if raw == "" {
		raw = fallback
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r == ' ':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	value := strings.TrimSpace(b.String())
	if value == "" {
		value = fallback
	}
	if len(value) > 60 {
		value = strings.TrimSpace(value[:60])
	}
	if value == "." || value == ".." {
		value = "_" + value
	}
	return value
}

func (svc *service) loadRawMetaAll() map[string]any {
	path, err := svc.storagePathForKey(splitMetaKey)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return nil
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil {
		return nil
	}
	if rawValue, ok := root["value"]; ok {
		var inner map[string]json.RawMessage
		if json.Unmarshal(rawValue, &inner) != nil {
			return nil
		}
		root = inner
	}
	out := map[string]any{}
	for key, rawField := range root {
		var val any
		if json.Unmarshal(rawField, &val) == nil {
			out[key] = val
		}
	}
	return out
}

func (svc *service) loadRawMetaFields(keys ...string) map[string]any {
	all := svc.loadRawMetaAll()
	if all == nil {
		return nil
	}
	out := map[string]any{}
	for _, key := range keys {
		if v, ok := all[key]; ok {
			out[key] = v
		}
	}
	return out
}


