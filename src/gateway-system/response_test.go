package gateway

import (
	"errors"
	"testing"
)

func TestErrorPayloadForResponseWrapsPlainErrorWithGatewayContext(t *testing.T) {
	payload := errorPayloadForResponse(errors.New("socket closed"))

	if payload.Code != "gateway.internal_error" || payload.System != systemName {
		t.Fatalf("payload source = %q/%q", payload.System, payload.Code)
	}
	if payload.Message != "internal gateway error" {
		t.Fatalf("payload message = %q", payload.Message)
	}
	if payload.Cause == nil || payload.Cause.Message != "socket closed" {
		t.Fatalf("payload cause = %#v", payload.Cause)
	}
	if payload.Cause.Code != "" || payload.Cause.System != "" {
		t.Fatalf("raw cause source = %q/%q", payload.Cause.System, payload.Cause.Code)
	}
}
