package apperrors

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

// CodeProviderServiceFailed 是模型供应商上游返回非成功状态码时的错误码：
// 其 Details 携带 statusCode 与上游响应头，供重试与兜底判断使用。
const CodeProviderServiceFailed = "provider.service_failed"

// CodeProviderModelGroupExhausted 表示模型组的兜底链已逐个试完仍全部失败：
// 上层看到这个码就知道"换模型"这条路已经走过，不必再叠加自己的重试。
const CodeProviderModelGroupExhausted = "provider.model_group_exhausted"

// Walk 深度遍历错误负载树：自身 → Cause → 各 Causes；visit 返回 true 时停止并回报命中。
func Walk(payload *types.ErrorPayload, visit func(*types.ErrorPayload) bool) bool {
	if payload == nil {
		return false
	}
	if visit(payload) {
		return true
	}
	if Walk(payload.Cause, visit) {
		return true
	}
	for _, cause := range payload.Causes {
		if Walk(cause, visit) {
			return true
		}
	}
	return false
}

// HasErrorCode 判断负载树里是否出现过任一指定错误码。
func HasErrorCode(payload *types.ErrorPayload, codes ...string) bool {
	wanted := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		wanted[strings.TrimSpace(code)] = struct{}{}
	}
	return Walk(payload, func(item *types.ErrorPayload) bool {
		_, ok := wanted[strings.TrimSpace(item.Code)]
		return ok
	})
}

// ProviderStatusCode 取上游 HTTP 状态码：只认 provider.service_failed 节点自带的 details.statusCode。
func ProviderStatusCode(payload *types.ErrorPayload) (int, bool) {
	var status int
	found := Walk(payload, func(item *types.ErrorPayload) bool {
		if strings.TrimSpace(item.Code) != CodeProviderServiceFailed {
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

// ProviderRetryAfter 取上游给出的 Retry-After 等待时长；不做上限截断，由调用方按自己的策略封顶。
func ProviderRetryAfter(payload *types.ErrorPayload) (time.Duration, bool) {
	var delay time.Duration
	found := Walk(payload, func(item *types.ErrorPayload) bool {
		if strings.TrimSpace(item.Code) != CodeProviderServiceFailed {
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
		for key, item := range typed {
			headers[key] = []string{item}
		}
		return headers, true
	case map[string]any:
		headers := make(map[string][]string, len(typed))
		for key, item := range typed {
			values := stringsFromAny(item)
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
		if milliseconds {
			return time.Duration(parsed * float64(time.Millisecond)), true
		}
		return time.Duration(parsed * float64(time.Second)), true
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
	return delay, true
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
