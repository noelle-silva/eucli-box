package requestrecord

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

type fakeNetwork struct {
	requests []types.HTTPRequest
	response types.HTTPResponse
	err      error
}

func (f *fakeNetwork) Do(ctx context.Context, req types.HTTPRequest) (types.HTTPResponse, error) {
	f.requests = append(f.requests, req)
	return f.response, f.err
}

func (f *fakeNetwork) DoStream(ctx context.Context, req types.HTTPRequest, onChunk types.HTTPStreamHandler) (types.HTTPResponse, error) {
	f.requests = append(f.requests, req)
	return f.response, f.err
}

type fakeStorage struct {
	mu      sync.Mutex
	config  types.RequestRecordConfig
	records []types.RequestRecord
}

func (f *fakeStorage) LoadRequestRecordConfig(ctx context.Context) (types.RequestRecordConfig, error) {
	return f.config, nil
}

func (f *fakeStorage) SaveRequestRecordConfig(ctx context.Context, config types.RequestRecordConfig) (types.RequestRecordConfig, error) {
	f.config = config
	return config, nil
}

func (f *fakeStorage) AppendRequestRecord(ctx context.Context, record types.RequestRecord) (types.RequestRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, record)
	return record, nil
}

func (f *fakeStorage) ListRequestRecords(ctx context.Context) ([]types.RequestRecordSummary, error) {
	return nil, nil
}

func (f *fakeStorage) LoadRequestRecord(ctx context.Context, recordID string) (types.RequestRecord, error) {
	return types.RequestRecord{}, nil
}

func (f *fakeStorage) recordCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.records)
}

func (f *fakeStorage) recordAt(index int) types.RequestRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.records[index]
}

func waitForRecordCount(t *testing.T, storage *fakeStorage, count int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if storage.recordCount() >= count {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d records, got %d", count, storage.recordCount())
}

func TestRecordingNetworkRecordsRequestAndResponse(t *testing.T) {
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Headers: map[string][]string{"Content-Type": {"text/event-stream"}}, Body: []byte("data: hello"), Duration: 150 * time.Millisecond}}
	storage := &fakeStorage{config: types.RequestRecordConfig{Enabled: true, Limit: 100}}
	system, err := NewSystem(network, storage)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}

	req := types.HTTPRequest{Method: "POST", URL: "https://api.example.com/v1/chat/completions", Headers: map[string]string{"Authorization": "Bearer secret", "x-api-key": "secret-key", "Content-Type": "application/json"}, Body: []byte(`{"model":"x"}`)}
	if _, err := system.Do(context.Background(), req); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	waitForRecordCount(t, storage, 1)
	if len(network.requests) != 1 {
		t.Fatalf("network requests = %d", len(network.requests))
	}
	record := storage.recordAt(0)
	if record.Method != "POST" || record.URL != "https://api.example.com/v1/chat/completions" || record.Body != `{"model":"x"}` {
		t.Fatalf("record request = %#v", record)
	}
	if record.Headers["Authorization"] != "[REDACTED]" || record.Headers["x-api-key"] != "[REDACTED]" {
		t.Fatalf("record headers not redacted = %#v", record.Headers)
	}
	if record.Headers["Content-Type"] != "application/json" {
		t.Fatalf("record headers lost original values = %#v", record.Headers)
	}
	if record.ResponseStatus != 200 || record.ResponseBody != "data: hello" || len(record.ResponseHeaders["Content-Type"]) != 1 {
		t.Fatalf("record response = %#v", record)
	}
	if record.DurationMs != 150 || record.Error != "" {
		t.Fatalf("record timing/error = %#v", record)
	}
}

func TestRecordingDisabledForwardsWithoutRecording(t *testing.T) {
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte("ok")}}
	storage := &fakeStorage{config: types.RequestRecordConfig{Enabled: false, Limit: 100}}
	system, err := NewSystem(network, storage)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	if _, err := system.Do(context.Background(), types.HTTPRequest{Method: "GET", URL: "https://api.example.com/v1/models"}); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if len(network.requests) != 1 {
		t.Fatalf("downstream requests = %d", len(network.requests))
	}
	time.Sleep(100 * time.Millisecond)
	if storage.recordCount() != 0 {
		t.Fatalf("records should be empty when disabled = %d", storage.recordCount())
	}
}

func TestRecordingStreamRecordsAndKeepsFailedRequest(t *testing.T) {
	networkErr := errors.New("connection refused")
	network := &fakeNetwork{err: networkErr}
	storage := &fakeStorage{config: types.RequestRecordConfig{Enabled: true, Limit: 100}}
	system, err := NewSystem(network, storage)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	if _, err := system.DoStream(context.Background(), types.HTTPRequest{Method: "POST", URL: "https://api.example.com/v1/chat/completions"}, nil); !errors.Is(err, networkErr) {
		t.Fatalf("DoStream() error = %v", err)
	}
	waitForRecordCount(t, storage, 1)
	record := storage.recordAt(0)
	if record.Error != "connection refused" || record.ResponseStatus != 0 {
		t.Fatalf("failed record = %#v", record)
	}
}

func TestSaveRequestRecordConfigValidatesLimit(t *testing.T) {
	network := &fakeNetwork{}
	storage := &fakeStorage{config: types.RequestRecordConfig{Limit: types.RequestRecordLimitDefault}}
	system, err := NewSystem(network, storage)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	if _, err := system.SaveRequestRecordConfig(context.Background(), types.RequestRecordConfig{Limit: types.RequestRecordLimitMax + 1}); err == nil {
		t.Fatalf("expected invalid limit error")
	}
	saved, err := system.SaveRequestRecordConfig(context.Background(), types.RequestRecordConfig{Enabled: true, Limit: 20})
	if err != nil {
		t.Fatalf("SaveRequestRecordConfig() error = %v", err)
	}
	if !saved.Enabled || saved.Limit != 20 || !storage.config.Enabled || storage.config.Limit != 20 {
		t.Fatalf("saved config = %#v storage = %#v", saved, storage.config)
	}
}
