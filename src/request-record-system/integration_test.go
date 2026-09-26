package requestrecord

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"eucli-box/pkg/types"
	networkrequest "eucli-box/src/network-request-system"
)

func TestRecordingNetworkAgainstHTTPTestServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("upstream authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"deepseek-v4.1-flash"}]}`))
	}))
	defer server.Close()

	realNetwork, err := networkrequest.NewSystem(networkrequest.Config{MaxTimeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("networkrequest.NewSystem() error = %v", err)
	}
	storage := &fakeStorage{config: types.RequestRecordConfig{Enabled: true, Limit: 100}}
	system, err := NewSystem(realNetwork, storage)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}

	response, err := system.Do(context.Background(), types.HTTPRequest{
		Method:  "GET",
		URL:     server.URL + "/v1/models",
		Headers: map[string]string{"Authorization": "Bearer secret"},
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("response status = %d", response.StatusCode)
	}
	if len(storage.records) != 1 {
		t.Fatalf("records = %d", len(storage.records))
	}
	record := storage.records[0]
	if record.Method != "GET" || record.URL != server.URL+"/v1/models" {
		t.Fatalf("record request = %#v", record)
	}
	if record.Headers["Authorization"] != "[REDACTED]" {
		t.Fatalf("record headers not redacted = %#v", record.Headers)
	}
	if record.ResponseStatus != http.StatusOK || record.ResponseBody != `{"data":[{"id":"deepseek-v4.1-flash"}]}` {
		t.Fatalf("record response = %#v", record)
	}
	if len(record.ResponseHeaders["Content-Type"]) != 1 || record.ResponseHeaders["Content-Type"][0] != "application/json" {
		t.Fatalf("record response headers = %#v", record.ResponseHeaders)
	}
	if record.DurationMs < 0 {
		t.Fatalf("record duration = %d", record.DurationMs)
	}
}
