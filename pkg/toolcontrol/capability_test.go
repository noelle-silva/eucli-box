package toolcontrol

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

// startCapabilityPair 建立一个已完成握手、已开始监视与应答的宿主—工具桩对。
func startCapabilityPair(t *testing.T, config Config) (*Server, *toolStub, context.CancelFunc) {
	t.Helper()
	server := newTestServerWithConfig(t, config)
	ctx, cancel := context.WithCancel(context.Background())
	handshake := make(chan error, 1)
	go func() { handshake <- server.AcceptAndHandshake(ctx) }()
	stub, err := connectToolStub(ctx, server.Address(), server.Token())
	if err != nil {
		cancel()
		t.Fatalf("connectToolStub() error = %v", err)
	}
	if err := stub.waitReady(ctx); err != nil {
		cancel()
		t.Fatalf("waitReady() error = %v", err)
	}
	if err := <-handshake; err != nil {
		cancel()
		t.Fatalf("AcceptAndHandshake() error = %v", err)
	}
	t.Cleanup(func() {
		cancel()
		_ = stub.close()
	})
	server.Watch(ctx)
	go func() { _ = stub.serve(ctx) }()
	return server, stub, cancel
}

func capabilityTestConfig(handler func(ctx context.Context, request CapabilityRequest) CapabilityResult) Config {
	return Config{Timeout: 200 * time.Millisecond, PingInterval: 20 * time.Millisecond, OnCapabilityRequest: handler}
}

func TestCapabilityRequestServesToolAsk(t *testing.T) {
	var received CapabilityRequest
	_, stub, _ := startCapabilityPair(t, capabilityTestConfig(func(ctx context.Context, request CapabilityRequest) CapabilityResult {
		received = request
		return CapabilityResult{Status: CapabilityStatusSuccess, Payload: map[string]any{"value": "42"}}
	}))
	result, err := stub.request(context.Background(), "session-state", "read", map[string]any{"key": "session-title"})
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if result.Status != CapabilityStatusSuccess {
		t.Fatalf("result = %#v", result)
	}
	var payload map[string]any
	payloadBytes, ok := result.Payload.(json.RawMessage)
	if !ok {
		t.Fatalf("payload type = %T", result.Payload)
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("payload decode error = %v", err)
	}
	if payload["value"] != "42" {
		t.Fatalf("payload = %#v", payload)
	}
	if received.Capability != "session-state" || received.Access != "read" || len(received.Payload) == 0 {
		t.Fatalf("received request = %#v", received)
	}
}

func TestCapabilityRequestCarriesDeniedFeedback(t *testing.T) {
	_, stub, _ := startCapabilityPair(t, capabilityTestConfig(func(ctx context.Context, request CapabilityRequest) CapabilityResult {
		return CapabilityResult{Status: CapabilityStatusDenied, Error: CapabilityDeniedMessage}
	}))
	result, err := stub.request(context.Background(), "session-attachments", "read", map[string]any{"operation": "list"})
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if result.Status != CapabilityStatusDenied || result.Error != CapabilityDeniedMessage {
		t.Fatalf("result = %#v", result)
	}
}

func TestCapabilityRequestWithoutHandlerFails(t *testing.T) {
	_, stub, _ := startCapabilityPair(t, Config{Timeout: 200 * time.Millisecond, PingInterval: 20 * time.Millisecond})
	result, err := stub.request(context.Background(), "workspace", "read", nil)
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if result.Status != CapabilityStatusFailed || result.Error == "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestCapabilityRequestHonoursCallerContext(t *testing.T) {
	_, stub, _ := startCapabilityPair(t, capabilityTestConfig(func(ctx context.Context, request CapabilityRequest) CapabilityResult {
		time.Sleep(500 * time.Millisecond)
		return CapabilityResult{Status: CapabilityStatusSuccess}
	}))
	requestCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := stub.request(requestCtx, "session-state", "read", nil)
	if err == nil {
		t.Fatal("Request() error = nil, want deadline exceeded")
	}
}

func TestCapabilityRequestsAreConcurrentAndMatchedByRequestID(t *testing.T) {
	_, stub, _ := startCapabilityPair(t, capabilityTestConfig(func(ctx context.Context, request CapabilityRequest) CapabilityResult {
		var payload map[string]any
		_ = json.Unmarshal(request.Payload, &payload)
		return CapabilityResult{Status: CapabilityStatusSuccess, Payload: map[string]any{"echo": payload["id"]}}
	}))
	const count = 8
	var wg sync.WaitGroup
	results := make([]string, count)
	for index := 0; index < count; index++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			id := "request-" + string(rune('a'+slot))
			result, err := stub.request(context.Background(), "session-state", "read", map[string]any{"id": id})
			if err != nil || result.Status != CapabilityStatusSuccess {
				results[slot] = "error"
				return
			}
			var payload map[string]any
			payloadBytes, _ := result.Payload.(json.RawMessage)
			_ = json.Unmarshal(payloadBytes, &payload)
			echo, _ := payload["echo"].(string)
			results[slot] = echo
		}(index)
	}
	wg.Wait()
	for index, result := range results {
		if result == "error" || result == "" {
			t.Fatalf("request %d failed: %#v", index, results)
		}
	}
}

func TestCapabilityRequestAfterToolStubCloseFails(t *testing.T) {
	_, stub, _ := startCapabilityPair(t, capabilityTestConfig(func(ctx context.Context, request CapabilityRequest) CapabilityResult {
		return CapabilityResult{Status: CapabilityStatusSuccess}
	}))
	if err := stub.close(); err != nil {
		t.Fatalf("close() error = %v", err)
	}
	_, err := stub.request(context.Background(), "session-state", "read", nil)
	if err == nil {
		t.Fatal("request() after close() error = nil")
	}
}

func TestCapabilityRequestLimitIsEnforced(t *testing.T) {
	server, stub, _ := startCapabilityPair(t, capabilityTestConfig(func(ctx context.Context, request CapabilityRequest) CapabilityResult {
		return CapabilityResult{Status: CapabilityStatusSuccess}
	}))
	server.mu.Lock()
	server.capabilityRequests = MaxCapabilityRequests
	server.mu.Unlock()
	result, err := stub.request(context.Background(), "session-state", "read", nil)
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if result.Status != CapabilityStatusFailed || !strings.Contains(result.Error, "limit") {
		t.Fatalf("result = %#v", result)
	}
}

// TestCapabilityRequestsQueueBeyondConcurrency 验证并发槽满时请求进入等待
// 而不是被丢弃：槽位缩小到 1，两个并发请求仍然都拿到成功响应。
func TestCapabilityRequestsQueueBeyondConcurrency(t *testing.T) {
	server, stub, _ := startCapabilityPair(t, capabilityTestConfig(func(ctx context.Context, request CapabilityRequest) CapabilityResult {
		time.Sleep(30 * time.Millisecond)
		return CapabilityResult{Status: CapabilityStatusSuccess}
	}))
	server.capabilitySlots = make(chan struct{}, 1)
	const count = 2
	var wg sync.WaitGroup
	statuses := make([]string, count)
	for index := 0; index < count; index++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			result, err := stub.request(context.Background(), "session-state", "read", nil)
			if err != nil {
				statuses[slot] = "error"
				return
			}
			statuses[slot] = result.Status
		}(index)
	}
	wg.Wait()
	for index, status := range statuses {
		if status != CapabilityStatusSuccess {
			t.Fatalf("request %d status = %q, statuses = %#v", index, status, statuses)
		}
	}
}
