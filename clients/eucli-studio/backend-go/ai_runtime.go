package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	assistantStreamKeyPrefix = "bg.stream."
	assistantFinalKeyPrefix  = "engine.v1/final/"
	assistantMidRunKeyPrefix = "engine.v1/mid-run/"
	toolConfirmationKey      = "engine.v1/tool-confirmation"
	uiChatUpdatedNoticeKey   = "ui/notice/chat-updated"
)

func newService(dataDir string) *service {
	svc := &service{dataDir: dataDir}
	initialConfig := loadBoxConnectionFromStorage(svc)
	svc.box = newBoxClient(svc, initialConfig)
	svc.ai = newAIRunQueue(svc)
	svc.box.setToolConfirmationCallback(func(event boxRunEvent) {
		_ = svc.storageSetByKey("runtime/"+toolConfirmationKey, map[string]any{
			"event":   event,
			"updated": time.Now().UnixMilli(),
		})
		svc.pushFrontendNotification("aiChat.tool.confirmation", event)
	})
	svc.box.events.setOnDisconnect(func() {
		_ = svc.storageRemoveByKey("runtime/" + toolConfirmationKey)
		svc.pushFrontendNotification("box.ws.disconnected", map[string]any{"at": time.Now().UnixMilli()})
	})
	return svc
}

func assistantStreamStorageKey(assistantMid string) string {
	return "runtime/" + assistantStreamKeyPrefix + strings.TrimSpace(assistantMid)
}

func assistantFinalStorageKey(assistantMid string) string {
	return "runtime/" + assistantFinalKeyPrefix + strings.TrimSpace(assistantMid)
}

func assistantMidRunStorageKey(assistantMid string) string {
	return "runtime/" + assistantMidRunKeyPrefix + strings.TrimSpace(assistantMid)
}

func uiChatUpdatedNoticeStorageKey() string {
	return "runtime/" + uiChatUpdatedNoticeKey
}

func requestAssistantMid(params json.RawMessage) string {
	var payload map[string]any
	_ = json.Unmarshal(params, &payload)
	return strings.TrimSpace(asString(payload["assistantMid"]))
}

func (svc *service) storageSetByKey(key string, value any) error {
	path, err := svc.storagePathForKey(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	prepared, err := svc.prepareStorageValueForSet(key, value)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(prepared, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWriteFile(path, data, 0o644)
}

func (svc *service) storageRemoveByKey(key string) error {
	path, err := svc.storagePathForKey(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (svc *service) imageReadDataURLByRel(relPath string) (string, error) {
	path, _, err := svc.imagePathForRel(relPath)
	if err != nil {
		return "", err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	mime := imageMimeFromExt(filepath.Ext(path))
	if !strings.HasPrefix(mime, "image/") {
		return "", fmt.Errorf("unsupported image MIME: %s", mime)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(payload), nil
}

func (svc *service) getPendingConfirmation() (any, error) {
	key := "runtime/" + toolConfirmationKey
	value, err := svc.storageGetByKey(key)
	if value == nil && !svc.box.isBoxConnected() {
		return map[string]any{"disconnected": true}, nil
	}
	return value, err
}

func (svc *service) submitConfirmation(params json.RawMessage) (map[string]bool, error) {
	var payload struct {
		DecisionID string `json:"decisionId"`
		Approved   bool   `json:"approved"`
	}
	_ = json.Unmarshal(params, &payload)
	decisionID := strings.TrimSpace(payload.DecisionID)
	if decisionID == "" {
		return nil, errors.New("decisionId is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := svc.box.submitToolConfirmation(ctx, decisionID, payload.Approved); err != nil {
		return nil, err
	}

	_ = svc.storageRemoveByKey("runtime/" + toolConfirmationKey)
	return map[string]bool{"ok": true}, nil
}

func (svc *service) getBoxConnection() (map[string]any, error) {
	value, err := svc.storageGetByKey("box/connection")
	if err != nil {
		return nil, err
	}
	if value == nil {
		return map[string]any{"url": "http://127.0.0.1:8765", "key": ""}, nil
	}
	cfg, ok := value.(map[string]any)
	if !ok || cfg == nil {
		if value != nil {
			log.Printf("getBoxConnection: unexpected type %T, using default config", value)
		}
		return map[string]any{"url": "http://127.0.0.1:8765", "key": ""}, nil
	}
	return cfg, nil
}

func (svc *service) saveBoxConnection(params json.RawMessage) (map[string]any, error) {
	var payload struct {
		URL string `json:"url"`
		Key string `json:"key"`
	}
	if err := json.Unmarshal(params, &payload); err != nil {
		return nil, err
	}
	url := strings.TrimSpace(payload.URL)
	if url == "" {
		return nil, errors.New("url is required")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, errors.New("url must start with http:// or https://")
	}
	url = strings.TrimRight(url, "/")
	key := strings.TrimSpace(payload.Key)
	cfg := map[string]any{"url": url, "key": key, "updatedAt": nowMs()}
	if err := svc.storageSetByKey("box/connection", cfg); err != nil {
		return nil, err
	}
	svc.box.reloadConnection(svc)
	return cfg, nil
}

func (svc *service) testBoxConnection() (map[string]any, error) {
	value, err := svc.storageGetByKey("box/connection")
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, errors.New("请先在设置页面配置 eucli-box 连接地址")
	}
	cfg, ok := value.(map[string]any)
	if !ok || cfg == nil || strings.TrimSpace(asString(cfg["url"])) == "" {
		if !ok && value != nil {
			log.Printf("testBoxConnection: unexpected type %T", value)
		}
		return nil, errors.New("请先在设置页面配置 eucli-box 连接地址")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := svc.box.health(ctx)
	if strings.TrimSpace(asString(result["status"])) != "ok" {
		return nil, fmt.Errorf("无法连接到 eucli-box: %s", asString(result["error"]))
	}
	result["url"] = cfg["url"]
	return result, nil
}

func (svc *service) pushBoxRole(params json.RawMessage) (map[string]any, error) {
	var role map[string]any
	if err := json.Unmarshal(params, &role); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := svc.box.putRole(ctx, role); err != nil {
		return nil, err
	}
	synced := true
	if err := svc.syncBoxCatalog(context.Background()); err != nil {
		log.Printf("[WARN] pushBoxRole: syncBoxCatalog failed after PUT succeeded: %v", err)
		synced = false
	}
	return map[string]any{"ok": true, "synced": synced}, nil
}

func (svc *service) pushBoxProvider(params json.RawMessage) (map[string]any, error) {
	var provider map[string]any
	if err := json.Unmarshal(params, &provider); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := svc.box.putProvider(ctx, provider); err != nil {
		return nil, err
	}
	synced := true
	if err := svc.syncBoxCatalog(context.Background()); err != nil {
		log.Printf("[WARN] pushBoxProvider: syncBoxCatalog failed after PUT succeeded: %v", err)
		synced = false
	}
	return map[string]any{"ok": true, "synced": synced}, nil
}

func (svc *service) deleteBoxRole(params json.RawMessage) (map[string]any, error) {
	var payload struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(params, &payload)
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		return nil, errors.New("id is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := svc.box.deleteRole(ctx, id); err != nil {
		return nil, err
	}
	synced := true
	if err := svc.syncBoxCatalog(context.Background()); err != nil {
		log.Printf("[WARN] deleteBoxRole: syncBoxCatalog failed after DELETE succeeded: %v", err)
		synced = false
	}
	return map[string]any{"ok": true, "synced": synced}, nil
}

func (svc *service) deleteBoxProvider(params json.RawMessage) (map[string]any, error) {
	var payload struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(params, &payload)
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		return nil, errors.New("id is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := svc.box.deleteProvider(ctx, id); err != nil {
		return nil, err
	}
	synced := true
	if err := svc.syncBoxCatalog(context.Background()); err != nil {
		log.Printf("[WARN] deleteBoxProvider: syncBoxCatalog failed after DELETE succeeded: %v", err)
		synced = false
	}
	return map[string]any{"ok": true, "synced": synced}, nil
}

func (svc *service) syncBoxCatalogRPC() (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := svc.syncBoxCatalog(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func (svc *service) listBoxSessions(params json.RawMessage) (any, error) {
	var payload struct {
		RoleID string `json:"roleId"`
	}
	_ = json.Unmarshal(params, &payload)
	roleID := strings.TrimSpace(payload.RoleID)
	if roleID == "" {
		return nil, errors.New("roleId is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return svc.box.listSessions(ctx, roleID)
}

func (svc *service) createBoxSession(params json.RawMessage) (any, error) {
	var payload struct {
		RoleID string `json:"roleId"`
	}
	_ = json.Unmarshal(params, &payload)
	roleID := strings.TrimSpace(payload.RoleID)
	if roleID == "" {
		return nil, errors.New("roleId is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return svc.box.createSession(ctx, roleID)
}

func (svc *service) getBoxSession(params json.RawMessage) (any, error) {
	var payload struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(params, &payload)
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		return nil, errors.New("id is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return svc.box.getSession(ctx, id)
}

func (svc *service) deleteBoxSession(params json.RawMessage) (map[string]any, error) {
	var payload struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(params, &payload)
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		return nil, errors.New("id is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := svc.box.deleteSession(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}
