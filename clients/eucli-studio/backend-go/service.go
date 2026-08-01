package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

type releaseChecker interface {
	CheckOnly(ctx context.Context, installed []releasecheck.InstalledArtifact, currentBoxVersion string, requested []types.ReleaseArtifactIdentity) types.ReleaseCheckSnapshot
}

type service struct {
	config              *configStore
	release             clientRelease
	eb                  *ebClient
	projection          *projectionService
	runtime             *runtimeStore
	hub                 *eventHub
	connectionMu        sync.RWMutex
	connectionState     *runtimeBootstrap
	connectionChanged   chan struct{}
	releaseChecker      releaseChecker
	releaseCheckMu      sync.RWMutex
	releaseChecks       types.ReleaseCheckSnapshot
	releaseCheckRunning bool
}

func newService(config *configStore, release clientRelease, hub *eventHub, checker releaseChecker) (*service, error) {
	if config == nil {
		return nil, errors.New("eucli-studio config store is required")
	}
	if checker == nil {
		return nil, errors.New("eucli-studio release checker is required")
	}
	eb := newEBClient(config, release)
	return &service{
		config: config, release: release, eb: eb, projection: newProjectionService(config, eb), runtime: newRuntimeStore(), hub: hub,
		connectionChanged: make(chan struct{}, 1), releaseChecker: checker,
		releaseChecks: releasecheck.PendingSnapshot(),
	}, nil
}

func (s *service) dispatch(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "aiChat.healthCheck":
		cfg, _ := s.config.load()
		return map[string]any{"protocolVersion": directProtocolVersion, "clientVersion": s.release.Version, "status": "ok", "configured": cfg.EucliBoxURL != ""}, nil
	case "studio.bootstrap":
		return s.bootstrap(ctx)
	case "releaseChecks.refresh":
		return s.refreshReleaseChecks(ctx)
	case "eucli.config.get":
		return s.config.load()
	case "eucli.config.set":
		var cfg clientConfig
		if err := json.Unmarshal(paramsOrEmpty(params), &cfg); err != nil {
			return nil, err
		}
		saved, err := s.config.save(cfg)
		if err == nil {
			s.clearConnectionState()
		}
		return saved, err
	}
	if !isBusinessMethod(method) {
		return nil, newError("METHOD_NOT_FOUND", "未知请求："+method)
	}
	if err := s.requireBusinessConnection(); err != nil {
		return nil, err
	}
	switch method {
	case "eucli.eb.request":
		var req ebRequest
		if err := json.Unmarshal(paramsOrEmpty(params), &req); err != nil {
			return nil, err
		}
		return s.eb.request(ctx, req)
	case "aiChat.netRequest":
		return s.handleNetRequest(ctx, params)
	case "aiChat.submitChatCompletion", "aiChat.submitManyChatCompletions", "aiChat.submitRawServiceRequest":
		return nil, newError("EB_MODEL_REQUIRED", fmt.Sprintf("%s 必须通过 e-b runs 接口推进，客户端不再本地运行", method))
	case "aiChat.cancelAssistant", "aiChat.getAssistantRuntime", "aiChat.readAssistantStream", "aiChat.consumeAssistantFinal", "aiChat.resetAssistantRuntime", "aiChat.waitServiceFinal":
		return nil, newError("EB_MODEL_REQUIRED", fmt.Sprintf("%s 必须通过 e-b runs/events 接口读取，客户端不再维护本地运行态", method))
	case "aiChat.storageGet":
		key, err := storageKey(params)
		if err != nil {
			return nil, err
		}
		if logicalKey, ok := runtimeLogicalKey(key); ok {
			value, exists := s.runtime.get(logicalKey)
			if !exists {
				return nil, nil
			}
			return value, nil
		}
		return s.projection.get(ctx, key)
	case "aiChat.storageSet":
		key, value, err := storageSetPayload(params)
		if err != nil {
			return nil, err
		}
		if logicalKey, ok := runtimeLogicalKey(key); ok {
			s.runtime.set(logicalKey, value)
			s.broadcastRuntimeStorageSet(logicalKey, value)
			return map[string]any{}, nil
		}
		return map[string]any{}, s.projection.set(ctx, key, value)
	case "aiChat.storageRemove":
		key, err := storageKey(params)
		if err != nil {
			return nil, err
		}
		if logicalKey, ok := runtimeLogicalKey(key); ok {
			s.runtime.remove(logicalKey)
			return map[string]any{}, nil
		}
		return map[string]any{}, s.projection.remove(ctx, key)
	case "aiChat.imageRead":
		return s.handleImageRead(ctx, params)
	case "aiChat.imageWrite":
		return s.handleImageWrite(ctx, params)
	case "aiChat.imageDelete":
		return s.handleImageDelete(ctx, params)
	case "aiChat.imagePick":
		return nil, newError("NOT_IMPLEMENTED", fmt.Sprintf("%s 必须由宿主 UI 处理", method))
	default:
		return nil, newError("METHOD_NOT_FOUND", "未知请求："+method)
	}
}

func (s *service) startStandaloneReleaseCheck(ctx context.Context) {
	cfg, err := s.config.load()
	if err != nil || strings.TrimSpace(cfg.EucliBoxURL) != "" {
		return
	}
	s.releaseCheckMu.Lock()
	if s.releaseCheckRunning {
		s.releaseCheckMu.Unlock()
		return
	}
	s.releaseCheckRunning = true
	s.releaseChecks = releasecheck.CheckingSnapshot(s.releaseChecks, time.Now().UTC())
	s.releaseCheckMu.Unlock()
	go func() {
		snapshot := s.checkBoxOfficialSource(ctx)
		s.releaseCheckMu.Lock()
		s.releaseChecks = snapshot
		s.releaseCheckRunning = false
		s.releaseCheckMu.Unlock()
	}()
}

func (s *service) refreshReleaseChecks(ctx context.Context) (types.ReleaseCheckSnapshot, error) {
	state := s.connectionSnapshot()
	if state != nil && state.EucliBoxReachable {
		raw, err := s.eb.request(ctx, ebRequest{Method: "POST", Path: "/api/release-checks/refresh", Timeout: 30000})
		if err != nil {
			return types.ReleaseCheckSnapshot{}, err
		}
		return decodeReleaseCheckSnapshot(raw)
	}
	snapshot := s.checkBoxOfficialSource(ctx)
	s.releaseCheckMu.Lock()
	s.releaseChecks = snapshot
	s.releaseCheckRunning = false
	s.releaseCheckMu.Unlock()
	return snapshot, nil
}

func (s *service) checkBoxOfficialSource(ctx context.Context) types.ReleaseCheckSnapshot {
	return s.releaseChecker.CheckOnly(ctx, nil, "", []types.ReleaseArtifactIdentity{{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox}})
}

func (s *service) releaseCheckSnapshot() types.ReleaseCheckSnapshot {
	s.releaseCheckMu.RLock()
	defer s.releaseCheckMu.RUnlock()
	result := s.releaseChecks
	result.Results = append([]types.ReleaseCheckResult(nil), s.releaseChecks.Results...)
	return result
}

func isBusinessMethod(method string) bool {
	switch method {
	case "eucli.eb.request",
		"aiChat.netRequest",
		"aiChat.submitChatCompletion",
		"aiChat.submitManyChatCompletions",
		"aiChat.submitRawServiceRequest",
		"aiChat.cancelAssistant",
		"aiChat.getAssistantRuntime",
		"aiChat.readAssistantStream",
		"aiChat.consumeAssistantFinal",
		"aiChat.resetAssistantRuntime",
		"aiChat.waitServiceFinal",
		"aiChat.storageGet",
		"aiChat.storageSet",
		"aiChat.storageRemove",
		"aiChat.imageRead",
		"aiChat.imageWrite",
		"aiChat.imageDelete",
		"aiChat.imagePick":
		return true
	default:
		return false
	}
}

func (s *service) broadcastRuntimeStorageSet(logicalKey string, value any) {
	if logicalKey != chatUpdatedNoticeRuntimeKey || s.hub == nil {
		return
	}

	// Chat-updated notices are the event-driven refresh root for the UI. They are
	// written only after the durable chat write path succeeds, so broadcasting
	// them here makes the real data change speak for itself. Do not route this
	// through projection/e-b storage: the notice is a runtime coordination fact,
	// not business data, and using it as the primary signal keeps the UI from
	// falling back to high-frequency polling for normal updates.
	s.hub.broadcast(eventFrame{Type: "event", Name: directEventChatUpdated, Payload: value})
}

func (s *service) handleImageRead(ctx context.Context, params json.RawMessage) (any, error) {
	req, err := imagePayload(params)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(filepath.ToSlash(req.Path), "stickers/") {
		return s.eb.request(ctx, ebRequest{Method: "GET", Path: "/api/stickers/image", Query: mustJSON(map[string]any{"path": filepath.ToSlash(req.Path)})})
	}
	if strings.HasPrefix(filepath.ToSlash(req.Path), "sessions/") {
		return s.eb.request(ctx, ebRequest{Method: "GET", Path: "/api/session-attachments/image", Query: mustJSON(map[string]any{"path": filepath.ToSlash(req.Path)})})
	}
	if groupAvatarPathPattern.MatchString(strings.TrimSpace(filepath.ToSlash(req.Path))) {
		groupID, err := s.projection.groupIDByAvatarPath(ctx, req.Path)
		if err != nil {
			return nil, err
		}
		return s.eb.request(ctx, ebRequest{Method: "GET", Path: fmt.Sprintf("/api/groups/%s/avatar", groupID)})
	}
	roleID, err := s.projection.roleIDByAvatarPath(ctx, req.Path)
	if err != nil {
		return nil, err
	}
	return s.eb.request(ctx, ebRequest{Method: "GET", Path: fmt.Sprintf("/api/roles/%s/avatar", roleID)})
}

func (s *service) handleImageWrite(ctx context.Context, params json.RawMessage) (any, error) {
	req, err := imagePayload(params)
	if err != nil {
		return nil, err
	}
	roleID, err := s.projection.roleIDByAvatarPath(ctx, req.Path)
	if err != nil {
		groupID, groupErr := s.projection.groupIDByAvatarPath(ctx, req.Path)
		if groupErr != nil {
			return nil, err
		}
		if req.DataURL == "" {
			return nil, newError("BAD_REQUEST", "image data url is required")
		}
		_, err = s.eb.request(ctx, ebRequest{Method: "PUT", Path: fmt.Sprintf("/api/groups/%s/avatar", groupID), Body: mustJSON(map[string]any{"dataUrl": req.DataURL})})
		if err != nil {
			return nil, err
		}
		return map[string]any{"relPath": req.Path}, nil
	}
	if req.DataURL == "" {
		return nil, newError("BAD_REQUEST", "image data url is required")
	}
	_, err = s.eb.request(ctx, ebRequest{Method: "PUT", Path: fmt.Sprintf("/api/roles/%s/avatar", roleID), Body: mustJSON(map[string]any{"dataUrl": req.DataURL})})
	if err != nil {
		return nil, err
	}
	return map[string]any{"relPath": req.Path}, nil
}

func (s *service) handleImageDelete(ctx context.Context, params json.RawMessage) (any, error) {
	req, err := imagePayload(params)
	if err != nil {
		return nil, err
	}
	roleID, err := s.projection.roleIDByAvatarPath(ctx, req.Path)
	if err != nil {
		groupID, groupErr := s.projection.groupIDByAvatarPath(ctx, req.Path)
		if groupErr != nil {
			return nil, err
		}
		_, err = s.eb.request(ctx, ebRequest{Method: "DELETE", Path: fmt.Sprintf("/api/groups/%s/avatar", groupID)})
		if err != nil {
			return nil, err
		}
		return map[string]any{}, nil
	}
	_, err = s.eb.request(ctx, ebRequest{Method: "DELETE", Path: fmt.Sprintf("/api/roles/%s/avatar", roleID)})
	if err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}

type imageRequest struct {
	Path    string
	DataURL string
}

func imagePayload(params json.RawMessage) (imageRequest, error) {
	var raw map[string]any
	if err := json.Unmarshal(paramsOrEmpty(params), &raw); err != nil {
		return imageRequest{}, err
	}
	path := strings.TrimSpace(fmt.Sprint(firstPresent(raw, "relPath", "path")))
	if path == "" {
		return imageRequest{}, newError("BAD_REQUEST", "image path is required")
	}
	return imageRequest{Path: path, DataURL: strings.TrimSpace(fmt.Sprint(firstPresent(raw, "dataUrlOrBase64", "dataUrl", "base64")))}, nil
}

func firstPresent(raw map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := raw[key]; ok && value != nil {
			return value
		}
	}
	return ""
}

func storageKey(params json.RawMessage) (string, error) {
	var req struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(paramsOrEmpty(params), &req); err != nil {
		return "", err
	}
	if req.Key == "" {
		return "", newError("BAD_REQUEST", "storage key is required")
	}
	return req.Key, nil
}

func storageSetPayload(params json.RawMessage) (string, any, error) {
	var req struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(paramsOrEmpty(params), &req); err != nil {
		return "", nil, err
	}
	if req.Key == "" {
		return "", nil, newError("BAD_REQUEST", "storage key is required")
	}
	return req.Key, req.Value, nil
}

func (s *service) handleNetRequest(ctx context.Context, params json.RawMessage) (any, error) {
	var raw map[string]any
	if err := json.Unmarshal(paramsOrEmpty(params), &raw); err != nil {
		return nil, err
	}
	path, _ := raw["path"].(string)
	if path == "" {
		urlValue, _ := raw["url"].(string)
		path = urlValue
	}
	if path == "" {
		return nil, errors.New("aiChat.netRequest requires path")
	}
	body, _ := json.Marshal(raw["body"])
	query, _ := json.Marshal(raw["query"])
	req := ebRequest{Method: fmt.Sprint(raw["method"]), Path: path, Query: query, Body: body}
	if timeout, ok := raw["timeoutMs"].(float64); ok {
		req.Timeout = int(timeout)
	}
	data, err := s.eb.request(ctx, req)
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": 200, "body": data}, nil
}

func paramsOrEmpty(params json.RawMessage) json.RawMessage {
	if len(params) == 0 || string(params) == "null" {
		return json.RawMessage(`{}`)
	}
	return params
}
