package webfetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func writeConfig(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	payload := `{"maxResponseBytes":5000000,"maxBodyChars":100000,"defaultTimeoutMs":30000,"maxRedirects":5,"maxOutputChars":200000,"proxyUrl":"","maxRetries":2,"retryBaseDelayMs":10,"maxDelayMs":100}`
	if err := os.WriteFile(filepath.Join(directory, "config.json"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return directory
}

func TestExecuteConvertsHTMLToMarkdown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>t</title><script>bad()</script></head><body><h1>Title</h1><p>Hello <strong>world</strong></p><table><tr><th>A</th><th>B</th></tr><tr><td>1</td><td>2</td></tr></table></body></html>`))
	}))
	defer server.Close()
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:         map[string]any{"url": server.URL},
		DefaultConfig:     map[string]any{"maxOutputChars": 200000},
		ToolBodyDirectory: writeConfig(t),
	})
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	for _, fragment := range []string{"# Title", "**world**", "| A", "| B", externalContentNotice} {
		if !strings.Contains(result.Content, fragment) {
			t.Fatalf("content missing %q:\n%s", fragment, result.Content)
		}
	}
	if strings.Contains(result.Content, "bad()") {
		t.Fatalf("script content must be removed:\n%s", result.Content)
	}
	if result.Metadata["statusCode"] != 200 {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteReturnsNonSuccessStatusAsResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("missing"))
	}))
	defer server.Close()
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:         map[string]any{"url": server.URL},
		ToolBodyDirectory: writeConfig(t),
	})
	if result.Status != types.ToolStatusSuccess || !strings.Contains(result.Content, "HTTP 404") || !strings.Contains(result.Content, "missing") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteRejectsBinaryContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte{0x00, 0x01, 0x02})
	}))
	defer server.Close()
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:         map[string]any{"url": server.URL},
		ToolBodyDirectory: writeConfig(t),
	})
	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "unsupported content type") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteFollowsRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("arrived"))
	}))
	defer server.Close()
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:         map[string]any{"url": server.URL + "/start"},
		ToolBodyDirectory: writeConfig(t),
	})
	if result.Status != types.ToolStatusSuccess || !strings.Contains(result.Content, "arrived") {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Metadata["finalUrl"].(string), "/final") {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestExecuteRejectsURLWithCredentials(t *testing.T) {
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:         map[string]any{"url": "http://user:pass@example.com/"},
		ToolBodyDirectory: writeConfig(t),
	})
	if result.Status != types.ToolStatusFailed || !strings.Contains(result.Error, "credentials") {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteTruncatesLongOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(strings.Repeat("a", 5000)))
	}))
	defer server.Close()
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:         map[string]any{"url": server.URL, "maxOutputChars": 1000},
		ToolBodyDirectory: writeConfig(t),
	})
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	if result.Metadata["truncated"] != true || !strings.Contains(result.Content, "Content truncated") {
		t.Fatalf("result = %#v", result)
	}
	if runeLength(result.Content) > 1000 {
		t.Fatalf("content length = %d", runeLength(result.Content))
	}
}

func TestExecuteHonorsLocalhostURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("local"))
	}))
	defer server.Close()
	result := Execute(context.Background(), types.ToolExecutionInput{
		Arguments:         map[string]any{"url": server.URL},
		ToolBodyDirectory: writeConfig(t),
	})
	if result.Status != types.ToolStatusSuccess || !strings.Contains(result.Content, "local") {
		t.Fatalf("result = %#v", result)
	}
}

func TestDecodeBodyHonorsCharset(t *testing.T) {
	decoded, err := decodeBody([]byte{0x48, 0x69}, "text/plain; charset=utf-8")
	if err != nil || decoded != "Hi" {
		t.Fatalf("decoded = %q err = %v", decoded, err)
	}
}
