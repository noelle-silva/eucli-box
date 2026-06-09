package apperrors

import (
	"fmt"
	"strings"
	"testing"
)

func TestBuildErrorPayloadPreservesPlainWrapper(t *testing.T) {
	inner := New("storage-system", "storage.write_failed", "write failed")
	err := fmt.Errorf("delete metadata write failed: %w", inner)

	payload := BuildErrorPayload(err)
	if payload == nil {
		t.Fatal("BuildErrorPayload() returned nil")
	}
	if payload.Code != "" || payload.System != "" {
		t.Fatalf("outer payload should stay raw, got code=%q system=%q", payload.Code, payload.System)
	}
	if !strings.Contains(payload.Message, "delete metadata write failed") {
		t.Fatalf("outer payload message = %q", payload.Message)
	}
	if payload.Cause == nil {
		t.Fatal("outer payload cause is nil")
	}
	if payload.Cause.Code != "storage.write_failed" || payload.Cause.Message != "write failed" || payload.Cause.System != "storage-system" {
		t.Fatalf("inner payload = %#v", payload.Cause)
	}
	if len(payload.Causes) != 0 {
		t.Fatalf("outer payload parallel causes = %#v", payload.Causes)
	}
}

func TestBuildErrorPayloadPreservesParallelCauses(t *testing.T) {
	writeErr := New("storage-system", "storage.write_failed", "metadata write failed")
	rollbackErr := New("storage-system", "storage.rollback_failed", "rollback failed")
	err := fmt.Errorf("delete metadata write failed: %w; rollback also failed: %w", writeErr, rollbackErr)

	payload := BuildErrorPayload(err)
	if payload == nil {
		t.Fatal("BuildErrorPayload() returned nil")
	}
	if !strings.Contains(payload.Message, "delete metadata write failed") {
		t.Fatalf("outer payload message = %q", payload.Message)
	}
	if payload.Cause != nil {
		t.Fatalf("outer payload single cause = %#v", payload.Cause)
	}
	if len(payload.Causes) != 2 {
		t.Fatalf("parallel cause count = %d, want 2", len(payload.Causes))
	}
	if payload.Causes[0].Code != "storage.write_failed" || payload.Causes[1].Code != "storage.rollback_failed" {
		t.Fatalf("parallel causes = %#v", payload.Causes)
	}
}
