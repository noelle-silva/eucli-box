package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const defaultEucliBoxURL = "http://127.0.0.1:8765"

type storageReader interface {
	Get(key string) (any, error)
}

type boxConnectionConfig struct {
	URL string `json:"url"`
	Key string `json:"key"`
}

type boxClient struct {
	httpBase string
	wsBase   string
	key      string
	http     *http.Client
	events   *boxEventHub
}

type boxRunRequest struct {
	RoleID      string `json:"roleId"`
	SessionID   string `json:"sessionId"`
	Message     string `json:"message"`
	LocalRoleID string `json:"-"`
	LocalChatID string `json:"-"`
}

type boxRunState struct {
	ID        string `json:"id"`
	RoleID    string `json:"roleId"`
	SessionID string `json:"sessionId"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
}

type boxRunResult struct {
	Status string
	Text   string
	Reason string
}

type boxRunEvent struct {
	ID      string          `json:"id"`
	RunID   string          `json:"runId"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type boxDataResponse struct {
	Data json.RawMessage `json:"data"`
}

type boxErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		System  string `json:"system"`
	} `json:"error"`
}

type boxRoleSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

type boxProviderSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
}

func newBoxClient(storage storageReader, initialConfig boxConnectionConfig) *boxClient {
	c := &boxClient{http: &http.Client{Timeout: 30 * time.Second}}
	c.applyConfig(initialConfig)
	c.events = newBoxEventHub(c.wsBase+"/ws/events", c.key)
	return c
}

func loadBoxConnectionFromStorage(storage storageReader) boxConnectionConfig {
	raw, err := storage.Get("box/connection")
	if err != nil || raw == nil {
		return boxConnectionConfig{
			URL: strings.TrimSpace(os.Getenv("EUCLI_BOX_URL")),
		}
	}
	configMap, ok := raw.(map[string]any)
	if !ok {
		return boxConnectionConfig{
			URL: strings.TrimSpace(os.Getenv("EUCLI_BOX_URL")),
		}
	}
	return boxConnectionConfig{
		URL: strings.TrimSpace(asString(configMap["url"])),
		Key: strings.TrimSpace(asString(configMap["key"])),
	}
}

func (c *boxClient) applyConfig(config boxConnectionConfig) {
	httpBase := strings.TrimSpace(config.URL)
	if httpBase == "" {
		httpBase = defaultEucliBoxURL
	}
	httpBase = strings.TrimRight(httpBase, "/")
	c.httpBase = httpBase
	c.wsBase = websocketBaseURL(httpBase)
	c.key = strings.TrimSpace(config.Key)
}

func (c *boxClient) reloadConnection(storage storageReader) error {
	config := loadBoxConnectionFromStorage(storage)
	c.applyConfig(config)
	if c.events != nil {
		c.events.updateConnection(c.wsBase+"/ws/events", c.key)
	}
	return nil
}

func websocketBaseURL(httpBase string) string {
	u, err := url.Parse(httpBase)
	if err != nil {
		return "ws://127.0.0.1:8765"
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	return strings.TrimRight(u.String(), "/")
}

func (c *boxClient) Close() {
	if c == nil || c.events == nil {
		return
	}
	c.events.Close()
}

func (c *boxClient) health(ctx context.Context) map[string]any {
	if c == nil {
		return map[string]any{"status": "missing"}
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	var roles []map[string]any
	if err := c.getJSON(ctx, "/api/roles", &roles); err != nil {
		return map[string]any{"status": "unreachable", "url": c.httpBase, "error": err.Error()}
	}
	return map[string]any{"status": "ok", "url": c.httpBase, "roles": len(roles)}
}

func (c *boxClient) listRoles(ctx context.Context) ([]map[string]any, error) {
	var summaries []boxRoleSummary
	if err := c.getJSON(ctx, "/api/roles", &summaries); err != nil {
		return nil, err
	}
	roles := make([]map[string]any, 0, len(summaries))
	var errs []string
	for _, summary := range summaries {
		roleID := strings.TrimSpace(summary.ID)
		if roleID == "" {
			continue
		}
		var role map[string]any
		if err := c.getJSON(ctx, "/api/roles/"+url.PathEscape(roleID), &role); err != nil {
			errs = append(errs, fmt.Sprintf("role %s: %v", roleID, err))
			continue
		}
		roles = append(roles, role)
	}
	if len(roles) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("listRoles all failed: %s", strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		log.Printf("boxClient.listRoles partial failures: %s", strings.Join(errs, "; "))
	}
	return roles, nil
}

func (c *boxClient) listProviders(ctx context.Context) ([]map[string]any, error) {
	var summaries []boxProviderSummary
	if err := c.getJSON(ctx, "/api/providers", &summaries); err != nil {
		return nil, err
	}
	providers := make([]map[string]any, 0, len(summaries))
	var errs []string
	for _, summary := range summaries {
		providerID := strings.TrimSpace(summary.ID)
		if providerID == "" {
			continue
		}
		var provider map[string]any
		if err := c.getJSON(ctx, "/api/providers/"+url.PathEscape(providerID), &provider); err != nil {
			errs = append(errs, fmt.Sprintf("provider %s: %v", providerID, err))
			continue
		}
		providers = append(providers, provider)
	}
	if len(providers) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("listProviders all failed: %s", strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		log.Printf("boxClient.listProviders partial failures: %s", strings.Join(errs, "; "))
	}
	return providers, nil
}

func (c *boxClient) putRole(ctx context.Context, role map[string]any) error {
	roleID := strings.TrimSpace(asString(role["id"]))
	if roleID == "" {
		return errors.New("role id is required")
	}
	modelRef := asMap(role["modelRef"])
	if modelRef == nil {
		return errors.New("modelRef is required")
	}
	providerID := strings.TrimSpace(asString(modelRef["providerId"]))
	if providerID == "" {
		return errors.New("modelRef.providerId is required")
	}
	modelID := strings.TrimSpace(asString(modelRef["modelId"]))
	if modelID == "" {
		return errors.New("modelRef.modelId is required")
	}

	body := map[string]any{
		"id":          roleID,
		"name":        strings.TrimSpace(asString(role["name"])),
		"avatar":      strings.TrimSpace(asString(role["avatar"])),
		"description": strings.TrimSpace(asString(role["systemPrompt"])),
		"prompts": []map[string]any{{
			"id":      rolePromptID(roleID),
			"content": strings.TrimSpace(asString(role["systemPrompt"])),
		}},
		"modelConfig": map[string]any{
			"providerId":  providerID,
			"model":       modelID,
			"temperature": asFloat64(role["temperature"], 0.7),
		},
		"createdAt": msToISO8601(asInt64(role["createdAt"], nowMs())),
		"updatedAt": msToISO8601(nowMs()),
	}

	var existing map[string]any
	getErr := c.getJSON(ctx, "/api/roles/"+url.PathEscape(roleID), &existing)
	if getErr == nil {
		if tp := asMap(existing["toolPermissions"]); tp != nil {
			body["toolPermissions"] = tp
		} else {
			body["toolPermissions"] = map[string]any{"mode": "whitelist", "list": []any{}}
		}
	} else if isHTTPNotFound(getErr) {
		body["toolPermissions"] = map[string]any{"mode": "whitelist", "list": []any{}}
	} else {
		return getErr
	}

	return c.postJSON(ctx, "/api/roles", body, nil)
}

func (c *boxClient) putProvider(ctx context.Context, provider map[string]any) error {
	providerID := strings.TrimSpace(asString(provider["id"]))
	if providerID == "" {
		return errors.New("provider id is required")
	}

	body := map[string]any{
		"id":        providerID,
		"name":      strings.TrimSpace(asString(provider["name"])),
		"baseUrl":   strings.TrimSpace(asString(provider["baseUrl"])),
		"key":       strings.TrimSpace(asString(provider["apiKey"])),
		"protocol":  firstNonEmpty(asString(provider["protocol"]), "openai"),
		"models":    providerModelsToBox(asMap(provider["modelsCache"])),
		"createdAt": msToISO8601(asInt64(provider["createdAt"], nowMs())),
		"updatedAt": msToISO8601(nowMs()),
	}

	modelsCache := asMap(provider["modelsCache"])
	apiKey := strings.TrimSpace(asString(provider["apiKey"]))
	needPreserve := (modelsCache == nil || len(asSlice(modelsCache["items"])) == 0) || apiKey == ""

	if needPreserve {
		var existing map[string]any
		getErr := c.getJSON(ctx, "/api/providers/"+url.PathEscape(providerID), &existing)
		if getErr == nil {
			if modelsCache == nil || len(asSlice(modelsCache["items"])) == 0 {
				body["models"] = existing["models"]
			}
			if apiKey == "" {
				body["key"] = existing["key"]
			}
		} else if !isHTTPNotFound(getErr) {
			return getErr
		}
	}

	return c.postJSON(ctx, "/api/providers", body, nil)
}

func (c *boxClient) deleteRole(ctx context.Context, roleID string) error {
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return errors.New("role id is required")
	}
	return c.deleteJSON(ctx, "/api/roles/"+url.PathEscape(roleID))
}

func (c *boxClient) deleteProvider(ctx context.Context, providerID string) error {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return errors.New("provider id is required")
	}
	return c.deleteJSON(ctx, "/api/providers/"+url.PathEscape(providerID))
}

func (c *boxClient) listSessions(ctx context.Context, roleID string) ([]map[string]any, error) {
	var sessions []map[string]any
	if err := c.getJSON(ctx, "/api/sessions?roleId="+url.QueryEscape(roleID), &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (c *boxClient) createSession(ctx context.Context, roleID string) (map[string]any, error) {
	var session map[string]any
	if err := c.postJSON(ctx, "/api/sessions", map[string]any{"roleId": roleID}, &session); err != nil {
		return nil, err
	}
	return session, nil
}

func (c *boxClient) getSession(ctx context.Context, sessionID string) (map[string]any, error) {
	var session map[string]any
	if err := c.getJSON(ctx, "/api/sessions/"+url.PathEscape(sessionID), &session); err != nil {
		return nil, err
	}
	return session, nil
}

func (c *boxClient) deleteSession(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}
	return c.deleteJSON(ctx, "/api/sessions/"+url.PathEscape(sessionID))
}

func (c *boxClient) getRun(ctx context.Context, runID string) (map[string]any, error) {
	var run map[string]any
	if err := c.getJSON(ctx, "/api/runs/"+url.PathEscape(runID), &run); err != nil {
		return nil, err
	}
	return run, nil
}

func (c *boxClient) submitToolConfirmation(ctx context.Context, decisionID string, approved bool) error {
	body := map[string]any{
		"decisionId": decisionID,
		"approved":   approved,
	}
	return c.postJSON(ctx, "/api/tool-confirmations", body, nil)
}

func (c *boxClient) isBoxConnected() bool {
	if c == nil || c.events == nil {
		return false
	}
	return c.events.isConnected()
}

func rolePromptID(roleID string) string {
	id := strings.TrimSpace(roleID)
	if len(id) > 8 {
		id = id[:8]
	}
	return id + "-p0"
}

func msToISO8601(ms int64) string {
	if ms <= 0 {
		ms = nowMs()
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

func providerModelsToBox(modelsCache map[string]any) []map[string]any {
	items := asSlice(modelsCache["items"])
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		model := asMap(item)
		out = append(out, map[string]any{
			"id":   strings.TrimSpace(asString(model["id"])),
			"name": strings.TrimSpace(asString(model["name"])),
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (c *boxClient) runChat(ctx context.Context, request boxRunRequest, onDelta func(string), onSession func(string)) (boxRunResult, error) {
	if c == nil {
		return boxRunResult{}, errors.New("eucli-box client is not configured")
	}

	state, err := c.startRun(ctx, request)
	if err != nil {
		return boxRunResult{}, err
	}
	runID := strings.TrimSpace(state.ID)
	if runID == "" {
		return boxRunResult{}, errors.New("eucli-box returned empty run id")
	}
	if strings.TrimSpace(state.SessionID) != "" && onSession != nil {
		onSession(state.SessionID)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan boxRunResult, 1)

	var (
		mu   sync.Mutex
		text strings.Builder
	)

	unsubscribe := c.events.subscribe(runID, func(event boxRunEvent) {
		if sessionID := sessionIDFromPayload(event.Payload); sessionID != "" && onSession != nil {
			onSession(sessionID)
		}
		switch event.Type {
		case "model_output":
			content := contentFromPayload(event.Payload)
			if content == "" {
				return
			}
			mu.Lock()
			text.WriteString(content)
			mu.Unlock()
			if onDelta != nil {
				onDelta(content)
			}
		case "run_completed":
			mu.Lock()
			current := strings.TrimSpace(text.String())
			mu.Unlock()
			select {
			case done <- boxRunResult{Status: "succeeded", Text: current}:
			default:
			}
		case "run_failed":
			reason := reasonFromPayload(event.Payload)
			if reason == "" {
				reason = "eucli-box run failed"
			}
			mu.Lock()
			current := strings.TrimSpace(text.String())
			mu.Unlock()
			select {
			case done <- boxRunResult{Status: "failed", Text: current, Reason: reason}:
			default:
			}
		case "run_cancelled":
			mu.Lock()
			current := strings.TrimSpace(text.String())
			if current == "" {
				current = "（已停止）"
			}
			mu.Unlock()
			select {
			case done <- boxRunResult{Status: "canceled", Text: current}:
			default:
			}
		}
	})
	defer unsubscribe()

	select {
	case result := <-done:
		return result, nil
	case <-ctx.Done():
		if err := c.cancelRun(context.Background(), runID); err != nil {
			log.Printf("boxClient.runChat: cancel run %s failed: %v", runID, err)
		}
		mu.Lock()
		current := strings.TrimSpace(text.String())
		if current == "" {
			current = "（已停止）"
		}
		mu.Unlock()
		return boxRunResult{Status: "canceled", Text: current}, nil
	}
}

func (c *boxClient) startRun(ctx context.Context, request boxRunRequest) (boxRunState, error) {
	var state boxRunState
	if err := c.postJSON(ctx, "/api/runs", request, &state); err != nil {
		return boxRunState{}, err
	}
	return state, nil
}

func (c *boxClient) cancelRun(ctx context.Context, runID string) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	return c.postJSON(ctx, "/api/runs/"+url.PathEscape(runID)+"/cancel", map[string]any{}, nil)
}

func (c *boxClient) subscribe(runID string, onEvent func(boxRunEvent)) func() {
	return c.events.subscribe(runID, onEvent)
}

func (c *boxClient) setToolConfirmationCallback(cb func(boxRunEvent)) {
	if c == nil || c.events == nil {
		return
	}
	c.events.setToolConfirmationCallback(cb)
}

func (c *boxClient) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.httpBase+path, nil)
	if err != nil {
		return err
	}
	c.setAuthHeader(req)
	return c.do(req, out)
}

func (c *boxClient) postJSON(ctx context.Context, path string, in any, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.httpBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(req)
	return c.do(req, out)
}

func (c *boxClient) deleteJSON(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.httpBase+path, nil)
	if err != nil {
		return err
	}
	c.setAuthHeader(req)
	return c.do(req, nil)
}

func (c *boxClient) setAuthHeader(req *http.Request) {
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
}

type httpError struct {
	StatusCode int
	Message    string
}

func (e *httpError) Error() string { return e.Message }

func newHTTPError(statusCode int, payload []byte) error {
	return &httpError{StatusCode: statusCode, Message: boxErrorMessage(payload, statusCode)}
}

func isHTTPNotFound(err error) bool {
	var he *httpError
	return errors.As(err, &he) && he.StatusCode == http.StatusNotFound
}

func (c *boxClient) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newHTTPError(resp.StatusCode, payload)
	}
	if out == nil {
		return nil
	}
	var wrapper boxDataResponse
	if err := json.Unmarshal(payload, &wrapper); err != nil {
		return err
	}
	if len(wrapper.Data) == 0 {
		return nil
	}
	return json.Unmarshal(wrapper.Data, out)
}

func boxErrorMessage(payload []byte, status int) string {
	var response boxErrorResponse
	if err := json.Unmarshal(payload, &response); err == nil {
		message := strings.TrimSpace(response.Error.Message)
		if message != "" {
			return message
		}
	}
	text := strings.TrimSpace(string(payload))
	if text != "" {
		return text
	}
	return fmt.Sprintf("eucli-box HTTP %d", status)
}

func sessionIDFromPayload(payload json.RawMessage) string {
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return ""
	}
	return strings.TrimSpace(asString(obj["sessionId"]))
}

func contentFromPayload(payload json.RawMessage) string {
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return ""
	}
	return strings.TrimSpace(asString(obj["content"]))
}

func reasonFromPayload(payload json.RawMessage) string {
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(obj["reason"], obj["message"], obj["error"]))
}

func (svc *service) loadBoxSessionID(roleID string, chatID string) (string, error) {
	value, err := svc.storageGetByKey(boxSessionMapKey(roleID, chatID))
	if err != nil {
		return "", err
	}
	box, _ := value.(map[string]any)
	return strings.TrimSpace(asString(box["sessionId"])), nil
}

func (svc *service) boxRunChat(ctx context.Context, request boxRunRequest, onDelta func(string), onSession func(string)) (boxRunResult, error) {
	return svc.box.runChat(ctx, request, onDelta, onSession)
}

func (svc *service) saveBoxSessionID(roleID string, chatID string, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	return svc.storageSetByKey(boxSessionMapKey(roleID, chatID), map[string]any{"sessionId": sessionID, "updatedAt": nowMs()})
}

func boxSessionMapKey(roleID string, chatID string) string {
	return "box/session-map/" + boxStorageSegment(roleID) + "/" + boxStorageSegment(chatID)
}

func boxStorageSegment(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = "empty"
	}
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
