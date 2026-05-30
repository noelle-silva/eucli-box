package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	SessionID   string `json:"sessionId,omitempty"`
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
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
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
	for _, summary := range summaries {
		roleID := strings.TrimSpace(summary.ID)
		if roleID == "" {
			continue
		}
		var role map[string]any
		if err := c.getJSON(ctx, "/api/roles/"+url.PathEscape(roleID), &role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func (c *boxClient) listProviders(ctx context.Context) ([]map[string]any, error) {
	var summaries []boxProviderSummary
	if err := c.getJSON(ctx, "/api/providers", &summaries); err != nil {
		return nil, err
	}
	providers := make([]map[string]any, 0, len(summaries))
	for _, summary := range summaries {
		providerID := strings.TrimSpace(summary.ID)
		if providerID == "" {
			continue
		}
		var provider map[string]any
		if err := c.getJSON(ctx, "/api/providers/"+url.PathEscape(providerID), &provider); err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	return providers, nil
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
			if text.Len() > 0 {
				text.WriteString("\n\n")
			}
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
		_ = c.cancelRun(context.Background(), runID)
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

func (c *boxClient) setAuthHeader(req *http.Request) {
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
}

func (c *boxClient) do(req *http.Request, out any) error {
	if c.key != "" {
		req.Header.Set("X-Eucli-Key", c.key)
	}
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
		return errors.New(boxErrorMessage(payload, resp.StatusCode))
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
