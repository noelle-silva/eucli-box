//go:build windows

package datastorage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	apperrors "eucli-box/pkg/errors"
)

func TestCommitJSONRetriesTransientSharingViolation(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "index.json")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	temp, err := prepareJSON(context.Background(), target, map[string]string{"value": "new"})
	if err != nil {
		t.Fatalf("prepareJSON: %v", err)
	}
	defer func() { _ = os.Remove(temp) }()

	handle, err := os.Open(target)
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(15 * time.Millisecond)
		_ = handle.Close()
		close(released)
	}()

	before := CurrentStorageWriteMetrics()
	started := time.Now()
	if err := commitJSON(temp, target); err != nil {
		t.Fatalf("commitJSON() error = %v, want success after retry", err)
	}
	elapsed := time.Since(started)
	<-released

	after := CurrentStorageWriteMetrics()
	if after.CommitRetries <= before.CommitRetries {
		t.Fatalf("commit retries did not increase: before=%d after=%d", before.CommitRetries, after.CommitRetries)
	}
	if elapsed < 5*time.Millisecond {
		t.Fatalf("commitJSON() returned after %v, retry path did not run", elapsed)
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !strings.Contains(string(payload), "new") {
		t.Fatalf("target content = %q, want the new payload", payload)
	}
}

func TestCommitJSONExhaustsRetryBudgetWhenTargetStaysOpen(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "index.json")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	temp, err := prepareJSON(context.Background(), target, map[string]string{"value": "new"})
	if err != nil {
		t.Fatalf("prepareJSON: %v", err)
	}
	defer func() { _ = os.Remove(temp) }()

	handle, err := os.Open(target)
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	defer handle.Close()

	before := CurrentStorageWriteMetrics()
	started := time.Now()
	err = commitJSON(temp, target)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("commitJSON() = nil, want failure while target stays open")
	}
	if elapsed < 150*time.Millisecond {
		t.Fatalf("commitJSON() failed after %v, want the retry budget to be consumed", elapsed)
	}

	after := CurrentStorageWriteMetrics()
	if after.CommitRetryExhausted <= before.CommitRetryExhausted {
		t.Fatalf("exhausted counter did not increase: before=%d after=%d", before.CommitRetryExhausted, after.CommitRetryExhausted)
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("commitJSON() error = %T, want *AppError", err)
	}
	if appErr.Code != "storage.write_failed" {
		t.Fatalf("error code = %q, want storage.write_failed", appErr.Code)
	}
	details, ok := appErr.Details.(commitFailureDetails)
	if !ok {
		t.Fatalf("error details = %T, want commitFailureDetails", appErr.Details)
	}
	if details.SystemErrorCode != int(errorAccessDenied) && details.SystemErrorCode != int(errorSharingViolation) {
		t.Fatalf("system error code = %d, want access denied or sharing violation", details.SystemErrorCode)
	}
}

func TestCommitJSONDoesNotRetryNonSharingError(t *testing.T) {
	dir := t.TempDir()
	temp := filepath.Join(dir, "temp.json")
	if err := os.WriteFile(temp, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	target := filepath.Join(dir, "missing", "index.json")

	started := time.Now()
	err := commitJSON(temp, target)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("commitJSON() = nil, want failure")
	}
	if elapsed > 150*time.Millisecond {
		t.Fatalf("commitJSON() took %v, want fast failure without retry", elapsed)
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "storage.write_failed" {
		t.Fatalf("commitJSON() error = %v, want storage.write_failed", err)
	}
}

func TestIsRetryableCommitError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "access denied", err: &os.LinkError{Op: "rename", Old: "a", New: "b", Err: errorAccessDenied}, want: true},
		{name: "sharing violation", err: &os.LinkError{Op: "rename", Old: "a", New: "b", Err: errorSharingViolation}, want: true},
		{name: "file not found", err: &os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.Errno(2)}, want: false},
		{name: "plain error", err: errors.New("plain"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryableCommitError(tc.err); got != tc.want {
				t.Fatalf("isRetryableCommitError() = %v, want %v", got, tc.want)
			}
		})
	}
}
