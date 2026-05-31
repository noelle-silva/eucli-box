package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ebClient struct {
	config *configStore
	http   *http.Client
}

func newEBClient(config *configStore) *ebClient {
	return &ebClient{config: config, http: &http.Client{Timeout: 120 * time.Second}}
}

type ebRequest struct {
	Method  string          `json:"method"`
	Path    string          `json:"path"`
	Query   json.RawMessage `json:"query,omitempty"`
	Body    json.RawMessage `json:"body,omitempty"`
	Timeout int             `json:"timeoutMs,omitempty"`
}

func (c *ebClient) request(ctx context.Context, req ebRequest) (any, error) {
	cfg, err := c.config.requireConfigured()
	if err != nil {
		return nil, err
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	path := strings.TrimSpace(req.Path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return nil, newError("BAD_REQUEST", "e-b request path must start with /")
	}
	if strings.HasPrefix(path, "/ws/") {
		return nil, newError("BAD_REQUEST", "websocket paths are not valid HTTP request targets")
	}

	target, err := url.Parse(cfg.EucliBoxURL + path)
	if err != nil {
		return nil, err
	}
	if len(req.Query) > 0 && string(req.Query) != "null" {
		queryValues := target.Query()
		var query map[string]any
		if err := json.Unmarshal(req.Query, &query); err != nil {
			return nil, newError("BAD_REQUEST", "query must be an object")
		}
		for key, raw := range query {
			if key == "" || raw == nil {
				continue
			}
			queryValues.Set(key, fmt.Sprint(raw))
		}
		target.RawQuery = queryValues.Encode()
	}

	var body io.Reader
	if len(req.Body) > 0 && string(req.Body) != "null" {
		body = bytes.NewReader(req.Body)
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.Timeout)*time.Millisecond)
		defer cancel()
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if cfg.EucliBoxKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.EucliBoxKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newError("EB_REQUEST_FAILED", responseMessage(payload, resp.Status))
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return map[string]any{}, nil
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}
	if envelope, ok := decoded.(map[string]any); ok {
		if data, exists := envelope["data"]; exists {
			return data, nil
		}
	}
	return decoded, nil
}

func responseMessage(payload []byte, status string) string {
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err == nil {
		if errBox, ok := decoded["error"].(map[string]any); ok {
			if msg := strings.TrimSpace(fmt.Sprint(errBox["message"])); msg != "" {
				return msg
			}
		}
	}
	text := strings.TrimSpace(string(payload))
	if text != "" {
		return text
	}
	return status
}
