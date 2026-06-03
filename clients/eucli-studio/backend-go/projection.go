package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	uiVersion           = 7
	splitSchemaVersion  = 1
	sessionFavoritesKey = "sessions/favorites"
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
		return p.stickers(ctx)
	case "chats/index":
		return p.chatsIndex(ctx)
	case "providers/index":
		return p.providersIndex(ctx)
	case "groups/index":
		return map[string]any{"groupOrder": []any{}, "groupFolders": map[string]any{}, "updatedAt": nowMillis()}, nil
	case sessionFavoritesKey:
		return p.loadFavorites(ctx)
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
	if match := roleChatIndexPattern.FindStringSubmatch(key); match != nil {
		return p.saveRoleChatIndex(ctx, match[1], value)
	}
	if key == "meta/index" {
		return p.saveMeta(ctx, value)
	}
	if key == sessionFavoritesKey {
		return p.saveFavorites(ctx, objectMap(value))
	}
	if key == "chats/index" {
		return p.saveChatsIndex(value)
	}
	if key == "providers/index" {
		return nil
	}
	if key == "stickers/index" {
		return p.saveStickers(value)
	}
	if key == "groups/index" {
		return nil
	}
	if strings.HasPrefix(key, "groups/") || strings.HasPrefix(key, "runtime/") {
		return newError("NOT_IMPLEMENTED", "storage key 未接入 e-b 根动作："+key)
	}
	return newError("NOT_IMPLEMENTED", "未知 storage key："+key)
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
	if key == sessionFavoritesKey {
		return p.saveFavorites(ctx, map[string]any{"folders": []any{}, "chatRefsByFolderId": map[string]any{}})
	}
	if key == "meta/index" || key == "chats/index" || key == "providers/index" || key == "groups/index" || strings.HasPrefix(key, "groups/") || strings.HasPrefix(key, "runtime/") {
		return newError("NOT_IMPLEMENTED", "storage key 未接入 e-b 根动作："+key)
	}
	if key == "stickers/index" {
		return p.saveStickers(map[string]any{})
	}
	return newError("NOT_IMPLEMENTED", "未知 storage key："+key)
}

func (p *projectionService) saveMeta(ctx context.Context, value any) error {
	meta := objectMap(value)
	settings := objectMap(meta["settings"])
	if err := p.saveAssistConfigs(ctx, settings); err != nil {
		return err
	}
	settings = stripAssistSettings(settings)
	_, err := p.config.updateProjection(func(projection *projectionConfig) {
		projection.UI = objectMap(meta["ui"])
		projection.Settings = settings
	})
	return err
}

func (p *projectionService) loadFavorites(ctx context.Context) (map[string]any, error) {
	data, err := p.eb.request(ctx, ebRequest{Method: "GET", Path: "/api/sessions/favorites"})
	if err != nil {
		return nil, err
	}
	favorites, ok := data.(map[string]any)
	if !ok {
		return nil, newError("BAD_RESPONSE", "favorites response must be an object")
	}
	return favorites, nil
}

func (p *projectionService) saveFavorites(ctx context.Context, favorites map[string]any) error {
	_, err := p.eb.request(ctx, ebRequest{Method: "PUT", Path: "/api/sessions/favorites", Body: mustJSON(favorites)})
	return err
}

func (p *projectionService) saveAssistConfigs(ctx context.Context, settings map[string]any) error {
	services := objectMap(settings["aiServices"])
	requests := []struct {
		config map[string]any
		path   string
	}{
		{config: objectMap(services["stickerNaming"]), path: "/api/assist/stickers/name/config"},
		{config: objectMap(services["mermaidFix"]), path: "/api/assist/mermaid-fix/config"},
		{config: objectMap(services["chatTitleNaming"]), path: "/api/assist/chat-title/config"},
	}
	for _, req := range requests {
		if len(req.config) == 0 {
			continue
		}
		providerID := stringField(req.config, "providerId")
		modelPick := stringField(req.config, "modelId")
		customModelID := stringField(req.config, "customModelId")
		modelID := modelPick
		if modelPick == "__custom__" {
			modelID = customModelID
		}
		if _, err := p.eb.request(ctx, ebRequest{Method: "PUT", Path: req.path, Body: mustJSON(map[string]any{"enabled": boolField(req.config, "enabled", false), "modelPick": modelPick, "customModelId": customModelID, "coordinate": map[string]any{"providerId": providerID, "modelId": modelID}, "systemPrompt": stringField(req.config, "systemPrompt"), "temperature": 0.2})}); err != nil {
			return err
		}
	}
	return nil
}

func stripAssistSettings(settings map[string]any) map[string]any {
	settings = objectMap(settings)
	services := objectMap(settings["aiServices"])
	if len(services) == 0 {
		return settings
	}
	delete(services, "stickerNaming")
	delete(services, "mermaidFix")
	delete(services, "chatTitleNaming")
	settings["aiServices"] = services
	return settings
}

func (p *projectionService) saveChatsIndex(value any) error {
	index := objectMap(value)
	roleOrder := stringSlice(index["roleOrder"])
	_, err := p.config.updateProjection(func(projection *projectionConfig) {
		projection.RoleOrder = roleOrder
	})
	return err
}

func (p *projectionService) saveStickers(value any) error {
	stickers := objectMap(value)
	_, err := p.config.updateProjection(func(projection *projectionConfig) {
		settings := objectMap(projection.Settings)
		settings["stickers"] = map[string]any{"enabled": boolField(stickers, "enabled", false)}
		projection.Settings = settings
	})
	return err
}

func (p *projectionService) stickers(ctx context.Context) (any, error) {
	library, err := p.eb.request(ctx, ebRequest{Method: "GET", Path: "/api/stickers"})
	if err != nil {
		return nil, err
	}
	cfg, err := p.config.load()
	if err != nil {
		return nil, err
	}
	enabled := boolField(objectMap(cfg.Projection.Settings["stickers"]), "enabled", false)
	libraryMap := objectMap(library)
	categories := []string{}
	stickerMap := map[string]any{}
	for _, summary := range objectList(libraryMap["categories"]) {
		name := stringField(summary, "name")
		if name == "" {
			continue
		}
		categories = append(categories, name)
		stickerMap[name] = map[string]any{}
	}
	itemsByCategory := objectMap(libraryMap["map"])
	for _, category := range categories {
		box := map[string]any{}
		for _, item := range objectList(itemsByCategory[category]) {
			name := stringField(item, "name")
			relPath := stringField(item, "relPath")
			if name == "" || relPath == "" {
				continue
			}
			createdAt := millisFromAny(item["createdAt"])
			updatedAt := millisFromAnyOrZero(item["updatedAt"])
			if updatedAt == 0 {
				updatedAt = createdAt
			}
			box[name] = map[string]any{"relPath": relPath, "createdAt": createdAt, "updatedAt": updatedAt}
		}
		stickerMap[category] = box
	}
	return map[string]any{"enabled": enabled, "categories": categories, "map": stickerMap, "updatedAt": millisFromAny(libraryMap["updatedAt"])}, nil
}

func (p *projectionService) loadAssistConfig(ctx context.Context, path string) (map[string]any, error) {
	data, err := p.eb.request(ctx, ebRequest{Method: "GET", Path: path})
	if err != nil {
		return nil, err
	}
	config := objectMap(data)
	modelID := stringField(objectMap(config["coordinate"]), "modelId")
	modelPick := stringField(config, "modelPick")
	customModelID := stringField(config, "customModelId")
	if modelPick == "" {
		modelPick = modelID
	}
	if modelPick == "__custom__" && customModelID == "" {
		customModelID = modelID
	}
	return map[string]any{
		"enabled":       boolField(config, "enabled", false),
		"providerId":    stringField(objectMap(config["coordinate"]), "providerId"),
		"modelId":       modelPick,
		"customModelId": customModelID,
		"systemPrompt":  stringField(config, "systemPrompt"),
	}, nil
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
	mermaidFix, err := p.loadAssistConfig(ctx, "/api/assist/mermaid-fix/config")
	if err != nil {
		return nil, err
	}
	chatTitleNaming, err := p.loadAssistConfig(ctx, "/api/assist/chat-title/config")
	if err != nil {
		return nil, err
	}
	stickerNaming, err := p.loadAssistConfig(ctx, "/api/assist/stickers/name/config")
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
	roleByID := map[string]map[string]any{}
	for _, role := range roles {
		id := stringField(role, "id")
		if id == "" {
			continue
		}
		roleByID[id] = role
	}
	for _, id := range orderedIDs(projection.RoleOrder, mapKeys(roleByID)) {
		role := roleByID[id]
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
		"settings":         mergeSettings(projection.Settings, providers, mermaidFix, chatTitleNaming, stickerNaming),
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
	if active != "" {
		found := false
		for _, id := range chatIds {
			if id == active {
				found = true
				break
			}
		}
		if !found {
			active = ""
		}
	}
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

func (p *projectionService) saveRoleChatIndex(ctx context.Context, folder string, value any) error {
	roleID, err := p.roleIDByFolder(ctx, folder)
	if err != nil {
		return err
	}
	index := objectMap(value)
	activeChatID := stringField(index, "activeChatId")
	_, err = p.config.updateProjection(func(projection *projectionConfig) {
		if activeChatID == "" {
			delete(projection.ActiveChatByRole, roleID)
			return
		}
		projection.ActiveChatByRole[roleID] = activeChatID
	})
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
		"toolPolicy":   normalizeUIToolPolicy(role["toolPolicy"]),
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
		"toolPolicy":  normalizeUIToolPolicy(role["toolPolicy"]),
		"createdAt":   timeFromMillis(role["createdAt"]),
		"updatedAt":   time.Now().UTC().Format(time.RFC3339),
	}
}

func normalizeUIToolPolicy(value any) map[string]any {
	policy := objectMap(value)
	runModeSource := objectMap(policy["runModes"])
	tools := []any{}
	runModes := map[string]any{}
	seen := map[string]struct{}{}
	for _, tool := range stringSlice(policy["tools"]) {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		if _, ok := seen[tool]; ok {
			continue
		}
		mode := strings.TrimSpace(fmt.Sprint(runModeSource[tool]))
		seen[tool] = struct{}{}
		tools = append(tools, tool)
		if mode == "ask" || mode == "direct" {
			runModes[tool] = mode
		}
	}
	return map[string]any{"tools": tools, "runModes": runModes}
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
	messages := []map[string]any{}
	for _, msg := range objectList(session["messages"]) {
		createdAt := millisFromAny(msg["createdAt"])
		updatedAt := millisFromAnyOrZero(msg["updatedAt"])
		if updatedAt == 0 {
			updatedAt = createdAt
		}
		uiMessage := map[string]any{"id": stringField(msg, "id"), "type": messageStorageType(msg), "role": messageRole(msg), "content": stringField(msg, "content"), "parentMid": stringField(msg, "parentMessageId"), "branchId": fallback(stringField(msg, "branchId"), "main"), "createdAt": createdAt, "updatedAt": updatedAt}
		if parts := objectList(msg["parts"]); len(parts) > 0 {
			uiMessage["parts"] = anyList(parts)
		}
		images, attachments := toUIMessageAttachments(objectList(msg["attachments"]))
		if len(images) > 0 {
			uiMessage["images"] = images
		}
		if len(attachments) > 0 {
			uiMessage["attachments"] = attachments
		}
		messages = append(messages, uiMessage)
	}
	createdAt := millisFromAny(session["createdAt"])
	updatedAt := millisFromAnyOrZero(session["updatedAt"])
	if updatedAt == 0 {
		updatedAt = millisFromAnyOrZero(session["lastActive"])
	}
	if updatedAt == 0 {
		updatedAt = createdAt
	}
	branching := deriveUIBranching(messages, createdAt, updatedAt)
	return map[string]any{"id": stringField(session, "id"), "title": fallback(stringField(session, "title"), "新聊天"), "createdAt": createdAt, "updatedAt": updatedAt, "branching": branching, "messages": anyList(messages)}
}

func deriveUIBranching(messages []map[string]any, createdAt int64, updatedAt int64) map[string]any {
	if len(messages) == 0 {
		return map[string]any{"schemaVersion": 1, "activeBranchId": "main", "branches": []any{map[string]any{"id": "main", "name": "主线", "headMid": "", "createdAt": createdAt, "updatedAt": updatedAt, "forkFromMid": ""}}}
	}
	byID := map[string]map[string]any{}
	children := map[string][]string{}
	for _, message := range messages {
		id := stringField(message, "id")
		if id == "" {
			continue
		}
		byID[id] = message
		parentID := stringField(message, "parentMid")
		if parentID != "" {
			children[parentID] = append(children[parentID], id)
		}
	}
	for parentID := range children {
		sort.SliceStable(children[parentID], func(i int, j int) bool {
			return messageSortLess(byID[children[parentID][i]], byID[children[parentID][j]])
		})
	}
	leaves := []map[string]any{}
	for _, message := range messages {
		id := stringField(message, "id")
		if id == "" {
			continue
		}
		if len(children[id]) == 0 {
			leaves = append(leaves, message)
		}
	}
	if len(leaves) == 0 {
		leaves = append(leaves, messages[len(messages)-1])
	}
	sort.SliceStable(leaves, func(i int, j int) bool { return messageSortLess(leaves[i], leaves[j]) })
	branches := make([]any, 0, len(leaves))
	activeBranchID := "main"
	usedBranchIDs := map[string]struct{}{}
	for index, leaf := range leaves {
		headMid := stringField(leaf, "id")
		branchID := fallback(stringField(leaf, "branchId"), "main")
		branchName := "主线"
		if _, ok := usedBranchIDs[branchID]; ok {
			branchID = "leaf_" + sanitizeBranchID(headMid)
		}
		if len(leaves) > 1 && branchID != "main" {
			branchName = fmt.Sprintf("分支 %d", index+1)
		}
		usedBranchIDs[branchID] = struct{}{}
		if index == len(leaves)-1 {
			activeBranchID = branchID
		}
		branches = append(branches, map[string]any{"id": branchID, "name": branchName, "headMid": headMid, "createdAt": millisFromAnyOrZero(leaf["createdAt"]), "updatedAt": updatedAt, "forkFromMid": stringField(leaf, "parentMid")})
	}
	return map[string]any{"schemaVersion": 1, "activeBranchId": activeBranchID, "branches": branches}
}

func messageSortLess(left map[string]any, right map[string]any) bool {
	leftTime := millisFromAnyOrZero(left["createdAt"])
	rightTime := millisFromAnyOrZero(right["createdAt"])
	if leftTime != rightTime {
		return leftTime < rightTime
	}
	return stringField(left, "id") < stringField(right, "id")
}

func sanitizeBranchID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "main"
	}
	replacer := strings.NewReplacer(" ", "_", "/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return replacer.Replace(value)
}

func anyList(items []map[string]any) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func fromUIChat(value any, roleID string) map[string]any {
	chat := objectMap(value)
	messages := []any{}
	for _, msg := range objectList(chat["messages"]) {
		message := map[string]any{"id": stringField(msg, "id"), "type": messageStorageType(msg), "content": stringField(msg, "content"), "parentMessageId": stringField(msg, "parentMid"), "branchId": fallback(stringField(msg, "branchId"), "main"), "createdAt": timeFromMillis(msg["createdAt"]), "updatedAt": timeFromMillis(msg["updatedAt"])}
		if parts := objectList(msg["parts"]); len(parts) > 0 {
			message["parts"] = anyList(parts)
		}
		attachments := fromUIMessageAttachments(objectList(msg["attachments"]), stringSlice(msg["images"]))
		if len(attachments) > 0 {
			message["attachments"] = attachments
		}
		messages = append(messages, message)
	}
	updatedAt := timeFromMillis(chat["updatedAt"])
	return map[string]any{"id": stringField(chat, "id"), "roleId": roleID, "title": fallback(stringField(chat, "title"), "新聊天"), "status": "created", "messages": messages, "createdAt": timeFromMillis(chat["createdAt"]), "updatedAt": updatedAt, "lastActive": updatedAt}
}

func toUIMessageAttachments(attachments []map[string]any) ([]any, []any) {
	images := []any{}
	files := []any{}
	for _, attachment := range attachments {
		kind := strings.TrimSpace(strings.ToLower(stringField(attachment, "kind")))
		if kind == "image" {
			path := stringField(attachment, "path")
			if path != "" {
				images = append(images, path)
			}
			continue
		}
		text := stringField(attachment, "text")
		if text == "" {
			continue
		}
		fullLen := int(numberField(attachment, "fullLen", float64(len([]rune(text)))))
		sendLen := int(numberField(attachment, "sendLen", float64(len([]rune(text)))))
		sendPct := int(numberField(attachment, "sendPct", 100))
		files = append(files, map[string]any{"id": stringField(attachment, "id"), "name": fallback(stringField(attachment, "name"), "文件"), "kind": fallback(kind, "txt"), "lang": fallback(stringField(attachment, "lang"), "text"), "text": text, "fullLen": fullLen, "sendLen": sendLen, "sendPct": sendPct})
	}
	return images, files
}

func fromUIMessageAttachments(files []map[string]any, images []string) []any {
	attachments := []any{}
	for _, imagePath := range images {
		imagePath = strings.TrimSpace(imagePath)
		if imagePath == "" {
			continue
		}
		attachments = append(attachments, map[string]any{"kind": "image", "name": "图片", "path": imagePath})
	}
	for _, file := range files {
		text := stringField(file, "text")
		if text == "" {
			continue
		}
		attachments = append(attachments, map[string]any{"id": stringField(file, "id"), "kind": fallback(stringField(file, "kind"), "txt"), "name": fallback(stringField(file, "name"), "文件"), "lang": fallback(stringField(file, "lang"), "text"), "text": text, "fullLen": int(numberField(file, "fullLen", float64(len([]rune(text))))), "sendLen": int(numberField(file, "sendLen", float64(len([]rune(text))))), "sendPct": int(numberField(file, "sendPct", 100))})
	}
	return attachments
}

func mergeSettings(settings map[string]any, providers []map[string]any, mermaidFix map[string]any, chatTitleNaming map[string]any, stickerNaming map[string]any) map[string]any {
	out := map[string]any{"streamEnabled": true, "transparentChatBg": false, "chatBgOpacity": 0, "chatBgBlur": 0, "topbarOpacity": 100, "topbarBlur": 0, "composerOpacity": 86, "composerBlur": 10, "branchTree": map[string]any{"dir": "lr", "view": "float", "followSelected": true, "modalHotkey": ""}, "renderSafetyPolicy": "original", "userMessageCollapseEnabled": false, "userMessageCollapseLines": 8, "attachments": map[string]any{"sendLimitChars": 80000, "maxFileSizeMbByKind": map[string]any{"txt": 10, "md": 10, "pdf": 10, "docx": 10, "ppt": 10}}, "stickers": map[string]any{"enabled": false, "categories": []any{}, "map": map[string]any{}}, "providers": []any{}}
	for k, v := range settings {
		out[k] = v
	}
	uiProviders := []any{}
	for _, provider := range providers {
		uiProviders = append(uiProviders, toUIProvider(provider))
	}
	out["providers"] = uiProviders
	aiServices := objectMap(out["aiServices"])
	aiServices["mermaidFix"] = mermaidFix
	aiServices["chatTitleNaming"] = chatTitleNaming
	aiServices["stickerNaming"] = stickerNaming
	out["aiServices"] = aiServices
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

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]string); ok {
			return append([]string(nil), typed...)
		}
		return []string{}
	}
	out := []string{}
	seen := map[string]struct{}{}
	for _, item := range items {
		id := strings.TrimSpace(fmt.Sprint(item))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func mapKeys[T any](value map[string]T) []string {
	out := make([]string, 0, len(value))
	for key := range value {
		out = append(out, key)
	}
	return out
}

func orderedIDs(preferred []string, actual []string) []string {
	actualSet := map[string]struct{}{}
	for _, id := range actual {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		actualSet[id] = struct{}{}
	}
	out := []string{}
	seen := map[string]struct{}{}
	for _, id := range preferred {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := actualSet[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range actual {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
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
func boolField(m map[string]any, key string, fallback bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
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
	if n, ok := value.(int64); ok {
		return n
	}
	if n, ok := value.(int); ok {
		return int64(n)
	}
	return nowMillis()
}
func millisFromAnyOrZero(value any) int64 {
	if s, ok := value.(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UnixMilli()
		}
	}
	if n, ok := value.(float64); ok {
		return int64(n)
	}
	if n, ok := value.(int64); ok {
		return n
	}
	if n, ok := value.(int); ok {
		return int64(n)
	}
	return 0
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
	switch messageStorageType(message) {
	case "assistant", "tool", "tool_request", "tool_confirmation":
		return "assistant"
	}
	return "user"
}

func messageStorageType(message map[string]any) string {
	messageType := stringField(message, "type")
	if messageType == "" {
		messageType = stringField(message, "role")
	}
	switch messageType {
	case "assistant", "tool", "tool_request", "tool_confirmation", "failure":
		return messageType
	default:
		return "user"
	}
}

func isNotFoundError(err error) bool { return err != nil && strings.Contains(err.Error(), "不存在") }
