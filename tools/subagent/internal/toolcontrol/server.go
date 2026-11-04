package toolcontrol

import (
	"context"
	"encoding/json"
	"net"
	"time"
)

type messageResult struct {
	message Message
	err     error
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

func setConnectionDeadline(conn net.Conn, ctx context.Context) error {
	if deadline, ok := ctx.Deadline(); ok {
		return conn.SetDeadline(deadline)
	}
	return conn.SetDeadline(time.Now().Add(10 * time.Second))
}
