package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"eucli-box/pkg/types"
)

type fakeGatewaySessionChanges struct {
	mu          sync.Mutex
	subscribers []chan types.SessionChangeEvent
}

func newFakeGatewaySessionChanges() *fakeGatewaySessionChanges {
	return &fakeGatewaySessionChanges{}
}

func (f *fakeGatewaySessionChanges) SubscribeSessionChanges(ctx context.Context) (<-chan types.SessionChangeEvent, func(), error) {
	ch := make(chan types.SessionChangeEvent, 32)
	f.mu.Lock()
	f.subscribers = append(f.subscribers, ch)
	f.mu.Unlock()
	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			for index, subscriber := range f.subscribers {
				if subscriber == ch {
					f.subscribers = append(f.subscribers[:index], f.subscribers[index+1:]...)
					return
				}
			}
		})
	}
	return ch, unsubscribe, nil
}

func (f *fakeGatewaySessionChanges) publish(event types.SessionChangeEvent) {
	f.mu.Lock()
	subscribers := append([]chan types.SessionChangeEvent(nil), f.subscribers...)
	f.mu.Unlock()
	for _, subscriber := range subscribers {
		subscriber <- event
	}
}

func (f *fakeGatewaySessionChanges) waitSubscribers(t *testing.T, count int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		current := len(f.subscribers)
		f.mu.Unlock()
		if current >= count {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session change subscriber count did not reach %d", count)
}

func sessionChangeFixture() []types.SessionChangeEvent {
	occurredAt := time.Now().UTC()
	return []types.SessionChangeEvent{
		{TargetKind: types.SessionChangeTargetMessage, Location: types.RoleSessionLocation("role-a", "session-1"), ChangeType: types.SessionChangeMessageAdded, Version: 1, OccurredAt: occurredAt},
		{TargetKind: types.SessionChangeTargetMessage, Location: types.GroupSessionLocation("group-a", "session-2"), ChangeType: types.SessionChangeMessageEdited, Version: 2, OccurredAt: occurredAt.Add(time.Millisecond)},
		{TargetKind: types.SessionChangeTargetMessage, Location: types.RoleSessionLocation("role-b", "session-3"), ChangeType: types.SessionChangeMessageDeleted, Version: 3, OccurredAt: occurredAt.Add(2 * time.Millisecond)},
		{TargetKind: types.SessionChangeTargetMessage, Location: types.WorkspaceSessionLocation("workspace-a", "role-c", "session-4"), ChangeType: types.SessionChangeMessageSubtreeDeleted, Version: 4, OccurredAt: occurredAt.Add(3 * time.Millisecond)},
		{TargetKind: types.SessionChangeTargetSession, Location: types.WorkspaceSessionLocation("workspace-b", "role-d", "session-5"), ChangeType: types.SessionChangeSessionPersisted, Version: 5, OccurredAt: occurredAt.Add(4 * time.Millisecond)},
	}
}

// 广播同时覆盖直连网关入口与长期端口入口，两个通道上的客户端都拿到同一份事件事实。
func TestWebSocketBroadcastsSessionChangesToDirectAndLongTermClients(t *testing.T) {
	fakes := newGatewayFakes()
	access := &fakeGatewayAccess{verifyResult: types.PersistentKeyVerifyResult{Valid: true, KeyID: "key-1"}}
	system := newAccessTestGateway(t, fakes, access)
	directServer := httptest.NewServer(system.Handler())
	defer directServer.Close()
	longTermServer := httptest.NewServer(system.LongTermHandler())
	defer longTermServer.Close()

	directConn := dialGatewayWebSocket(t, directServer.URL, "session-credential-0000000000000000000000000000000000000000000000000000000000000000")
	defer directConn.Close()
	longTermConn := dialGatewayWebSocket(t, longTermServer.URL, "long-term-key")
	defer longTermConn.Close()
	fakes.sessionChanges.waitSubscribers(t, 2)

	events := sessionChangeFixture()
	for _, event := range events {
		fakes.sessionChanges.publish(event)
	}

	for name, conn := range map[string]*websocket.Conn{"direct": directConn, "long-term": longTermConn} {
		for _, want := range events {
			payload := readWebSocketPayload(t, conn)
			event := assertSessionChangeFrame(t, payload)
			if event.TargetKind != want.TargetKind || event.Location != want.Location || event.ChangeType != want.ChangeType || event.Version != want.Version || !event.OccurredAt.Equal(want.OccurredAt) {
				t.Fatalf("%s event = %#v, want %#v", name, event, want)
			}
		}
	}
}

// 每一条会话变更都单独到达，不按会话来源或归属筛选，也不攒批合并。
func TestWebSocketDeliversEverySessionChangeWithoutBatching(t *testing.T) {
	fakes := newGatewayFakes()
	access := &fakeGatewayAccess{verifyResult: types.PersistentKeyVerifyResult{Valid: true, KeyID: "key-1"}}
	system := newAccessTestGateway(t, fakes, access)
	server := httptest.NewServer(system.Handler())
	defer server.Close()

	conn := dialGatewayWebSocket(t, server.URL, "session-credential-0000000000000000000000000000000000000000000000000000000000000000")
	defer conn.Close()
	fakes.sessionChanges.waitSubscribers(t, 1)

	const eventCount = 32
	occurredAt := time.Now().UTC()
	wantEvents := make([]types.SessionChangeEvent, 0, eventCount)
	for index := 1; index <= eventCount; index++ {
		event := types.SessionChangeEvent{
			TargetKind: types.SessionChangeTargetMessage,
			Location:   types.RoleSessionLocation("role-batch", "session-batch"),
			ChangeType: types.SessionChangeMessageAdded,
			Version:    uint64(index),
			OccurredAt: occurredAt.Add(time.Duration(index) * time.Millisecond),
		}
		wantEvents = append(wantEvents, event)
		fakes.sessionChanges.publish(event)
	}

	for _, want := range wantEvents {
		event := assertSessionChangeFrame(t, readWebSocketPayload(t, conn))
		if event.Version != want.Version || event.ChangeType != want.ChangeType || event.Location != want.Location {
			t.Fatalf("event = %#v, want %#v", event, want)
		}
	}
}

func dialGatewayWebSocket(t *testing.T, serverURL string, key string) *websocket.Conn {
	t.Helper()
	target := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws/events"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+key)
	conn, response, err := websocket.DefaultDialer.Dial(target, header)
	if conn != nil {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	}
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("Dial(%s) error = %v status=%d", target, err, status)
	}
	return conn
}

func readWebSocketPayload(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	return payload
}

// assertSessionChangeFrame 校验广播帧就是阶段一契约的五个顶层字段，没有包装、改名或裁剪。
func assertSessionChangeFrame(t *testing.T, payload []byte) types.SessionChangeEvent {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatalf("decode event frame error = %v payload=%s", err, payload)
	}
	for _, field := range []string{"targetKind", "location", "changeType", "version", "occurredAt"} {
		if _, ok := raw[field]; !ok {
			t.Fatalf("event frame missing %q: %s", field, payload)
		}
	}
	if len(raw) != 5 {
		t.Fatalf("event frame has %d fields, want 5: %s", len(raw), payload)
	}
	event, err := types.UnmarshalSessionChangeEvent(payload)
	if err != nil {
		t.Fatalf("UnmarshalSessionChangeEvent() error = %v payload=%s", err, payload)
	}
	assertSessionLocationShape(t, payload, raw["location"], event.Location.Kind)
	return event
}

// assertSessionLocationShape 校验定位载荷只携带该形态应有的字段，三种形态互不夹带。
func assertSessionLocationShape(t *testing.T, payload []byte, raw json.RawMessage, kind types.SessionLocationKind) {
	t.Helper()
	var location map[string]json.RawMessage
	if err := json.Unmarshal(raw, &location); err != nil {
		t.Fatalf("decode location error = %v payload=%s", err, payload)
	}
	want := map[string]bool{"kind": true, "sessionId": true}
	switch kind {
	case types.SessionLocationRole:
		want["roleId"] = true
	case types.SessionLocationGroup:
		want["groupId"] = true
	case types.SessionLocationWorkspace:
		want["workspaceId"] = true
		want["roleId"] = true
	default:
		t.Fatalf("unexpected session location kind %q payload=%s", kind, payload)
	}
	if len(location) != len(want) {
		t.Fatalf("location %q has %d fields, want %d: %s", kind, len(location), len(want), payload)
	}
	for field := range want {
		if _, ok := location[field]; !ok {
			t.Fatalf("location %q missing %q: %s", kind, field, payload)
		}
	}
}
