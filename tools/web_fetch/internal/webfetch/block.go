package webfetch

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// blockedStatuses 是判定为「被封锁」的状态码集合。
var blockedStatuses = map[int]struct{}{
	http.StatusUnauthorized:        {}, // 401
	http.StatusForbidden:           {}, // 403
	http.StatusProxyAuthRequired:   {}, // 407
	http.StatusTooManyRequests:     {}, // 429
	444:                            {}, // nginx 关闭连接
	http.StatusInternalServerError: {}, // 500
	http.StatusBadGateway:          {}, // 502
	http.StatusServiceUnavailable:  {}, // 503
	http.StatusGatewayTimeout:      {}, // 504
}

// isBlockedStatus 判断状态码是否属于被封锁；被封锁的响应会触发换身份重试。
func isBlockedStatus(status int) bool {
	_, ok := blockedStatuses[status]
	return ok
}

// retryDelay 计算下一次重试前的等待时长：按退避基数翻倍，并受上限约束。
// retryAfter 非零时优先使用（来自 Retry-After）。
func retryDelay(config Config, attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return capDelay(retryAfter, config.MaxDelayMs)
	}
	delay := time.Duration(config.RetryBaseDelayMs) * time.Millisecond
	for index := 1; index < attempt; index++ {
		delay *= 2
	}
	return capDelay(delay, config.MaxDelayMs)
}

func capDelay(delay time.Duration, maxDelayMs int) time.Duration {
	if maxDelayMs <= 0 {
		return delay
	}
	max := time.Duration(maxDelayMs) * time.Millisecond
	if delay > max {
		return max
	}
	return delay
}

// parseRetryAfter 解析 Retry-After 头，支持秒数与 HTTP 日期两种格式。
func parseRetryAfter(value string) (time.Duration, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(trimmed); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}
	if when, err := http.ParseTime(trimmed); err == nil {
		delay := time.Until(when)
		if delay < 0 {
			delay = 0
		}
		return delay, true
	}
	return 0, false
}

// sleep 等待指定时长，期间若上下文取消则立即返回错误。
func sleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("fetch cancelled: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
