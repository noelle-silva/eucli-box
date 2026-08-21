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
}

func Connect(ctx context.Context, address string, token string) (*Client, error) {
	if address == "" || token == "" {
		return nil, errors.New("tool control address and token are required")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connect tool control: %w", err)
	}
	client := &Client{conn: conn, token: token, decoder: json.NewDecoder(conn), encoder: json.NewEncoder(conn)}
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
		if err := validatePing(message, c.token); err != nil {
			return err
		}
		if err := c.write(Message{Version: ProtocolVersion, Type: MessagePong, Token: c.token, Sequence: message.Sequence}); err != nil {
			return err
		}
	}
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	var err error
	c.closeOnce.Do(func() {
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
