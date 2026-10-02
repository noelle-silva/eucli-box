package webfetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestIsBlockedStatus(t *testing.T) {
	for _, status := range []int{401, 403, 407, 429, 444, 500, 502, 503, 504} {
		if !isBlockedStatus(status) {
			t.Fatalf("status %d must be blocked", status)
		}
	}
	for _, status := range []int{200, 201, 204, 301, 400, 404, 418} {
		if isBlockedStatus(status) {
			t.Fatalf("status %d must not be blocked", status)
		}
	}
}

func TestRetryDelayBacksOff(t *testing.T) {
	config := Config{RetryBaseDelayMs: 500, MaxDelayMs: 5000}
	if got := retryDelay(config, 1, 0); got != 500*time.Millisecond {
		t.Fatalf("attempt 1 = %v", got)
	}
	if got := retryDelay(config, 2, 0); got != 1000*time.Millisecond {
		t.Fatalf("attempt 2 = %v", got)
	}
	if got := retryDelay(config, 10, 0); got != 5000*time.Millisecond {
		t.Fatalf("attempt 10 must cap = %v", got)
	}
	if got := retryDelay(config, 1, 2*time.Second); got != 2*time.Second {
		t.Fatalf("retry-after honored = %v", got)
	}
	if got := retryDelay(config, 1, 60*time.Second); got != 5000*time.Millisecond {
		t.Fatalf("retry-after capped = %v", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if delay, ok := parseRetryAfter("2"); !ok || delay != 2*time.Second {
		t.Fatalf("seconds = %v ok=%v", delay, ok)
	}
	future := time.Now().Add(5 * time.Second).UTC().Format(http.TimeFormat)
	delay, ok := parseRetryAfter(future)
	if !ok || delay <= 0 || delay > 6*time.Second {
		t.Fatalf("date = %v ok=%v", delay, ok)
	}
	if _, ok := parseRetryAfter("not-a-date"); ok {
		t.Fatal("invalid value must not parse")
	}
	if _, ok := parseRetryAfter(""); ok {
		t.Fatal("empty value must not parse")
	}
}

func TestSelectBrowserProfileVaries(t *testing.T) {
	names := profileNames()
	if len(names) < 3 {
		t.Fatalf("expected multiple profiles, got %v", names)
	}
	for _, name := range names {
		if _, ok := profileByName(name); !ok {
			t.Fatalf("profile %q not found by name", name)
		}
	}
	if _, ok := profileByName("does-not-exist"); ok {
		t.Fatal("unknown profile must not resolve")
	}
}

func TestExecuteRetriesOnBlockedStatus(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) < 3 {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("blocked"))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("finally ok"))
	}))
	defer server.Close()
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:         map[string]any{"url": server.URL},
		ToolBodyDirectory: writeConfig(t),
	})
	if result.Status != types.ToolStatusSuccess || result.Metadata["attempts"] != 3 {
		t.Fatalf("result = %#v", result)
	}
	if result.Metadata["statusCode"] != 200 {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteReturnsBlockedStatusAfterRetries(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("still blocked"))
	}))
	defer server.Close()
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:         map[string]any{"url": server.URL},
		ToolBodyDirectory: writeConfig(t),
	})
	if result.Status != types.ToolStatusSuccess || result.Metadata["statusCode"] != 403 {
		t.Fatalf("result = %#v", result)
	}
	if atomic.LoadInt32(&hits) != 3 {
		t.Fatalf("hits = %d, expected 3 attempts", hits)
	}
}
