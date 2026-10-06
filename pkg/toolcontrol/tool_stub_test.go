package toolcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"
)

// toolStub 是宿主测试内自带的最小工具侧桩：只按协议线格式与服务端交互，
// 不依赖任何已移出的工具侧实现，用于替代真实工具客户端驱动服务端测试。
type toolStub struct {
	conn      net.Conn
	token     string
	decoder   *json.Decoder
	encoder   *json.Encoder
	writeMu   sync.Mutex
	closeOnce sync.Once
	done      chan struct{}

	pendingMu sync.Mutex
	pending   map[string]chan Message
}

func connectToolStub(ctx context.Context, address string, token string) (*toolStub, error) {
	if address == "" || token == "" {
		return nil, errors.New("tool stub address and token are required")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	stub := &toolStub{conn: conn, token: token, decoder: json.NewDecoder(conn), encoder: json.NewEncoder(conn), done: make(chan struct{}), pending: map[string]chan Message{}}
	if err := stub.write(Message{Version: ProtocolVersion, Type: MessageHello, Token: token}); err != nil {
		conn.Close()
		return nil, err
	}
	return stub, nil
}

func (s *toolStub) waitReady(ctx context.Context) error {
	if s == nil || s.conn == nil || s.decoder == nil {
		return errors.New("tool stub is not initialized")
	}
	message, err := decodeWithContext(ctx, s.conn, s.decoder)
	if err != nil {
		return err
	}
	if message.Version != ProtocolVersion || message.Type != MessageReady || message.Token != s.token {
		return errInvalidMessage
	}
	return nil
}

func (s *toolStub) serve(ctx context.Context) error {
	if s == nil || s.conn == nil || s.decoder == nil {
		return errors.New("tool stub is not initialized")
	}
	for {
		message, err := decodeWithContext(ctx, s.conn, s.decoder)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		switch message.Type {
		case MessagePing:
			if message.Version != ProtocolVersion || message.Token != s.token || message.Sequence == 0 {
				return errInvalidMessage
			}
			if err := s.write(Message{Version: ProtocolVersion, Type: MessagePong, Token: s.token, Sequence: message.Sequence}); err != nil {
				return err
			}
		case MessageCapabilityResponse:
			if message.Version != ProtocolVersion || message.Token != s.token {
				return errInvalidMessage
			}
			s.dispatchCapabilityResponse(message)
		default:
			return errInvalidMessage
		}
	}
}

func (s *toolStub) dispatchCapabilityResponse(message Message) {
	s.pendingMu.Lock()
	waiter := s.pending[message.RequestID]
	s.pendingMu.Unlock()
	if waiter == nil {
		return
	}
	select {
	case waiter <- message:
	default:
	}
}

func (s *toolStub) request(ctx context.Context, capability string, access string, payload any) (CapabilityResult, error) {
	if s == nil || s.conn == nil || s.encoder == nil {
		return CapabilityResult{}, errors.New("tool stub is closed")
	}
	var encoded json.RawMessage
	if payload != nil {
		bytes, err := json.Marshal(payload)
		if err != nil {
			return CapabilityResult{}, err
		}
		encoded = bytes
	}
	requestID, err := newToken()
	if err != nil {
		return CapabilityResult{}, err
	}
	waiter := make(chan Message, 1)
	s.pendingMu.Lock()
	s.pending[requestID] = waiter
	s.pendingMu.Unlock()
	defer func() {
		s.pendingMu.Lock()
		delete(s.pending, requestID)
		s.pendingMu.Unlock()
	}()
	if err := s.write(Message{Version: ProtocolVersion, Type: MessageCapabilityRequest, Token: s.token, RequestID: requestID, Capability: capability, Access: access, Payload: encoded}); err != nil {
		return CapabilityResult{}, err
	}
	select {
	case response := <-waiter:
		return CapabilityResult{Status: response.Status, Error: response.Error, Payload: response.Payload}, nil
	case <-ctx.Done():
		return CapabilityResult{}, ctx.Err()
	case <-s.done:
		return CapabilityResult{}, errors.New("tool stub is closed")
	}
}

func (s *toolStub) sendOutputUpdate(sequence uint64, update OutputUpdate) error {
	if s == nil || s.conn == nil || s.encoder == nil {
		return errors.New("tool stub is closed")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}
	if err := s.encoder.Encode(Message{Version: ProtocolVersion, Type: MessageOutputUpdate, Token: s.token, Sequence: sequence, Update: &update}); err != nil {
		return err
	}
	return s.conn.SetWriteDeadline(time.Time{})
}

func (s *toolStub) close() error {
	if s == nil {
		return nil
	}
	var err error
	s.closeOnce.Do(func() {
		close(s.done)
		if s.conn != nil {
			err = s.conn.Close()
		}
	})
	return err
}

func (s *toolStub) write(message Message) error {
	if s.conn == nil || s.encoder == nil {
		return errors.New("tool stub is closed")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	if err := s.encoder.Encode(message); err != nil {
		return err
	}
	return s.conn.SetWriteDeadline(time.Time{})
}
