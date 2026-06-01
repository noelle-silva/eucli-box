package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	uiVersion          = 7
	splitSchemaVersion = 1
)

var (
	roleKeyPattern        = regexp.MustCompile(`^roles/([^/]+)/role$`)
	providerKeyPattern    = regexp.MustCompile(`^providers/([^/]+)/provider$`)
	chatKeyPattern        = regexp.MustCompile(`^chats/([^/]+)/([^/]+)/chat$`)
	roleChatIndexPattern  = regexp.MustCompile(`^chats/([^/]+)/index$`)
	roleAvatarPathPattern = regexp.MustCompile(`^roles/([^/]+)/avatar\.(png|jpg|jpeg|webp)$`)
)

type projectionService struct {
	config *configStore
	eb     *ebClient
}

func newProjectionService(config *configStore, eb *ebClient) *projectionService {
	return &projectionService{config: config, eb: eb}
}

func (p *projectionService) get(ctx context.Context, key string) (any, error) {
	switch key {
	case "meta/index":
		return p.meta(ctx)
	case "stickers/index":
		return map[string]any{}, nil
	case "chats/index":
		return p.chatsIndex(ctx)
	case "providers/index":
		return p.providersIndex(ctx)
	case "groups/index":
		return map[string]any{"groupOrder": []any{}, "groupFolders": map[string]any{}, "updatedAt": nowMillis()}, nil
	}
	if match := roleKeyPattern.FindStringSubmatch(key); match != nil {
		return p.roleByFolder(ctx, match[1])
	}
	if match := providerKeyPattern.FindStringSubmatch(key); match != nil {
		return p.providerByFolder(ctx, match[1])
	}
	if match := roleChatIndexPattern.FindStringSubmatch(key); match != nil {
		return p.roleChatIndex(ctx, match[1])
	}
	if match := chatKeyPattern.FindStringSubmatch(key); match != nil {
		return p.chatByFolder(ctx, match[1], match[2])
	}
	cfg, err := p.config.load()
	if err != nil {
		return nil, err
	}
	if value, ok := cfg.Projection.ClientObjects[key]; ok {
		return value, nil
	}
	return nil, nil
}

func (p *projectionService) set(ctx context.Context, key string, value any) error {
	if match := roleKeyPattern.FindStringSubmatch(key); match != nil {
		return p.saveRole(ctx, match[1], value)
	}
	if match := providerKeyPattern.FindStringSubmatch(key); match != nil {
		return p.saveProvider(ctx, match[1], value)
	}
	if match := chatKeyPattern.FindStringSubmatch(key); match != nil {
		return p.saveSession(ctx, match[1], value)
	}
	if key == "providers/index" {
		return nil
	}
	_, err := p.config.updateProjection(func(projection *projectionConfig) {
		projection.ClientObjects[key] = value
		if key == "meta/index" {
			if meta, ok := value.(map[string]any); ok {
				projection.UI = objectMap(meta["ui"])
				projection.Settings = objectMap(meta["settings"])
				projection.Favorites = objectMap(meta["favorites"])
			}
		}
	})
	return err
}

func (p *projectionService) remove(ctx context.Context, key string) error {
	if match := roleKeyPattern.FindStringSubmatch(key); match != nil {
		roleID, err := p.roleIDByFolder(ctx, match[1])
		if isNotFoundError(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := p.eb.request(ctx, ebRequest{Method: "DELETE", Path: fmt.Sprintf("/api/roles/%s", roleID)}); err != nil {
			return err
		}
		_, err = p.config.updateProjection(func(projection *projectionConfig) {
			delete(projection.RoleFolders, roleID)
			delete(projection.ActiveChatByRole, roleID)
		})
		return err
	}
	if match := providerKeyPattern.FindStringSubmatch(key); match != nil {
		providerID, err := p.providerIDByFolder(ctx, match[1])
		if err != nil {
			return err
		}
		if _, err := p.eb.request(ctx, ebRequest{Method: "DELETE", Path: fmt.Sprintf("/api/providers/%s", providerID)}); err != nil {
			return err
		}
		_, err = p.config.updateProjection(func(projection *projectionConfig) {
			delete(projection.ProviderFolders, providerID)
		})
		return err
	}
	if match := chatKeyPattern.FindStringSubmatch(key); match != nil {
		roleID, err := p.roleIDByFolder(ctx, match[1])
		if err != nil {
			return err
		}
		_, err = p.eb.request(ctx, ebRequest{Method: "DELETE", Path: fmt.Sprintf("/api/roles/%s/sessions/%s", roleID, match[2])})
		return err
	}
	_, err := p.config.updateProjection(func(projection *projectionConfig) {
		delete(projection.ClientObjects, key)
	})
	return err
}

func (p *projectionService) meta(ctx context.Context) (any, error) {
	roles, err := p.listRoles(ctx)
	if err != nil {
		return nil, err
	}
	providers, err := p.listProviders(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := p.config.load()
	if err != nil {
		return nil, err
	}
	projection := cfg.Projection
	roleOrder := []string{}
	roleFolders := map[string]string{}
	chatIndexByRole := map[string]any{}
	for _, role := range roles {
		id := stringField(role, "id")
		if id == "" {
			continue
		}
		folder := folderFor(projection.RoleFolders, id, stringField(role, "name"), "角色")
		roleOrder = append(roleOrder, id)
		roleFolders[id] = folder
		index, _ := p.sessionsIndexForRole(ctx, id)
		chatIndexByRole[id] = index
	}
	providerOrder := []string{}
	providerFolders := map[string]string{}
	for _, provider := range providers {
		id := stringField(provider, "id")
		if id == "" {
			continue
		}
		providerOrder = append(providerOrder, id)
		providerFolders[id] = folderFor(projection.ProviderFolders, id, stringField(provider, "name"), "供应商")
	}
	return map[string]any{
		"schemaVersion":    splitSchemaVersion,
		"dataVersion":      uiVersion,
		"updatedAt":        nowMillis(),
		"ui":               projection.UI,
		"settings":         mergeSettings(projection.Settings, providers),
		"favorites":        projection.Favorites,
		"roleOrder":        roleOrder,
		"roleFolders":      roleFolders,
		"chatIndexByRole":  chatIndexByRole,
		"groupOrder":       []any{},
		"groupFolders":     map[string]any{},
		"chatIndexByGroup": map[string]any{},
		"providerOrder":    providerOrder,
		"providerFolders":  providerFolders,
	}, nil
}

func (p *projectionService) chatsIndex(ctx context.Context) (any, error) {
	meta, err := p.meta(ctx)
	if err != nil {
		return nil, err
	}
	m := meta.(map[string]any)
	return map[string]any{"roleOrder": m["roleOrder"], "roleFolders": m["roleFolders"], "updatedAt": nowMillis()}, nil
}

func (p *projectionService) providersIndex(ctx context.Context) (any, error) {
	meta, err := p.meta(ctx)
	if err != nil {
		return nil, err
	}
	m := meta.(map[string]any)
	return map[string]any{"providerOrder": m["providerOrder"], "providerFolders": m["providerFolders"], "updatedAt": nowMillis()}, nil
}

func (p *projectionService) roleChatIndex(ctx context.Context, folder string) (any, error) {
	roleID, err := p.roleIDByFolder(ctx, folder)
	if err != nil {
		return nil, err
	}
	return p.sessionsIndexForRole(ctx, roleID)
}

func (p *projectionService) sessionsIndexForRole(ctx context.Context, roleID string) (any, error) {
	sessions, err := p.listSessions(ctx, roleID)
	if err != nil {
		return map[string]any{"activeChatId": "", "chatIds": []any{}, "chatUpdatedAt": map[string]any{}, "chatMetas": []any{}, "updatedAt": nowMillis()}, err
	}
	chatIds := []string{}
	chatUpdatedAt := map[string]any{}
	chatMetas := []any{}
	for _, session := range sessions {
		id := stringField(session, "id")
		if id == "" {
			continue
		}
		updatedAt := millisFromAny(session["lastActive"])
		chatIds = append(chatIds, id)
		chatUpdatedAt[id] = updatedAt
		chatMetas = append(chatMetas, map[string]any{"id": id, "title": fallback(stringField(session, "title"), "新聊天"), "updatedAt": updatedAt, "createdAt": updatedAt})
	}
	cfg, _ := p.config.load()
	active := cfg.Projection.ActiveChatByRole[roleID]
	if active == "" && len(chatIds) > 0 {
		active = chatIds[0]
	}
	return map[string]any{"activeChatId": active, "chatIds": chatIds, "chatUpdatedAt": chatUpdatedAt, "chatMetas": chatMetas, "updatedAt": nowMillis()}, nil
}

func (p *projectionService) roleByFolder(ctx context.Context, folder string) (any, error) {
	roles, err := p.listRoles(ctx)
	if err != nil {
		return nil, err
	}
	cfg, _ := p.config.load()
	for _, role := range roles {
		id := stringField(role, "id")
		if folderFor(cfg.Projection.RoleFolders, id, stringField(role, "name"), "角色") == folder {
			uiRole := toUIRole(role)
			if avatarImage, err := p.loadRoleAvatar(ctx, id); err == nil {
				uiRole["avatarImage"] = avatarImage
			}
			return uiRole, nil
		}
	}
	return nil, newError("NOT_FOUND", "角色不存在")
}

func (p *projectionService) roleIDByAvatarPath(ctx context.Context, path string) (string, error) {
	match := roleAvatarPathPattern.FindStringSubmatch(strings.TrimSpace(path))
	if match == nil {
		return "", newError("NOT_IMPLEMENTED", "仅支持角色头像图片路径")
	}
	return p.roleIDByFolder(ctx, match[1])
}

func (p *projectionService) providerByFolder(ctx context.Context, folder string) (any, error) {
	providers, err := p.listProviders(ctx)
	if err != nil {
		return nil, err
	}
	cfg, _ := p.config.load()
	for _, provider := range providers {
		id := stringField(provider, "id")
		if folderFor(cfg.Projection.ProviderFolders, id, stringField(provider, "name"), "供应商") == folder {
			return toUIProvider(provider), nil
		}
	}
	return nil, newError("NOT_FOUND", "供应商不存在")
}

func (p *projectionService) chatByFolder(ctx context.Context, folder string, chatID string) (any, error) {
	roleID, err := p.roleIDByFolder(ctx, folder)
	if err != nil {
		return nil, err
	}
	session, err := p.loadSession(ctx, roleID, chatID)
	if err != nil {
		return nil, err
	}
	return toUIChat(session), nil
}

func (p *projectionService) saveRole(ctx context.Context, folder string, value any) error {
	role := fromUIRole(value)
	roleID := stringField(role, "id")
	if roleID == "" {
		return newError("BAD_REQUEST", "role id is required")
	}
	if _, err := p.eb.request(ctx, ebRequest{Method: "POST", Path: "/api/roles", Body: mustJSON(role)}); err != nil {
		return err
	}
	_, err := p.config.updateProjection(func(projection *projectionConfig) {
		projection.RoleFolders[roleID] = folder
	})
	return err
}

func (p *projectionService) saveProvider(ctx context.Context, folder string, value any) error {
	provider := fromUIProvider(value)
	providerID := stringField(provider, "id")
	if providerID == "" {
		return newError("BAD_REQUEST", "provider id is required")
	}
	if _, err := p.eb.request(ctx, ebRequest{Method: "POST", Path: "/api/providers", Body: mustJSON(provider)}); err != nil {
		return err
	}
	_, err := p.config.updateProjection(func(projection *projectionConfig) {
		projection.ProviderFolders[providerID] = folder
	})
	return err
}

func (p *projectionService) saveSession(ctx context.Context, folder string, value any) error {
	roleID, err := p.roleIDByFolder(ctx, folder)
	if err != nil {
		return err
	}
	session := fromUIChat(value, roleID)
	_, err = p.eb.request(ctx, ebRequest{Method: "POST", Path: fmt.Sprintf("/api/roles/%s/sessions", roleID), Body: mustJSON(session)})
	return err
}

func (p *projectionService) listRoles(ctx context.Context) ([]map[string]any, error) {
	data, err := p.eb.request(ctx, ebRequest{Method: "GET", Path: "/api/roles"})
	if err != nil {
		return nil, err
	}
	summaries := objectList(data)
	roles := make([]map[string]any, 0, len(summaries))
	for _, summary := range summaries {
		id := stringField(summary, "id")
		if id == "" {
			continue
		}
		detail, err := p.eb.request(ctx, ebRequest{Method: "GET", Path: fmt.Sprintf("/api/roles/%s", id)})
		if err != nil {
			return nil, err
		}
		roles = append(roles, objectMap(detail))
	}
	return roles, nil
}

func (p *projectionService) listProviders(ctx context.Context) ([]map[string]any, error) {
	data, err := p.eb.request(ctx, ebRequest{Method: "GET", Path: "/api/providers"})
	if err != nil {
		return nil, err
	}
	summaries := objectList(data)
	providers := make([]map[string]any, 0, len(summaries))
	for _, summary := range summaries {
		id := stringField(summary, "id")
		if id == "" {
			continue
		}
		detail, err := p.eb.request(ctx, ebRequest{Method: "GET", Path: fmt.Sprintf("/api/providers/%s", id)})
		if err != nil {
			return nil, err
		}
		providers = append(providers, objectMap(detail))
	}
	return providers, nil
}

func (p *projectionService) listSessions(ctx context.Context, roleID string) ([]map[string]any, error) {
	data, err := p.eb.request(ctx, ebRequest{Method: "GET", Path: fmt.Sprintf("/api/roles/%s/sessions", roleID)})
	return objectList(data), err
}

func (p *projectionService) loadSession(ctx context.Context, roleID string, sessionID string) (map[string]any, error) {
	data, err := p.eb.request(ctx, ebRequest{Method: "GET", Path: fmt.Sprintf("/api/roles/%s/sessions/%s", roleID, sessionID)})
	if err != nil {
		return nil, err
	}
	return objectMap(data), nil
}

func (p *projectionService) loadRoleAvatar(ctx context.Context, roleID string) (string, error) {
	data, err := p.eb.request(ctx, ebRequest{Method: "GET", Path: fmt.Sprintf("/api/roles/%s/avatar", roleID)})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(fmt.Sprint(data)), nil
}

func (p *projectionService) roleIDByFolder(ctx context.Context, folder string) (string, error) {
	roles, err := p.listRoles(ctx)
	if err != nil {
		return "", err
	}
	cfg, _ := p.config.load()
	for _, role := range roles {
		id := stringField(role, "id")
		if folderFor(cfg.Projection.RoleFolders, id, stringField(role, "name"), "角色") == folder {
			return id, nil
		}
	}
	return "", newError("NOT_FOUND", "角色不存在")
}

func (p *projectionService) providerIDByFolder(ctx context.Context, folder string) (string, error) {
	providers, err := p.listProviders(ctx)
	if err != nil {
		return "", err
	}
	cfg, _ := p.config.load()
	for _, provider := range providers {
		id := stringField(provider, "id")
		if folderFor(cfg.Projection.ProviderFolders, id, stringField(provider, "name"), "供应商") == folder {
			return id, nil
		}
	}
	return "", newError("NOT_FOUND", "供应商不存在")
}

func toUIRole(role map[string]any) map[string]any {
	modelConfig := objectMap(role["modelConfig"])
	coordinate := objectMap(modelConfig["coordinate"])
	return map[string]any{
		"id":           stringField(role, "id"),
		"name":         fallback(stringField(role, "name"), "未命名角色"),
		"avatar":       stringField(role, "avatar"),
		"systemPrompt": promptText(objectList(role["prompts"])),
		"temperature":  numberField(modelConfig, "temperature", 0.7),
		"modelRef":     map[string]any{"providerId": stringField(coordinate, "providerId"), "modelId": stringField(coordinate, "modelId")},
		"createdAt":    millisFromAny(role["createdAt"]),
		"updatedAt":    millisFromAny(role["updatedAt"]),
	}
}

func fromUIRole(value any) map[string]any {
	role := objectMap(value)
	modelRef := objectMap(role["modelRef"])
	now := time.Now().UTC().Format(time.RFC3339)
	return map[string]any{
		"id":          stringField(role, "id"),
		"name":        fallback(stringField(role, "name"), "未命名角色"),
		"avatar":      stringField(role, "avatar"),
		"description": stringField(role, "description"),
		"prompts":     []any{map[string]any{"id": "system", "role": "system", "content": stringField(role, "systemPrompt"), "order": 0, "createdAt": now, "updatedAt": now}},
		"modelConfig": map[string]any{"coordinate": map[string]any{"providerId": stringField(modelRef, "providerId"), "modelId": stringField(modelRef, "modelId")}, "temperature": numberField(role, "temperature", 0.7)},
		"toolPolicy":  map[string]any{"mode": "whitelist", "tools": []any{}},
		"createdAt":   timeFromMillis(role["createdAt"]),
		"updatedAt":   time.Now().UTC().Format(time.RFC3339),
	}
}

func toUIProvider(provider map[string]any) map[string]any {
	models := []any{}
	for _, model := range objectList(provider["models"]) {
		id := stringField(model, "id")
		if id != "" {
			models = append(models, id)
		}
	}
	return map[string]any{"id": stringField(provider, "id"), "name": stringField(provider, "name"), "baseUrl": stringField(provider, "baseUrl"), "apiKey": stringField(provider, "key"), "protocol": stringField(provider, "protocol"), "modelsCache": map[string]any{"items": models, "fetchedAt": millisFromAny(provider["updatedAt"])}}
}

func fromUIProvider(value any) map[string]any {
	provider := objectMap(value)
	return map[string]any{"id": stringField(provider, "id"), "name": stringField(provider, "name"), "baseUrl": stringField(provider, "baseUrl"), "key": stringField(provider, "apiKey"), "protocol": stringField(provider, "protocol")}
}

func toUIChat(session map[string]any) map[string]any {
	messages := []any{}
	for _, msg := range objectList(session["messages"]) {
		messages = append(messages, map[string]any{"id": stringField(msg, "id"), "role": messageRole(msg), "content": stringField(msg, "content"), "createdAt": millisFromAny(msg["createdAt"])})
	}
	createdAt := millisFromAny(session["createdAt"])
	updatedAt := millisFromAny(session["lastActive"])
	return map[string]any{"id": stringField(session, "id"), "title": fallback(stringField(session, "title"), "新聊天"), "createdAt": createdAt, "updatedAt": updatedAt, "messages": messages}
}

func fromUIChat(value any, roleID string) map[string]any {
	chat := objectMap(value)
	messages := []any{}
	for _, msg := range objectList(chat["messages"]) {
		messages = append(messages, map[string]any{"id": stringField(msg, "id"), "type": messageRole(msg), "content": stringField(msg, "content"), "createdAt": timeFromMillis(msg["createdAt"])})
	}
	return map[string]any{"id": stringField(chat, "id"), "roleId": roleID, "title": fallback(stringField(chat, "title"), "新聊天"), "status": "created", "messages": messages, "createdAt": timeFromMillis(chat["createdAt"]), "lastActive": timeFromMillis(chat["updatedAt"])}
}

func mergeSettings(settings map[string]any, providers []map[string]any) map[string]any {
	out := map[string]any{"streamEnabled": true, "transparentChatBg": false, "chatBgOpacity": 0, "chatBgBlur": 0, "topbarOpacity": 100, "topbarBlur": 0, "composerOpacity": 86, "composerBlur": 10, "branchTree": map[string]any{"dir": "lr", "view": "float", "followSelected": true, "modalHotkey": ""}, "renderSafetyPolicy": "original", "userMessageCollapseEnabled": false, "userMessageCollapseLines": 8, "attachments": map[string]any{"sendLimitChars": 80000, "maxFileSizeMbByKind": map[string]any{"txt": 10, "md": 10, "pdf": 10, "docx": 10, "ppt": 10}}, "stickers": map[string]any{"enabled": false, "categories": []any{}, "map": map[string]any{}}, "providers": []any{}}
	for k, v := range settings {
		out[k] = v
	}
	uiProviders := []any{}
	for _, provider := range providers {
		uiProviders = append(uiProviders, toUIProvider(provider))
	}
	out["providers"] = uiProviders
	return out
}

func objectMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func objectList(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]map[string]any); ok {
			return typed
		}
		return nil
	}
	out := []map[string]any{}
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func stringField(m map[string]any, key string) string {
	if m == nil || m[key] == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(m[key]))
}
func numberField(m map[string]any, key string, fallback float64) float64 {
	if n, ok := m[key].(float64); ok {
		return n
	}
	return fallback
}
func fallback(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
func nowMillis() int64 { return time.Now().UnixMilli() }
func millisFromAny(value any) int64 {
	if s, ok := value.(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UnixMilli()
		}
	}
	if n, ok := value.(float64); ok {
		return int64(n)
	}
	return nowMillis()
}
func timeFromMillis(value any) string {
	ms := millisFromAny(value)
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}
func mustJSON(value any) json.RawMessage { payload, _ := json.Marshal(value); return payload }
func folderFor(existing map[string]string, id string, name string, fallbackName string) string {
	if existing[id] != "" {
		return existing[id]
	}
	return safeFolderName(fallback(name, fallbackName))
}
func safeFolderName(value string) string {
	return strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_").Replace(value)
}
func promptText(prompts []map[string]any) string {
	for _, prompt := range prompts {
		if stringField(prompt, "role") == "system" {
			return stringField(prompt, "content")
		}
	}
	return ""
}
func messageRole(message map[string]any) string {
	role := stringField(message, "role")
	if role == "" {
		role = stringField(message, "type")
	}
	if role == "assistant" {
		return "assistant"
	}
	return "user"
}

func isNotFoundError(err error) bool { return err != nil && strings.Contains(err.Error(), "不存在") }
