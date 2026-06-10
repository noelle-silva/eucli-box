package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

const (
	modelRetryMaxAttempts        = 3
	modelStreamRetryMaxAttempts  = 5
	modelRetryInitialDelay       = 2 * time.Second
	modelRetryMaxDelay           = 30 * time.Second
	modelRetryTimerMaxDelay      = time.Duration(1<<63 - 1)
	modelRetryStatusServiceBusy  = "模型服务暂时繁忙"
	modelRetryStatusRateLimited  = "模型服务要求稍后再试"
	modelRetryStatusNetworkIssue = "模型请求连接不稳定"
)

type modelRetryDecision struct {
	Retryable bool
	Delay     time.Duration
	Message   string
}

func modelRetryLimit(stream bool) int {
	if stream {
		return modelStreamRetryMaxAttempts
	}
	return modelRetryMaxAttempts
}

func modelRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := modelRetryInitialDelay
	for i := 1; i < attempt; i++ {
		if delay >= modelRetryMaxDelay/2 {
			return modelRetryMaxDelay
		}
		delay *= 2
	}
	if delay > modelRetryMaxDelay {
		return modelRetryMaxDelay
	}
	return delay
}

func modelRetryDecisionForError(err error, attempt int) modelRetryDecision {
	if err == nil || errors.Is(err, context.Canceled) {
		return modelRetryDecision{}
	}
	payload := apperrors.BuildErrorPayload(err)
	if payload == nil {
		return modelRetryDecision{}
	}
	if status, ok := firstProviderStatusCode(payload); ok {
		if status == 408 || status == 409 || status == 429 || status >= 500 {
			message := modelRetryStatusServiceBusy
			if status == 429 {
				message = modelRetryStatusRateLimited
			}
			if retryAfter, hasRetryAfter := firstRetryAfter(payload); hasRetryAfter {
				return modelRetryDecision{Retryable: true, Delay: retryAfter, Message: message}
			}
			return modelRetryDecision{Retryable: true, Delay: modelRetryDelay(attempt), Message: message}
		}
		return modelRetryDecision{}
	}
	if hasRetryableNetworkCode(payload) {
		return modelRetryDecision{Retryable: true, Delay: modelRetryDelay(attempt), Message: modelRetryStatusNetworkIssue}
	}
	return modelRetryDecision{}
}

func hasRetryableNetworkCode(payload *types.ErrorPayload) bool {
	return walkErrorPayload(payload, func(item *types.ErrorPayload) bool {
		switch strings.TrimSpace(item.Code) {
		case "network.timeout", "network.connection_failed", "network.connection_lost", "network.request_failed":
			return true
		default:
			return false
		}
	})
}

func firstProviderStatusCode(payload *types.ErrorPayload) (int, bool) {
	var status int
	found := walkErrorPayload(payload, func(item *types.ErrorPayload) bool {
		if strings.TrimSpace(item.Code) != "provider.service_failed" {
			return false
		}
		value, ok := statusCodeFromDetails(item.Details)
		if !ok {
			return false
		}
		status = value
		return true
	})
	return status, found
}

func firstRetryAfter(payload *types.ErrorPayload) (time.Duration, bool) {
	var delay time.Duration
	found := walkErrorPayload(payload, func(item *types.ErrorPayload) bool {
		if strings.TrimSpace(item.Code) != "provider.service_failed" {
			return false
		}
		value, ok := retryAfterFromDetails(item.Details)
		if !ok {
			return false
		}
		delay = value
		return true
	})
	return delay, found
}

func walkErrorPayload(payload *types.ErrorPayload, visit func(*types.ErrorPayload) bool) bool {
	if payload == nil {
		return false
	}
	if visit(payload) {
		return true
	}
	if walkErrorPayload(payload.Cause, visit) {
		return true
	}
	for _, cause := range payload.Causes {
		if walkErrorPayload(cause, visit) {
			return true
		}
	}
	return false
}

func statusCodeFromDetails(details any) (int, bool) {
	data, ok := details.(map[string]any)
	if !ok {
		return 0, false
	}
	return intFromAny(data["statusCode"])
}

func retryAfterFromDetails(details any) (time.Duration, bool) {
	data, ok := details.(map[string]any)
	if !ok {
		return 0, false
	}
	headers, ok := headersFromAny(data["headers"])
	if !ok {
		return 0, false
	}
	if delay, ok := retryAfterHeader(headers, "retry-after-ms"); ok {
		return delay, true
	}
	return retryAfterHeader(headers, "retry-after")
}

func headersFromAny(value any) (map[string][]string, bool) {
	switch typed := value.(type) {
	case map[string][]string:
		return typed, true
	case map[string]string:
		headers := make(map[string][]string, len(typed))
		for key, value := range typed {
			headers[key] = []string{value}
		}
		return headers, true
	case map[string]any:
		headers := make(map[string][]string, len(typed))
		for key, value := range typed {
			values := stringsFromAny(value)
			if len(values) > 0 {
				headers[key] = values
			}
		}
		return headers, len(headers) > 0
	default:
		return nil, false
	}
}

func stringsFromAny(value any) []string {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	case []string:
		return typed
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				values = append(values, text)
			}
		}
		return values
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return nil
		}
		return []string{text}
	}
}

func retryAfterHeader(headers map[string][]string, name string) (time.Duration, bool) {
	for key, values := range headers {
		if !strings.EqualFold(key, name) {
			continue
		}
		for _, value := range values {
			if delay, ok := parseRetryAfter(value, strings.EqualFold(name, "retry-after-ms")); ok {
				return delay, true
			}
		}
	}
	return 0, false
}

func parseRetryAfter(value string, milliseconds bool) (time.Duration, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, false
	}
	if parsed, err := strconv.ParseFloat(text, 64); err == nil && parsed >= 0 {
		delay := time.Duration(parsed * float64(time.Second))
		if milliseconds {
			delay = time.Duration(parsed * float64(time.Millisecond))
		}
		return capModelRetryDelay(delay), true
	}
	if milliseconds {
		return 0, false
	}
	when, err := time.Parse(time.RFC1123, text)
	if err != nil {
		return 0, false
	}
	delay := time.Until(when)
	if delay <= 0 {
		return 0, false
	}
	return capModelRetryDelay(delay), true
}

func capModelRetryDelay(delay time.Duration) time.Duration {
	if delay < 0 {
		return 0
	}
	if delay > modelRetryTimerMaxDelay {
		return modelRetryTimerMaxDelay
	}
	return delay
}

func intFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		return parsed, err == nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func newRunRetryInfo(attempt int, maxAttempts int, delay time.Duration, message string, failures []types.RunRetryFailure) *types.RunRetryInfo {
	return &types.RunRetryInfo{
		Attempt:     attempt,
		MaxAttempts: maxAttempts,
		RetryAt:     time.Now().UTC().Add(delay),
		DelayMs:     int(delay / time.Millisecond),
		Message:     strings.TrimSpace(message),
		Failures:    cloneRunRetryFailures(failures),
	}
}

func newRunRetryFailure(attempt int, err error) types.RunRetryFailure {
	return types.RunRetryFailure{Attempt: attempt, Error: errorPayloadFromError(err, ""), OccurredAt: time.Now().UTC()}
}

func cloneRunRetryFailures(failures []types.RunRetryFailure) []types.RunRetryFailure {
	if len(failures) == 0 {
		return nil
	}
	out := make([]types.RunRetryFailure, 0, len(failures))
	for _, failure := range failures {
		out = append(out, types.RunRetryFailure{Attempt: failure.Attempt, Error: cloneErrorPayload(failure.Error), OccurredAt: failure.OccurredAt})
	}
	return out
}

func retryMessage(attempt int, maxAttempts int, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "模型请求失败，正在自动重试"
	}
	return fmt.Sprintf("%s（第 %d/%d 次）", message, attempt, maxAttempts)
}
