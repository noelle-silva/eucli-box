package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
)

const (
	splitMetaKey           = "meta/index"
	groupSpeakerUserPrefix = "用户"
	chatDefaultBranchID    = "main"
)

func (svc *service) buildOpenAIChatReqFromStorage(job map[string]any) (aiHTTPRequest, error) {
	return aiHTTPRequest{}, errBoxOnlyAIRuntime
}

func (svc *service) buildOpenAIGroupChatReqFromStorage(job map[string]any) (aiHTTPRequest, error) {
	return aiHTTPRequest{}, errBoxOnlyAIRuntime
}

func (svc *service) loadObject(key string) (map[string]any, error) {
	value, err := svc.storageGetByKey(key)
	if err != nil {
		return nil, err
	}
	obj, _ := value.(map[string]any)
	return obj, nil
}

func (svc *service) buildBoxRunRequestFromStorage(job map[string]any) (boxRunRequest, error) {
	roleID := strings.TrimSpace(asString(job["roleId"]))
	if roleID == "" {
		return boxRunRequest{}, errors.New("job 缺少 roleId")
	}
	message := strings.TrimSpace(asString(job["message"]))
	if message == "" {
		return boxRunRequest{}, errors.New("job 缺少 message；客户端不得从本地会话历史构造运行请求")
	}
	sessionID := strings.TrimSpace(asString(job["sessionId"]))
	if sessionID == "" {
		session, err := svc.box.createSession(context.Background(), roleID)
		if err != nil {
			return boxRunRequest{}, fmt.Errorf("create eucli-box session failed: %w", err)
		}
		sessionID = strings.TrimSpace(asString(session["id"]))
	}
	if sessionID == "" {
		return boxRunRequest{}, errors.New("eucli-box session id is required")
	}
	return boxRunRequest{RoleID: roleID, SessionID: sessionID, Message: message}, nil
}

func (svc *service) ensureBoxSessionForChat(ctx context.Context, roleID string, chatID string, currentSessionID string) (string, error) {
	sessionID := strings.TrimSpace(currentSessionID)
	if sessionID != "" {
		if _, err := svc.box.getSession(ctx, sessionID); err == nil {
			return sessionID, nil
		} else if !isHTTPNotFound(err) {
			return "", fmt.Errorf("verify eucli-box session failed: %w", err)
		}
	}

	newSession, err := svc.box.createSession(ctx, roleID)
	if err != nil {
		return "", fmt.Errorf("create eucli-box session failed: %w", err)
	}
	sessionID = strings.TrimSpace(asString(newSession["id"]))
	if sessionID == "" {
		return "", errors.New("create eucli-box session failed: empty session id")
	}
	return sessionID, nil
}

func buildHistory(chat map[string]any, job map[string]any) []map[string]any {
	msgs0 := normalizeObjectList(chat["messages"])
	branchID := strings.TrimSpace(asString(job["branchId"]))
	var history []map[string]any
	if branchID != "" {
		wantBranchID := normalizeBranchID(branchID)
		_ = wantBranchID
		byID := map[string]map[string]any{}
		for _, msg := range msgs0 {
			id := strings.TrimSpace(asString(msg["id"]))
			if id != "" {
				byID[id] = msg
			}
		}
		assistantMid := strings.TrimSpace(asString(job["assistantMid"]))
		assistantMsg := byID[assistantMid]
		tailMid := ""
		if assistantMsg != nil {
			tailMid = strings.TrimSpace(asString(assistantMsg["parentMid"]))
		}
		if tailMid == "" {
			for i := len(msgs0) - 1; i >= 0; i-- {
				if asString(msgs0[i]["role"]) == "user" {
					tailMid = strings.TrimSpace(asString(msgs0[i]["id"]))
					break
				}
			}
		}
		seen := map[string]bool{}
		for tailMid != "" && !seen[tailMid] {
			seen[tailMid] = true
			msg := byID[tailMid]
			if msg == nil {
				break
			}
			if !(asString(msg["role"]) == "assistant" && truthy(msg["pending"])) {
				history = append(history, msg)
			}
			tailMid = strings.TrimSpace(asString(msg["parentMid"]))
		}
		for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
			history[i], history[j] = history[j], history[i]
		}
	} else {
		base := msgs0
		cutoffMid := strings.TrimSpace(asString(job["cutoffMid"]))
		if cutoffMid != "" {
			for i, msg := range msgs0 {
				if asString(msg["id"]) == cutoffMid {
					base = msgs0[:i]
					break
				}
			}
		}
		for _, msg := range base {
			if asString(msg["role"]) == "assistant" && truthy(msg["pending"]) {
				continue
			}
			history = append(history, msg)
		}
	}
	return limitHistoryGo(history, 40)
}

func buildChatCompletionsRequest(baseURL, apiKey, modelID string, messages []map[string]any, temperature float64, stream bool) aiHTTPRequest {
	body, _ := json.Marshal(map[string]any{"model": modelID, "messages": messages, "temperature": temperature, "stream": stream})
	timeoutMs := int64(120000)
	if stream {
		timeoutMs = 15 * 60 * 1000
	}
	return aiHTTPRequest{
		Method: "POST",
		URL:    trimSlashGo(baseURL) + "/chat/completions",
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + apiKey,
		},
		Body:      string(body),
		TimeoutMs: timeoutMs,
	}
}

func buildUserTextForOpenAIGo(msg map[string]any) string {
	base := strings.TrimSpace(asString(msg["content"]))
	attachments := normalizeObjectList(msg["attachments"])
	if len(attachments) == 0 {
		return base
	}
	if len(attachments) == 1 {
		name := asString(attachments[0]["name"])
		if name != "" && base == "附件："+name {
			base = ""
		}
	}
	blocks := make([]string, 0, len(attachments))
	for _, att := range attachments {
		name := asString(att["name"])
		if strings.TrimSpace(name) == "" {
			name = "文件"
		}
		fullLen := clampInt64(asInt64(att["fullLen"], 0), 0, 10000000)
		sendLen := clampInt64(asInt64(att["sendLen"], 0), 0, fullLen)
		pct := clampInt64(asInt64(att["sendPct"], 100), 0, 100)
		lang := asString(att["lang"])
		if lang == "" {
			if asString(att["kind"]) == "md" {
				lang = "markdown"
			} else {
				lang = "text"
			}
		}
		raw := strings.TrimSpace(asString(att["text"]))
		if raw == "" {
			continue
		}
		snippet := strings.ReplaceAll(raw, "```", "``\u200b`")
		blocks = append(blocks, fmt.Sprintf("附件：%s（发送 %d%%：%d/%d 字符）\n```%s\n%s\n```", name, pct, sendLen, fullLen, lang, snippet))
		if len(blocks) >= 20 {
			break
		}
	}
	extra := strings.TrimSpace(strings.Join(blocks, "\n\n"))
	if extra == "" {
		return base
	}
	if base == "" {
		return extra
	}
	return strings.TrimSpace(base + "\n\n" + extra)
}

func normalizeChatModelOverrideGo(chat map[string]any) map[string]any {
	override := asMap(chat["modelOverride"])
	providerID := strings.TrimSpace(asString(override["providerId"]))
	modelID := strings.TrimSpace(asString(override["modelId"]))
	if providerID == "" || modelID == "" {
		return nil
	}
	return map[string]any{"providerId": providerID, "modelId": modelID}
}

func providerByID(providers []any, providerID string) map[string]any {
	pid := strings.TrimSpace(providerID)
	for _, item := range providers {
		provider := asMap(item)
		if strings.TrimSpace(asString(provider["id"])) == pid {
			return provider
		}
	}
	return nil
}

func normalizeObjectList(raw any) []map[string]any {
	list := asSlice(raw)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		obj := asMap(item)
		if obj != nil {
			out = append(out, obj)
		}
	}
	return out
}

func limitHistoryGo(messages []map[string]any, maxTurns int) []map[string]any {
	items := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		role := asString(msg["role"])
		if role == "user" || role == "assistant" {
			items = append(items, msg)
		}
	}
	if maxTurns <= 0 || len(items) <= maxTurns {
		return items
	}
	return items[len(items)-maxTurns:]
}

func normalizeStringIDs(raw any) []string {
	list := asSlice(raw)
	items := make([]string, 0, len(list))
	for _, item := range list {
		value := strings.TrimSpace(asString(item))
		if value != "" {
			items = append(items, value)
		}
	}
	sort.Strings(items)
	return items
}

func normImagePathsGo(raw any, maxCount int) []string {
	list := asSlice(raw)
	items := make([]string, 0, len(list))
	for _, item := range list {
		value := strings.TrimSpace(asString(item))
		if value == "" || len(value) > 4096 {
			continue
		}
		items = append(items, value)
		if len(items) >= maxCount {
			break
		}
	}
	return items
}

func normalizeBranchID(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return chatDefaultBranchID
	}
	if len(value) > 60 {
		value = strings.TrimSpace(value[:60])
	}
	b := strings.Builder{}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	value = b.String()
	if value == "" {
		return chatDefaultBranchID
	}
	return value
}

func trimSlashGo(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func isHTTPBaseURLGo(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func clampTempGo(raw any) float64 {
	n := asFloat64(raw, 0.7)
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0.7
	}
	if n < 0 {
		return 0
	}
	if n > 2 {
		return 2
	}
	return n
}

func asMap(raw any) map[string]any {
	value, _ := raw.(map[string]any)
	return value
}

func asSlice(raw any) []any {
	value, _ := raw.([]any)
	return value
}

func asFloat64(raw any, fallback float64) float64 {
	switch v := raw.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, err := v.Float64()
		if err == nil {
			return f
		}
	}
	return fallback
}

func clampInt64(value int64, minValue int64, maxValue int64) int64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
