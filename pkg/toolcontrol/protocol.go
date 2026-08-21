package toolcontrol

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	ProtocolVersion     = 1
	MessageHello        = "hello"
	MessageReady        = "ready"
	MessagePing         = "ping"
	MessagePong         = "pong"
	MessageOutputUpdate = "output_update"
)

// MaxOutputUpdates caps how many output update messages a tool may relay per run.
const MaxOutputUpdates = 10_000

type Message struct {
	Version  int           `json:"version"`
	Type     string        `json:"type"`
	Token    string        `json:"token,omitempty"`
	Sequence uint64        `json:"sequence,omitempty"`
	Update   *OutputUpdate `json:"update,omitempty"`
}

type OutputUpdate struct {
	Bytes   uint64 `json:"bytes"`
	Preview string `json:"preview"`
}

var errInvalidMessage = errors.New("invalid tool control message")

func newToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate tool control token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func validateHello(message Message, expectedToken string) error {
	if message.Version != ProtocolVersion || message.Type != MessageHello || message.Token == "" || message.Token != expectedToken {
		return errInvalidMessage
	}
	return nil
}

func validateReady(message Message, expectedToken string) error {
	if message.Version != ProtocolVersion || message.Type != MessageReady || message.Token == "" || message.Token != expectedToken || message.Sequence != 0 {
		return errInvalidMessage
	}
	return nil
}

func validatePing(message Message, expectedToken string) error {
	if message.Version != ProtocolVersion || message.Type != MessagePing || message.Token == "" || message.Token != expectedToken || message.Sequence == 0 {
		return errInvalidMessage
	}
	return nil
}

func validatePong(message Message, expectedToken string, expectedSequence uint64) error {
	if message.Version != ProtocolVersion || message.Type != MessagePong || message.Token == "" || message.Token != expectedToken || message.Sequence != expectedSequence {
		return errInvalidMessage
	}
	return nil
}

func validateOutputUpdate(message Message, expectedToken string) error {
	if message.Version != ProtocolVersion || message.Type != MessageOutputUpdate || message.Token == "" || message.Token != expectedToken || message.Sequence == 0 || message.Update == nil {
		return errInvalidMessage
	}
	return nil
}
