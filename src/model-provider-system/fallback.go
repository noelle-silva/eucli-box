package modelprovider

import (
	"context"
	"errors"
	"net/http"
	"strings"

	apperrors "eucli-box/pkg/errors"
)

// switchableNetworkCodes 是传输层里"换一个成员很可能就好了"的错误码。
var switchableNetworkCodes = []string{
	"network.timeout",
	"network.connection_failed",
	"network.connection_lost",
	"network.request_failed",
	"network.dns_failed",
	"network.tls_failed",
	"network.connection_refused",
}

// shouldSwitchMember 判断一次成员失败是否值得顺位换下一个成员。
// 换了可能就好的才换：链路不通、上游过载或限流、凭据失效、响应解析不出内容。
// 请求本身有问题（参数错、模型不存在）换谁都会一样错，直接失败更诚实。
func shouldSwitchMember(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	payload := apperrors.BuildErrorPayload(err)
	if payload == nil {
		return false
	}
	if status, ok := apperrors.ProviderStatusCode(payload); ok {
		switch status {
		case http.StatusUnauthorized,
			http.StatusForbidden,
			http.StatusRequestTimeout,
			http.StatusConflict,
			http.StatusTooManyRequests:
			return true
		}
		return status >= 500
	}
	return apperrors.HasErrorCode(payload, switchableNetworkCodes...) ||
		apperrors.HasErrorCode(payload, "provider.parse_failed", "provider.network_failed")
}

// failedMember 记录兜底链上一个成员的失败。
type failedMember struct {
	label string
	err   error
}

// aggregateMemberFailures 把整条兜底链的失败合成一个错误：
// 顶层说明"链已试完"，每个成员各自的失败挂成并列原因，便于一眼看出谁坏在哪。
func aggregateMemberFailures(failures []failedMember) error {
	labels := make([]string, 0, len(failures))
	causes := make([]error, 0, len(failures))
	for _, failure := range failures {
		labels = append(labels, failure.label)
		causes = append(causes, failure.err)
	}
	message := "model group fallback exhausted: " + strings.Join(labels, ", ")
	return apperrors.WrapWithCauses(systemName, apperrors.CodeProviderModelGroupExhausted, message, causes)
}
