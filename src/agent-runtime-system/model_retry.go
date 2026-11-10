package agentruntime

import (
	"context"
	"errors"
	"fmt"
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

// retryableNetworkCodes 是传输层值得再试一次的错误码。
var retryableNetworkCodes = []string{
	"network.timeout",
	"network.connection_failed",
	"network.connection_lost",
	"network.request_failed",
}

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
	// 兜底链已经把成员逐个试完：换模型这一步的成本已经付过，
	// 再叠加运行时自己的重试只会把上游调用量成倍放大。
	if apperrors.HasErrorCode(payload, apperrors.CodeProviderModelGroupExhausted) {
		return modelRetryDecision{}
	}
	if status, ok := apperrors.ProviderStatusCode(payload); ok {
		if status != 408 && status != 409 && status != 429 && status < 500 {
			return modelRetryDecision{}
		}
		message := modelRetryStatusServiceBusy
		if status == 429 {
			message = modelRetryStatusRateLimited
		}
		if retryAfter, ok := apperrors.ProviderRetryAfter(payload); ok {
			return modelRetryDecision{Retryable: true, Delay: capModelRetryDelay(retryAfter), Message: message}
		}
		return modelRetryDecision{Retryable: true, Delay: modelRetryDelay(attempt), Message: message}
	}
	if apperrors.HasErrorCode(payload, retryableNetworkCodes...) {
		return modelRetryDecision{Retryable: true, Delay: modelRetryDelay(attempt), Message: modelRetryStatusNetworkIssue}
	}
	return modelRetryDecision{}
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

func newRunRetryInfo(attempt int, maxAttempts int, delay time.Duration, message string, failure *types.ErrorPayload) *types.RunRetryInfo {
	return &types.RunRetryInfo{
		Attempt:     attempt,
		MaxAttempts: maxAttempts,
		RetryAt:     time.Now().UTC().Add(delay),
		DelayMs:     int(delay / time.Millisecond),
		Message:     strings.TrimSpace(message),
		Failure:     cloneErrorPayload(failure),
	}
}

func retryMessage(attempt int, maxAttempts int, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "模型请求失败，正在自动重试"
	}
	return fmt.Sprintf("%s（第 %d/%d 次）", message, attempt, maxAttempts)
}
