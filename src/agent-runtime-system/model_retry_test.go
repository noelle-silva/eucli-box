package agentruntime

import (
	"context"
	"testing"

	apperrors "eucli-box/pkg/errors"
)

// TestModelRetryDecisionSkipsExhaustedModelGroup 钉住运行时不叠加兜底：
// 模型组已经把成员逐个试完，运行时再重试只会把上游调用量成倍放大。
func TestModelRetryDecisionSkipsExhaustedModelGroup(t *testing.T) {
	upstream := apperrors.WrapWithDetails("model-provider-system", apperrors.CodeProviderServiceFailed, "boom", nil, map[string]any{"statusCode": 503})
	err := apperrors.WrapWithCauses("model-provider-system", apperrors.CodeProviderModelGroupExhausted, "fallback exhausted", []error{upstream})

	if decision := modelRetryDecisionForError(err, 1); decision.Retryable {
		t.Fatalf("decision = %#v, want 不可重试", decision)
	}
}

// TestModelRetryDecisionKeepsUpstreamStatusRetry 钉住原有重试判定没有被改坏。
func TestModelRetryDecisionKeepsUpstreamStatusRetry(t *testing.T) {
	rateLimited := apperrors.WrapWithDetails("model-provider-system", apperrors.CodeProviderServiceFailed, "rate limited", nil, map[string]any{"statusCode": 429})
	decision := modelRetryDecisionForError(rateLimited, 1)
	if !decision.Retryable || decision.Message != modelRetryStatusRateLimited {
		t.Fatalf("429 decision = %#v", decision)
	}

	badRequest := apperrors.WrapWithDetails("model-provider-system", apperrors.CodeProviderServiceFailed, "bad request", nil, map[string]any{"statusCode": 400})
	if decision := modelRetryDecisionForError(badRequest, 1); decision.Retryable {
		t.Fatalf("400 decision = %#v, want 不可重试", decision)
	}

	canceled := apperrors.Wrap("model-provider-system", "provider.network_failed", "canceled", context.Canceled)
	if decision := modelRetryDecisionForError(canceled, 1); decision.Retryable {
		t.Fatalf("取消 decision = %#v, want 不可重试", decision)
	}
}
