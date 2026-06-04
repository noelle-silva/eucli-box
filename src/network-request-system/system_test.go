package networkrequest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

func TestDoReturnsStandardResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content type = %s", r.Header.Get("Content-Type"))
		}
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	system, err := NewSystem(Config{MaxTimeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	resp, err := system.Do(context.Background(), types.HTTPRequest{
		Method:   http.MethodPost,
		URL:      server.URL,
		BodyKind: types.HTTPBodyJSON,
		Body:     []byte(`{"hello":"world"}`),
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Fatalf("body = %s", string(resp.Body))
	}
	if resp.Headers["X-Test"][0] != "ok" {
		t.Fatalf("header = %#v", resp.Headers)
	}
}

func TestDoDoesNotTreatNon2xxAsNetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream failed"))
	}))
	defer server.Close()

	system, err := NewSystem(Config{MaxTimeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	resp, err := system.Do(context.Background(), types.HTTPRequest{Method: http.MethodGet, URL: server.URL})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestDoRejectsInvalidRequest(t *testing.T) {
	system, err := NewSystem(Config{MaxTimeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	_, err = system.Do(context.Background(), types.HTTPRequest{Method: "TRACE", URL: "https://example.com"})
	assertAppErrorCode(t, err, "network.invalid_request")
}

func TestDoReportsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	system, err := NewSystem(Config{DefaultTimeout: time.Second, MaxTimeout: time.Second})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	_, err = system.Do(context.Background(), types.HTTPRequest{Method: http.MethodGet, URL: server.URL, Timeout: time.Millisecond})
	assertAppErrorCode(t, err, "network.timeout")
}

func TestDoStreamUsesIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		parts := []string{"a", "b", "c", "d"}
		for index, part := range parts {
			_, _ = w.Write([]byte(part))
			if flusher != nil {
				flusher.Flush()
			}
			if index < len(parts)-1 {
				time.Sleep(15 * time.Millisecond)
			}
		}
	}))
	defer server.Close()

	system, err := NewSystem(Config{DefaultTimeout: time.Second, MaxTimeout: time.Second})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	response, err := system.DoStream(context.Background(), types.HTTPRequest{Method: http.MethodGet, URL: server.URL, Timeout: 30 * time.Millisecond}, nil)
	if err != nil {
		t.Fatalf("DoStream() error = %v", err)
	}
	if string(response.Body) != "abcd" {
		t.Fatalf("body = %q", string(response.Body))
	}
}

func TestDoStreamReportsIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("a"))
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(80 * time.Millisecond)
		_, _ = w.Write([]byte("b"))
	}))
	defer server.Close()

	system, err := NewSystem(Config{DefaultTimeout: time.Second, MaxTimeout: time.Second})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	_, err = system.DoStream(context.Background(), types.HTTPRequest{Method: http.MethodGet, URL: server.URL, Timeout: 20 * time.Millisecond}, nil)
	assertAppErrorCode(t, err, "network.timeout")
}

func TestNewSystemRejectsZeroMaxTimeout(t *testing.T) {
	_, err := NewSystem(Config{MaxTimeout: 0, DefaultTimeout: time.Second})
	if err == nil {
		t.Fatal("expected error for zero MaxTimeout")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "network.invalid_request" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildRequestRejectsNilContext(t *testing.T) {
	system, err := NewSystem(Config{MaxTimeout: time.Second, DefaultTimeout: time.Second})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	_, err = system.Do(nil, types.HTTPRequest{Method: http.MethodGet, URL: "https://example.com"})
	assertAppErrorCode(t, err, "network.invalid_request")
}

func TestDoRejectsNonHttpScheme(t *testing.T) {
	system, err := NewSystem(Config{MaxTimeout: time.Second, DefaultTimeout: time.Second})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	_, err = system.Do(context.Background(), types.HTTPRequest{Method: http.MethodGet, URL: "ftp://example.com"})
	assertAppErrorCode(t, err, "network.invalid_request")
}

func assertAppErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not AppError", err)
	}
	if appErr.Code != code {
		t.Fatalf("code = %s, want %s", appErr.Code, code)
	}
}
