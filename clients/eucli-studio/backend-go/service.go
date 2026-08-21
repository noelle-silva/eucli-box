package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

type releaseChecker interface {
	CheckOnly(ctx context.Context, installed []releasecheck.InstalledArtifact, currentBoxVersion string, requested []types.ReleaseArtifactIdentity) types.ReleaseCheckSnapshot
	LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error)
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
	businessConnection  *boxConnection
	connectionChanged   chan struct{}
	localBox            *localBoxManager
	shutdownMu          sync.RWMutex
	shutdown            func()
	releaseChecker      releaseChecker
	releaseCheckMu      sync.RWMutex
	releaseChecks       types.ReleaseCheckSnapshot
	releaseCheckRunning bool
}

func newService(config *configStore, release clientRelease, hub *eventHub, source localBoxArtifactSource, checker releaseChecker, devBoxRoot string) (*service, error) {
	if config == nil {
		return nil, errors.New("eucli-studio config store is required")
	}
	eb := newEBClient(config, release)
	paths, err := newLocalBoxPathsWithRoot(filepath.Dir(config.path), devBoxRoot)
	if err != nil {
		return nil, err
	}
	service := &service{
		config: config, release: release, eb: eb, projection: newProjectionService(config, eb), runtime: newRuntimeStore(), hub: hub,
		connectionChanged: make(chan struct{}, 1), releaseChecker: checker,
		releaseChecks: releasecheck.PendingSnapshot(),
	}
	eb.setConnectionProvider(service.currentBoxConnection)
	service.localBox = newLocalBoxManager(paths, source, service.publishLocalBoxState, service.setLocalBoxConnection, service.clearLocalBoxConnection)
	return service, nil
}

func (s *service) dispatch(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "aiChat.healthCheck":
		cfg, _ := s.config.load()
		return map[string]any{"protocolVersion": directProtocolVersion, "clientVersion": s.release.Version, "status": "ok", "configured": cfg.EucliBoxURL != ""}, nil
	case "studio.bootstrap":
		return s.bootstrap(ctx)
	case "localBox.status":
		return s.localBoxStatus(ctx)
	case "localBox.install":
		return s.localBoxInstall(ctx)
	case "localBox.update":
		return s.localBoxUpdate(ctx)
	case "localBox.start":
		return s.localBoxStart(ctx)
	case "localBox.restart":
		return s.localBoxRestart(ctx)
	case "localBox.stop":
		return s.localBoxStop(ctx)
	case "localBox.exit":
		return s.localBoxExit(ctx)
	case "clientSettings.get":
		return s.config.getSettings()
	case "clientSettings.set":
		var settingReq struct {
			Name  string `json:"name"`
			Value any    `json:"value"`
		}
		if err := json.Unmarshal(paramsOrEmpty(params), &settingReq); err != nil {
			return nil, err
		}
		if settingReq.Name == "boxSourceKind" {
			return s.setBoxSourceKind(ctx, settingReq.Value)
		}
		if _, err := s.config.updateSetting(settingReq.Name, settingReq.Value); err != nil {
			return nil, err
		}
		return s.config.getSettings()
	case "releaseChecks.refresh":
		var refreshReq struct {
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(paramsOrEmpty(params), &refreshReq)
		return s.refreshReleaseChecks(ctx, refreshReq.Kind)
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

func (s *service) localBoxStatus(ctx context.Context) (localBoxState, error) {
	if s.localBox == nil {
		return initialLocalBoxState(), nil
	}
	return s.localBox.status(ctx)
}

func (s *service) localBoxInstall(ctx context.Context) (localBoxState, error) {
	if s.localBox == nil {
		return initialLocalBoxState(), newError("LOCAL_BOX_INSTALL_FAILED", "本机业务端职责未初始化")
	}
	return s.localBox.install(ctx)
}

// localBoxUpdate 是业务端更新入口，属于非业务方法：
// 客户端不适用时更新入口仍可用（需求 U3、设计 4.6）。
func (s *service) localBoxUpdate(ctx context.Context) (localBoxState, error) {
	if s.localBox == nil {
		return initialLocalBoxState(), newError("LOCAL_BOX_UPDATE_FAILED", "本机业务端职责未初始化")
	}
	return s.localBox.update(ctx, s.release)
}

func (s *service) localBoxExit(ctx context.Context) (localBoxState, error) {
	if s.localBox == nil {
		state := initialLocalBoxState()
		state.Status = localBoxStatusStopped
		return state, nil
	}
	cfg, err := s.config.load()
	if err != nil {
		return s.localBox.currentState(), err
	}
	if cfg.KeepBoxRunningOnExit {
		s.localBox.detach(ctx)
		s.requestShutdown()
		state := s.localBox.currentState()
		// 后台运行模式下客户端退出时，业务端继续运行；
		// 对客户端而言连接关系已结束，返回 stopped 让窗口正常关闭。
		state.Status = localBoxStatusStopped
		state.Connected = false
		return state, nil
	}
	state, err := s.localBox.stop(ctx)
	if err == nil && state.Status == localBoxStatusStopped {
		s.requestShutdown()
	}
	return state, err
}

// localBoxStart 启动当前已安装业务端；已在运行时直接返回当前状态。
func (s *service) localBoxStart(ctx context.Context) (localBoxState, error) {
	if s.localBox == nil {
		return initialLocalBoxState(), newError("LOCAL_BOX_INSTALL_FAILED", "本机业务端职责未初始化")
	}
	return s.localBox.start(ctx)
}

// localBoxRestart 先让业务端正常退出并等待真实结束，再启动新业务端。
func (s *service) localBoxRestart(ctx context.Context) (localBoxState, error) {
	if s.localBox == nil {
		return initialLocalBoxState(), newError("LOCAL_BOX_INSTALL_FAILED", "本机业务端职责未初始化")
	}
	if err := s.localBox.restart(ctx); err != nil {
		return s.localBox.currentState(), err
	}
	return s.localBox.status(ctx)
}

// localBoxStop 停止当前业务端并等待真实结束。
func (s *service) localBoxStop(ctx context.Context) (localBoxState, error) {
	if s.localBox == nil {
		return initialLocalBoxState(), nil
	}
	return s.localBox.stop(ctx)
}

func (s *service) setLocalBoxConnection(connection *boxConnection) {
	s.connectionMu.Lock()
	s.businessConnection = connection
	s.connectionMu.Unlock()
	// 已经通过本机受托连接成功连接，清理客户端设置中保存的旧地址和旧 Key 副本。
	if connection != nil && connection.Source == boxConnectionSourceLocal {
		if err := s.config.clearLegacyConnection(); err != nil {
			log.Printf("eucli-studio: 清理旧连接副本失败（不影响本次连接）：%v", err)
		}
	}
	s.signalConnectionChanged()
}

func (s *service) clearLocalBoxConnection() {
	s.connectionMu.Lock()
	s.businessConnection = nil
	if s.connectionState != nil {
		state := *s.connectionState
		state.BusinessAvailable = false
		state.EucliBoxIssue = "LOCAL_BOX_START_FAILED: 受托业务端连接已断开"
		s.connectionState = &state
	}
	s.connectionMu.Unlock()
	s.signalConnectionChanged()
}

func (s *service) publishLocalBoxState(state localBoxState) {
	if s.hub != nil {
		s.hub.broadcast(eventFrame{Type: "event", Name: directEventLocalBoxState, Payload: state})
	}
}

func (s *service) shutdownLocalBox(ctx context.Context) error {
	if s.localBox == nil {
		return nil
	}
	_, err := s.localBox.stop(ctx)
	return err
}

func (s *service) setShutdown(fn func()) {
	s.shutdownMu.Lock()
	s.shutdown = fn
	s.shutdownMu.Unlock()
}

func (s *service) requestShutdown() {
	s.shutdownMu.RLock()
	shutdown := s.shutdown
	s.shutdownMu.RUnlock()
	if shutdown != nil {
		go shutdown()
	}
}

func (s *service) refreshReleaseChecks(ctx context.Context, kind string) (types.ReleaseCheckSnapshot, error) {
	kind = strings.TrimSpace(kind)
	if kind != "" && kind != types.ReleaseArtifactKindBox && kind != types.ReleaseArtifactKindTool && kind != types.ReleaseArtifactKindPlugin {
		return failedReleaseCheckSnapshot(s.releaseCheckSnapshot(), fmt.Errorf("不支持的刷新分类 %q", kind)), nil
	}
	state := s.connectionSnapshot()
	if (state == nil || !state.EucliBoxReachable) && s.releaseChecker == nil {
		return s.releaseCheckSnapshot(), nil
	}

	s.releaseCheckMu.Lock()
	if s.releaseCheckRunning {
		snapshot := cloneReleaseCheckSnapshot(s.releaseChecks)
		s.releaseCheckMu.Unlock()
		return snapshot, nil
	}
	previous := cloneReleaseCheckSnapshot(s.releaseChecks)
	s.releaseCheckRunning = true
	s.releaseChecks = releasecheck.CheckingSnapshot(previous, time.Now().UTC())
	s.releaseCheckMu.Unlock()
	defer func() {
		s.releaseCheckMu.Lock()
		s.releaseCheckRunning = false
		s.releaseCheckMu.Unlock()
	}()

	if state != nil && state.EucliBoxReachable {
		body := mustJSON(map[string]any{"kind": kind})
		raw, err := s.eb.request(ctx, ebRequest{Method: "POST", Path: "/api/release-checks/refresh", Query: nil, Body: body, Timeout: 30000})
		if err != nil {
			snapshot := failedReleaseCheckSnapshot(previous, err)
			s.storeReleaseCheckSnapshot(snapshot)
			return snapshot, nil
		}
		snapshot, err := decodeReleaseCheckSnapshot(raw)
		if err != nil {
			snapshot := failedReleaseCheckSnapshot(previous, err)
			s.storeReleaseCheckSnapshot(snapshot)
			return snapshot, nil
		}
		snapshot = preservePreviousReleaseResults(previous, snapshot)
		s.storeReleaseCheckSnapshot(snapshot)
		return snapshot, nil
	}
	if kind != "" && kind != types.ReleaseArtifactKindBox {
		snapshot := failedReleaseCheckSnapshot(previous, fmt.Errorf("工具和插件刷新需要先连接业务端"))
		s.storeReleaseCheckSnapshot(snapshot)
		return snapshot, nil
	}
	snapshot := s.checkBoxOfficialSource(ctx)
	snapshot = preservePreviousReleaseResults(previous, snapshot)
	s.storeReleaseCheckSnapshot(snapshot)
	return snapshot, nil
}

func (s *service) checkBoxOfficialSource(ctx context.Context) types.ReleaseCheckSnapshot {
	if s.releaseChecker == nil {
		return releasecheck.PendingSnapshot()
	}
	return s.releaseChecker.CheckOnly(ctx, nil, "", []types.ReleaseArtifactIdentity{{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox}})
}

func (s *service) releaseCheckSnapshot() types.ReleaseCheckSnapshot {
	s.releaseCheckMu.RLock()
	defer s.releaseCheckMu.RUnlock()
	return cloneReleaseCheckSnapshot(s.releaseChecks)
}

func (s *service) storeReleaseCheckSnapshot(snapshot types.ReleaseCheckSnapshot) {
	s.releaseCheckMu.Lock()
	s.releaseChecks = cloneReleaseCheckSnapshot(snapshot)
	s.releaseCheckMu.Unlock()
}

func cloneReleaseCheckSnapshot(snapshot types.ReleaseCheckSnapshot) types.ReleaseCheckSnapshot {
	result := snapshot
	result.Results = append([]types.ReleaseCheckResult(nil), snapshot.Results...)
	for index := range result.Results {
		result.Results[index].AffectedArtifacts = append([]types.ReleaseArtifactIdentity(nil), snapshot.Results[index].AffectedArtifacts...)
	}
	return result
}

func failedReleaseCheckSnapshot(previous types.ReleaseCheckSnapshot, err error) types.ReleaseCheckSnapshot {
	now := time.Now().UTC()
	return types.ReleaseCheckSnapshot{
		Status:        types.ReleaseCheckStatusFailed,
		StartedAt:     now,
		CheckedAt:     now,
		Results:       append([]types.ReleaseCheckResult(nil), previous.Results...),
		FailureReason: strings.TrimSpace(err.Error()),
	}
}

func preservePreviousReleaseResults(previous types.ReleaseCheckSnapshot, current types.ReleaseCheckSnapshot) types.ReleaseCheckSnapshot {
	if current.Status != types.ReleaseCheckStatusFailed || previous.Status != types.ReleaseCheckStatusCompleted || !allReleaseResultsFailed(current.Results) {
		return current
	}
	current.Results = append([]types.ReleaseCheckResult(nil), previous.Results...)
	return current
}

func allReleaseResultsFailed(results []types.ReleaseCheckResult) bool {
	if len(results) == 0 {
		return true
	}
	for _, result := range results {
		if result.Status != types.ReleaseCheckStatusFailed {
			return false
		}
	}
	return true
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

// setBoxSourceKind 切换业务端安装来源：先同步业务端 /api/install-source，
// 成功后再持久化客户端配置，保证三层来源一致（不做静默不一致残留）。
func (s *service) setBoxSourceKind(ctx context.Context, value any) (map[string]any, error) {
	kind, ok := value.(string)
	if !ok {
		return nil, newError("BAD_REQUEST", "boxSourceKind 必须是来源类别")
	}
	kind = strings.TrimSpace(kind)
	if kind != string(localBoxSourceOfficial) && kind != string(localBoxSourceDevelopment) {
		return nil, newError("BAD_REQUEST", "boxSourceKind 只能是 official 或 development")
	}
	if !devBoxSourceEnabled() {
		return nil, newError("FORBIDDEN", "正式模式不允许切换业务端安装来源")
	}
	raw, err := s.eb.request(ctx, ebRequest{Method: "PUT", Path: "/api/install-source", Body: mustJSON(map[string]any{"kind": kind}), Timeout: 15000})
	if err != nil {
		return nil, fmt.Errorf("同步业务端安装来源失败：%w", err)
	}
	_ = raw
	if _, err := s.config.updateSetting("boxSourceKind", kind); err != nil {
		return nil, err
	}
	return s.config.getSettings()
}
