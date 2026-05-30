package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
	return svc.storageGetByKey(key)
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := svc.box.postJSON(ctx, "/api/tool-confirmations", map[string]any{
		"id":         "tc-" + fmt.Sprint(time.Now().UnixMilli()),
		"decisionId": decisionID,
		"approved":   payload.Approved,
		"createdAt":  time.Now().UTC().Format(time.RFC3339),
	}, nil); err != nil {
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
	cfg, _ := value.(map[string]any)
	if cfg == nil {
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
	cfg, _ := value.(map[string]any)
	if cfg == nil || strings.TrimSpace(asString(cfg["url"])) == "" {
		return nil, errors.New("请先在设置页面配置 eucli-box 连接地址")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := svc.box.health(ctx)
	result["url"] = cfg["url"]
	return result, nil
}
