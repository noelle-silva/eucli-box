package toolcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

type Client struct {
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

func Connect(ctx context.Context, address string, token string) (*Client, error) {
	if address == "" || token == "" {
		return nil, errors.New("tool control address and token are required")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connect tool control: %w", err)
	}
	client := &Client{conn: conn, token: token, decoder: json.NewDecoder(conn), encoder: json.NewEncoder(conn), done: make(chan struct{}), pending: map[string]chan Message{}}
	if err := setConnectionDeadline(conn, ctx); err != nil {
		conn.Close()
		return nil, err
	}
	if err := client.write(Message{Version: ProtocolVersion, Type: MessageHello, Token: token}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write tool control hello: %w", err)
	}
	return client, nil
}

func (c *Client) WaitReady(ctx context.Context) error {
	if c == nil || c.conn == nil || c.decoder == nil {
		return errors.New("tool control client is not initialized")
	}
	message, err := decodeWithContext(ctx, c.conn, c.decoder)
	if err != nil {
		return fmt.Errorf("read tool control ready: %w", err)
	}
	if err := validateReady(message, c.token); err != nil {
		return err
	}
	return c.conn.SetDeadline(time.Time{})
}

func (c *Client) Serve(ctx context.Context) error {
	if c == nil || c.conn == nil || c.decoder == nil {
		return errors.New("tool control client is not initialized")
	}
	for {
		message, err := decodeWithContext(ctx, c.conn, c.decoder)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		switch message.Type {
		case MessagePing:
			if err := validatePing(message, c.token); err != nil {
				return err
			}
			if err := c.write(Message{Version: ProtocolVersion, Type: MessagePong, Token: c.token, Sequence: message.Sequence}); err != nil {
				return err
			}
		case MessageCapabilityResponse:
			if err := c.dispatchCapabilityResponse(message); err != nil {
				return err
			}
		default:
			return errInvalidMessage
		}
	}
}

// dispatchCapabilityResponse 把宿主的能力响应交给等待中的请求；
// 已超时或未知请求的迟到响应直接忽略，不视为协议损坏。
func (c *Client) dispatchCapabilityResponse(message Message) error {
	if err := validateCapabilityResponse(message, c.token); err != nil {
		return err
	}
	c.pendingMu.Lock()
	waiter := c.pending[message.RequestID]
	c.pendingMu.Unlock()
	if waiter == nil {
		return nil
	}
	select {
	case waiter <- message:
	default:
	}
	return nil
}

// Request 向宿主发出一次运行时能力请求并等待响应。
// 请求的等待由调用方上下文控制；通道关闭或请求超时都返回错误。
func (c *Client) Request(ctx context.Context, capability string, access string, payload any) (CapabilityResult, error) {
	if c == nil || c.conn == nil || c.encoder == nil {
		return CapabilityResult{}, errors.New("tool control client is closed")
	}
	var encoded json.RawMessage
	if payload != nil {
		bytes, err := json.Marshal(payload)
		if err != nil {
			return CapabilityResult{}, fmt.Errorf("encode capability request: %w", err)
		}
		encoded = bytes
	}
	requestID, err := newToken()
	if err != nil {
		return CapabilityResult{}, err
	}
	waiter := make(chan Message, 1)
	c.pendingMu.Lock()
	c.pending[requestID] = waiter
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, requestID)
		c.pendingMu.Unlock()
	}()
	message := Message{Version: ProtocolVersion, Type: MessageCapabilityRequest, Token: c.token, RequestID: requestID, Capability: capability, Access: access, Payload: encoded}
	if err := c.write(message); err != nil {
		return CapabilityResult{}, fmt.Errorf("write capability request: %w", err)
	}
	select {
	case response := <-waiter:
		return CapabilityResult{Status: response.Status, Error: response.Error, Payload: response.Payload}, nil
	case <-ctx.Done():
		return CapabilityResult{}, ctx.Err()
	case <-c.done:
		return CapabilityResult{}, errors.New("tool control client is closed")
	}
}

// SendOutputUpdate relays one output update to the host with a short write
// deadline so a congested connection can never stall the command output for
// long. Best-effort: callers must not block their command execution on it.
func (c *Client) SendOutputUpdate(sequence uint64, update OutputUpdate) error {
	if c == nil || c.conn == nil || c.encoder == nil {
		return errors.New("tool control client is closed")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}
	if err := c.encoder.Encode(Message{Version: ProtocolVersion, Type: MessageOutputUpdate, Token: c.token, Sequence: sequence, Update: &update}); err != nil {
		return err
	}
	return c.conn.SetWriteDeadline(time.Time{})
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	var err error
	c.closeOnce.Do(func() {
		close(c.done)
		if c.conn != nil {
			err = c.conn.Close()
		}
	})
	return err
}

func (c *Client) write(message Message) error {
	if c.conn == nil || c.encoder == nil {
		return errors.New("tool control client is closed")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	if err := c.encoder.Encode(message); err != nil {
		return err
	}
	return c.conn.SetWriteDeadline(time.Time{})
}
