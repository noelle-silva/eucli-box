package toolcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Timeout      time.Duration
	PingInterval time.Duration
}

type FailureKind string

const (
	FailureUnresponsive FailureKind = "tool_unresponsive"
	FailureProtocol     FailureKind = "tool_protocol_failed"
)

type Server struct {
	listener net.Listener
	config   Config
	token    string

	mu            sync.Mutex
	conn          net.Conn
	decoder       *json.Decoder
	encoder       *json.Encoder
	writeMu       sync.Mutex
	closeOnce     sync.Once
	outputUpdates uint64
}

func NewServer(config Config) (*Server, error) {
	if config.Timeout <= 0 {
		return nil, errors.New("tool control timeout must be positive")
	}
	if config.PingInterval <= 0 {
		return nil, errors.New("tool control ping interval must be positive")
	}
	if config.PingInterval >= config.Timeout {
		return nil, errors.New("tool control ping interval must be less than timeout")
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for tool control: %w", err)
	}
	return &Server{listener: listener, config: config, token: token}, nil
}

func (s *Server) Address() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) Token() string {
	if s == nil {
		return ""
	}
	return s.token
}

func (s *Server) AcceptAndHandshake(ctx context.Context) error {
	if s == nil || s.listener == nil {
		return errors.New("tool control server is not initialized")
	}
	conn, err := acceptWithContext(ctx, s.listener)
	if err != nil {
		return err
	}
	s.listener.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)
	message, err := decodeWithContext(ctx, conn, decoder)
	if err != nil {
		conn.Close()
		return fmt.Errorf("read tool control hello: %w", err)
	}
	if err := validateHello(message, s.token); err != nil {
		conn.Close()
		return err
	}
	if err := setConnectionDeadline(conn, ctx); err != nil {
		conn.Close()
		return err
	}
	if err := encoder.Encode(Message{Version: ProtocolVersion, Type: MessageReady, Token: s.token}); err != nil {
		conn.Close()
		return fmt.Errorf("write tool control ready: %w", err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return fmt.Errorf("clear tool control deadline: %w", err)
	}
	s.mu.Lock()
	s.conn = conn
	s.decoder = decoder
	s.encoder = encoder
	s.mu.Unlock()
	return nil
}

func (s *Server) Watch(ctx context.Context) <-chan FailureKind {
	result := make(chan FailureKind, 1)
	s.mu.Lock()
	conn := s.conn
	decoder := s.decoder
	s.mu.Unlock()
	if conn == nil || decoder == nil {
		result <- FailureProtocol
		close(result)
		return result
	}
	go func() {
		defer close(result)
		defer conn.Close()
		messages := make(chan messageResult, 1)
		go readMessages(decoder, messages)

		ticker := time.NewTicker(s.config.PingInterval)
		defer ticker.Stop()
		var pendingSequence uint64
		var nextSequence uint64
		var deadlineTimer *time.Timer
		var deadline <-chan time.Time

		fail := func(kind FailureKind) {
			result <- kind
		}
		for {
			select {
			case <-ctx.Done():
				stopTimer(deadlineTimer)
				return
			case <-ticker.C:
				if pendingSequence != 0 {
					continue
				}
				nextSequence++
				message := Message{Version: ProtocolVersion, Type: MessagePing, Token: s.token, Sequence: nextSequence}
				if err := s.write(message); err != nil {
					fail(FailureProtocol)
					return
				}
				pendingSequence = nextSequence
				stopTimer(deadlineTimer)
				deadlineTimer = time.NewTimer(s.config.Timeout)
				deadline = deadlineTimer.C
			case <-deadline:
				fail(FailureUnresponsive)
				return
			case message := <-messages:
				if message.err != nil {
					if ctx.Err() != nil {
						return
					}
					if errors.Is(message.err, io.EOF) || isClosedConnError(message.err) {
						return
					}
					fail(FailureProtocol)
					return
				}
				if message.message.Type == MessageOutputUpdate {
					if err := validateOutputUpdate(message.message, s.token); err != nil {
						fail(FailureProtocol)
						return
					}
					s.mu.Lock()
					if s.outputUpdates < MaxOutputUpdates {
						s.outputUpdates++
					}
					s.mu.Unlock()
					continue
				}
				if err := validatePong(message.message, s.token, pendingSequence); err != nil || pendingSequence == 0 {
					fail(FailureProtocol)
					return
				}
				pendingSequence = 0
				stopTimer(deadlineTimer)
				deadlineTimer = nil
				deadline = nil
			}
		}
	}()
	return result
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	var err error
	s.closeOnce.Do(func() {
		if s.listener != nil {
			err = s.listener.Close()
		}
		s.mu.Lock()
		conn := s.conn
		s.mu.Unlock()
		if conn != nil {
			if closeErr := conn.Close(); err == nil {
				err = closeErr
			}
		}
	})
	return err
}

// OutputUpdateCount returns how many output updates the host has accepted.
func (s *Server) OutputUpdateCount() uint64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outputUpdates
}

func (s *Server) write(message Message) error {
	s.mu.Lock()
	conn := s.conn
	encoder := s.encoder
	s.mu.Unlock()
	if conn == nil || encoder == nil {
		return errors.New("tool control connection is not ready")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := conn.SetWriteDeadline(time.Now().Add(s.config.Timeout)); err != nil {
		return err
	}
	if err := encoder.Encode(message); err != nil {
		return err
	}
	return conn.SetWriteDeadline(time.Time{})
}

type messageResult struct {
	message Message
	err     error
}

func readMessages(decoder *json.Decoder, result chan<- messageResult) {
	for {
		var message Message
		if err := decoder.Decode(&message); err != nil {
			result <- messageResult{err: err}
			return
		}
		result <- messageResult{message: message}
	}
}

func decodeWithContext(ctx context.Context, conn net.Conn, decoder *json.Decoder) (Message, error) {
	result := make(chan messageResult, 1)
	go func() {
		var message Message
		err := decoder.Decode(&message)
		result <- messageResult{message: message, err: err}
	}()
	select {
	case decoded := <-result:
		return decoded.message, decoded.err
	case <-ctx.Done():
		conn.Close()
		return Message{}, ctx.Err()
	}
}

func acceptWithContext(ctx context.Context, listener net.Listener) (net.Conn, error) {
	result := make(chan struct {
		conn net.Conn
		err  error
	}, 1)
	go func() {
		conn, err := listener.Accept()
		result <- struct {
			conn net.Conn
			err  error
		}{conn: conn, err: err}
	}()
	select {
	case accepted := <-result:
		return accepted.conn, accepted.err
	case <-ctx.Done():
		listener.Close()
		return nil, ctx.Err()
	}
}

func setConnectionDeadline(conn net.Conn, ctx context.Context) error {
	if deadline, ok := ctx.Deadline(); ok {
		return conn.SetDeadline(deadline)
	}
	return conn.SetDeadline(time.Now().Add(10 * time.Second))
}

func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func isClosedConnError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "use of closed network connection") || strings.Contains(text, "broken pipe") || strings.Contains(text, "connection reset") || strings.Contains(text, "connection refused")
}
