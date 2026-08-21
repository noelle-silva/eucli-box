package toolcontrol

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestServerClientHandshake(t *testing.T) {
	server := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	handshake := make(chan error, 1)
	go func() { handshake <- server.AcceptAndHandshake(ctx) }()
	client, err := Connect(ctx, server.Address(), server.Token())
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}
	if err := <-handshake; err != nil {
		t.Fatalf("AcceptAndHandshake() error = %v", err)
	}
}

func TestServerRejectsWrongToken(t *testing.T) {
	server := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	handshake := make(chan error, 1)
	go func() { handshake <- server.AcceptAndHandshake(ctx) }()
	conn, err := net.Dial("tcp", server.Address())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(Message{Version: ProtocolVersion, Type: MessageHello, Token: "wrong"}); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if err := <-handshake; err == nil {
		t.Fatal("AcceptAndHandshake() error = nil")
	}
}

func TestServerRejectsWrongVersion(t *testing.T) {
	server := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	handshake := make(chan error, 1)
	go func() { handshake <- server.AcceptAndHandshake(ctx) }()
	conn, err := net.Dial("tcp", server.Address())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(Message{Version: ProtocolVersion + 1, Type: MessageHello, Token: server.Token()}); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if err := <-handshake; err == nil {
		t.Fatal("AcceptAndHandshake() error = nil")
	}
}

func TestClientValidatesReady(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()
	serverDone := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverDone <- acceptErr
			return
		}
		defer conn.Close()
		var hello Message
		if decodeErr := json.NewDecoder(bufio.NewReader(conn)).Decode(&hello); decodeErr != nil {
			serverDone <- decodeErr
			return
		}
		serverDone <- json.NewEncoder(conn).Encode(Message{Version: ProtocolVersion, Type: MessageReady, Token: "wrong"})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := Connect(ctx, listener.Addr().String(), "expected")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()
	if err := client.WaitReady(ctx); err == nil {
		t.Fatal("WaitReady() error = nil")
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("fake server error = %v", err)
	}
}

func TestClientRepliesToPingWithoutCommandOutput(t *testing.T) {
	server := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handshake := make(chan error, 1)
	go func() { handshake <- server.AcceptAndHandshake(ctx) }()
	client, err := Connect(ctx, server.Address(), server.Token())
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}
	if err := <-handshake; err != nil {
		t.Fatalf("AcceptAndHandshake() error = %v", err)
	}
	watch := server.Watch(ctx)
	serveDone := make(chan error, 1)
	go func() { serveDone <- client.Serve(ctx) }()
	select {
	case failure := <-watch:
		t.Fatalf("unexpected watchdog failure = %s", failure)
	case <-time.After(3 * time.Duration(50) * time.Millisecond):
		cancel()
	}
	select {
	case <-serveDone:
	case <-time.After(time.Second):
		t.Fatal("client Serve() did not stop")
	}
}

func TestWatchdogDeadlineStartsAfterPing(t *testing.T) {
	server := newTestServerWithConfig(t, Config{Timeout: 80 * time.Millisecond, PingInterval: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handshake := make(chan error, 1)
	go func() { handshake <- server.AcceptAndHandshake(ctx) }()
	conn, err := net.Dial("tcp", server.Address())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(Message{Version: ProtocolVersion, Type: MessageHello, Token: server.Token()}); err != nil {
		t.Fatalf("hello error = %v", err)
	}
	var ready Message
	if err := decoder.Decode(&ready); err != nil {
		t.Fatalf("ready error = %v", err)
	}
	if err := <-handshake; err != nil {
		t.Fatalf("AcceptAndHandshake() error = %v", err)
	}
	watch := server.Watch(ctx)
	var ping Message
	if err := decoder.Decode(&ping); err != nil {
		t.Fatalf("ping error = %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := encoder.Encode(Message{Version: ProtocolVersion, Type: MessagePong, Token: server.Token(), Sequence: ping.Sequence}); err != nil {
		t.Fatalf("pong error = %v", err)
	}
	select {
	case failure := <-watch:
		t.Fatalf("watchdog failed before pong = %s", failure)
	case <-time.After(20 * time.Millisecond):
		cancel()
	}
}

func TestRepeatedPingDoesNotExtendOutstandingDeadline(t *testing.T) {
	server := newTestServerWithConfig(t, Config{Timeout: 50 * time.Millisecond, PingInterval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handshake := make(chan error, 1)
	go func() { handshake <- server.AcceptAndHandshake(ctx) }()
	conn, err := net.Dial("tcp", server.Address())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(Message{Version: ProtocolVersion, Type: MessageHello, Token: server.Token()}); err != nil {
		t.Fatalf("hello error = %v", err)
	}
	var ready Message
	if err := decoder.Decode(&ready); err != nil {
		t.Fatalf("ready error = %v", err)
	}
	if err := <-handshake; err != nil {
		t.Fatalf("AcceptAndHandshake() error = %v", err)
	}
	watch := server.Watch(ctx)
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	var ping Message
	if err := decoder.Decode(&ping); err != nil {
		t.Fatalf("ping error = %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(15 * time.Millisecond)); err == nil {
		var second Message
		if err := decoder.Decode(&second); err == nil {
			t.Fatalf("received second outstanding ping: %#v", second)
		}
	}
	select {
	case failure := <-watch:
		if failure != FailureUnresponsive {
			t.Fatalf("failure = %s", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("watchdog did not expire")
	}
}

func TestWatchdogReportsConnectionClose(t *testing.T) {
	server := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handshake := make(chan error, 1)
	go func() { handshake <- server.AcceptAndHandshake(ctx) }()
	client, err := Connect(ctx, server.Address(), server.Token())
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if err := client.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}
	if err := <-handshake; err != nil {
		t.Fatalf("AcceptAndHandshake() error = %v", err)
	}
	watch := server.Watch(ctx)
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case failure, ok := <-watch:
		if ok {
			t.Fatalf("unexpected failure = %s", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("watchdog did not close after connection close")
	}
}

func TestWatchdogContextCancellationDoesNotReportUnresponsive(t *testing.T) {
	server := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	handshake := make(chan error, 1)
	go func() { handshake <- server.AcceptAndHandshake(ctx) }()
	client, err := Connect(ctx, server.Address(), server.Token())
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}
	if err := <-handshake; err != nil {
		t.Fatalf("AcceptAndHandshake() error = %v", err)
	}
	watch := server.Watch(ctx)
	cancel()
	select {
	case failure, ok := <-watch:
		if ok {
			t.Fatalf("unexpected failure = %s", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("watchdog did not close after cancellation")
	}
}

func TestOutputUpdateRelayWithHeartbeatInterleaved(t *testing.T) {
	server := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handshake := make(chan error, 1)
	go func() { handshake <- server.AcceptAndHandshake(ctx) }()
	client, err := Connect(ctx, server.Address(), server.Token())
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}
	if err := <-handshake; err != nil {
		t.Fatalf("AcceptAndHandshake() error = %v", err)
	}
	watch := server.Watch(ctx)
	serveDone := make(chan error, 1)
	go func() { serveDone <- client.Serve(ctx) }()
	for sequence := uint64(1); sequence <= 5; sequence++ {
		if err := client.SendOutputUpdate(sequence, OutputUpdate{Bytes: uint64(sequence * 100), Preview: "preview-line"}); err != nil {
			t.Fatalf("SendOutputUpdate(%d) error = %v", sequence, err)
		}
	}
	select {
	case failure := <-watch:
		t.Fatalf("unexpected watchdog failure = %s", failure)
	case <-time.After(3 * time.Duration(50) * time.Millisecond):
	}
	if count := server.OutputUpdateCount(); count != 5 {
		t.Fatalf("OutputUpdateCount() = %d", count)
	}
	cancel()
	select {
	case <-serveDone:
	case <-time.After(time.Second):
		t.Fatal("client Serve() did not stop")
	}
}

func TestOutputUpdateCapsAtLimit(t *testing.T) {
	server := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handshake := make(chan error, 1)
	go func() { handshake <- server.AcceptAndHandshake(ctx) }()
	client, err := Connect(ctx, server.Address(), server.Token())
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}
	if err := <-handshake; err != nil {
		t.Fatalf("AcceptAndHandshake() error = %v", err)
	}
	watch := server.Watch(ctx)
	go func() { _ = client.Serve(ctx) }()
	for sequence := uint64(1); sequence <= MaxOutputUpdates+50; sequence++ {
		if err := client.SendOutputUpdate(sequence, OutputUpdate{Bytes: uint64(sequence), Preview: "p"}); err != nil {
			t.Fatalf("SendOutputUpdate(%d) error = %v", sequence, err)
		}
	}
	time.Sleep(20 * time.Millisecond)
	if count := server.OutputUpdateCount(); count != MaxOutputUpdates {
		t.Fatalf("OutputUpdateCount() = %d, want %d", count, MaxOutputUpdates)
	}
	select {
	case failure := <-watch:
		t.Fatalf("unexpected watchdog failure = %s", failure)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestOutputUpdateRejectsInvalidToken(t *testing.T) {
	server := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handshake := make(chan error, 1)
	go func() { handshake <- server.AcceptAndHandshake(ctx) }()
	conn, err := net.Dial("tcp", server.Address())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)
	if err := encoder.Encode(Message{Version: ProtocolVersion, Type: MessageHello, Token: server.Token()}); err != nil {
		t.Fatalf("hello error = %v", err)
	}
	var ready Message
	if err := decoder.Decode(&ready); err != nil {
		t.Fatalf("ready error = %v", err)
	}
	if err := <-handshake; err != nil {
		t.Fatalf("AcceptAndHandshake() error = %v", err)
	}
	watch := server.Watch(ctx)
	if err := encoder.Encode(Message{Version: ProtocolVersion, Type: MessageOutputUpdate, Token: "wrong", Sequence: 1, Update: &OutputUpdate{Bytes: 1, Preview: "p"}}); err != nil {
		t.Fatalf("output update error = %v", err)
	}
	select {
	case failure := <-watch:
		if failure != FailureProtocol {
			t.Fatalf("failure = %s", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("watchdog did not report invalid output update")
	}
}

func newTestServer(t *testing.T) *Server {
	return newTestServerWithConfig(t, Config{Timeout: 200 * time.Millisecond, PingInterval: 20 * time.Millisecond})
}

func newTestServerWithConfig(t *testing.T, config Config) *Server {
	t.Helper()
	server, err := NewServer(config)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server
}
