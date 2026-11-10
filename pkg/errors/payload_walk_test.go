package apperrors

import (
	"testing"
	"time"
)

// TestProviderStatusCodeAndRetryAfter 钉住上游状态码与等待时长能从上层层层包装的错误里取出来。
func TestProviderStatusCodeAndRetryAfter(t *testing.T) {
	upstream := WrapWithDetails("model-provider-system", CodeProviderServiceFailed, "upstream rejected", nil, map[string]any{
		"statusCode": 429,
		"headers":    map[string]any{"retry-after": "7"},
	})
	err := Wrap("gateway-system", "gateway.upstream_failed", "call failed", upstream)

	payload := BuildErrorPayload(err)
	if status, ok := ProviderStatusCode(payload); !ok || status != 429 {
		t.Fatalf("status = %d, ok = %v, want 429", status, ok)
	}
	delay, ok := ProviderRetryAfter(payload)
	if !ok || delay != 7*time.Second {
		t.Fatalf("retry after = %s, ok = %v, want 7s", delay, ok)
	}
}

// TestHasErrorCodeWalksCauses 钉住错误码判断会走到并列原因里。
func TestHasErrorCodeWalksCauses(t *testing.T) {
	err := WrapWithCauses("model-provider-system", CodeProviderModelGroupExhausted, "exhausted", []error{
		New("model-provider-system", "network.connection_failed", "dial failed"),
		New("model-provider-system", "provider.storage_failed", "read failed"),
	})

	payload := BuildErrorPayload(err)
	if !HasErrorCode(payload, "network.connection_failed") {
		t.Fatalf("并列原因里的错误码没有被遍历到")
	}
}

// TestBuildErrorPayloadKeepsMultiCauseIdentity 钉住多原因错误保留自己的错误码与全部原因。
func TestBuildErrorPayloadKeepsMultiCauseIdentity(t *testing.T) {
	err := WrapWithCauses("model-provider-system", CodeProviderModelGroupExhausted, "exhausted", []error{
		New("model-provider-system", "provider.service_failed", "boom"),
		New("model-provider-system", "provider.service_failed", "bang"),
	})

	payload := BuildErrorPayload(err)
	if payload == nil || payload.Code != CodeProviderModelGroupExhausted {
		t.Fatalf("payload = %#v", payload)
	}
	if len(payload.Causes) != 2 {
		t.Fatalf("并列原因 = %d, want 2", len(payload.Causes))
	}
	if payload.Cause != nil {
		t.Fatalf("多原因不应退化成单 Cause")
	}
}
