// Package toolcontrol 承载 AI 工具与宿主之间的两套协议：
//   - 身份协议：hello / ready / ping / pong / output_update，负责握手、存活心跳与输出进度上报；
//   - AI工具-宿主运行时协议：capability_request / capability_response，负责工具执行期间
//     与宿主的一切双向交互；工具是主动发起方，宿主是被动服务方。
//
// 本文件是两套协议线格式的唯一事实来源。
package toolcontrol

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	ProtocolVersion     = 1
	MessageHello        = "hello"
	MessageReady        = "ready"
	MessagePing         = "ping"
	MessagePong         = "pong"
	MessageOutputUpdate = "output_update"
	// MessageCapabilityRequest 与 MessageCapabilityResponse 是运行时能力的
	// 双向消息：工具主动索取，宿主被动响应。
	MessageCapabilityRequest  = "capability_request"
	MessageCapabilityResponse = "capability_response"
)

// 能力请求的三种响应状态。
const (
	CapabilityStatusSuccess = "success"
	CapabilityStatusDenied  = "denied"
	CapabilityStatusFailed  = "failed"
)

// CapabilityDeniedMessage 是用户未授权某项能力时的统一失败反馈。
const CapabilityDeniedMessage = "用户未授权此能力，请前往工具设置页开启能力授权"

// MaxOutputUpdates caps how many output update messages a tool may relay per run.
const MaxOutputUpdates = 10_000

// MaxCapabilityRequests 限制单次执行中工具可发起的能力请求总数，
// 与输出更新上限同一防线：失控的工具不能无限打满宿主。
const MaxCapabilityRequests = 10_000

// MaxCapabilityConcurrency 限制宿主同时处理的能力请求数；
// 超出的请求在队列中等待服务，而不是被丢弃。
const MaxCapabilityConcurrency = 16

type Message struct {
	Version    int             `json:"version"`
	Type       string          `json:"type"`
	Token      string          `json:"token,omitempty"`
	Sequence   uint64          `json:"sequence,omitempty"`
	RequestID  string          `json:"requestId,omitempty"`
	Capability string          `json:"capability,omitempty"`
	Access     string          `json:"access,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Status     string          `json:"status,omitempty"`
	Error      string          `json:"error,omitempty"`
	Update     *OutputUpdate   `json:"update,omitempty"`
}

// CapabilityRequest 是宿主收到的一次运行时能力请求。
type CapabilityRequest struct {
	Capability string
	Access     string
	Payload    json.RawMessage
}

// CapabilityResult 是宿主对一次能力请求的裁决与响应数据。
type CapabilityResult struct {
	Status  string
	Error   string
	Payload any
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

func validateCapabilityRequest(message Message, expectedToken string) error {
	if message.Version != ProtocolVersion || message.Type != MessageCapabilityRequest || message.Token == "" || message.Token != expectedToken {
		return errInvalidMessage
	}
	if strings.TrimSpace(message.RequestID) == "" || strings.TrimSpace(message.Capability) == "" || strings.TrimSpace(message.Access) == "" {
		return errInvalidMessage
	}
	return nil
}

func validateCapabilityResponse(message Message, expectedToken string) error {
	if message.Version != ProtocolVersion || message.Type != MessageCapabilityResponse || message.Token == "" || message.Token != expectedToken {
		return errInvalidMessage
	}
	if strings.TrimSpace(message.RequestID) == "" {
		return errInvalidMessage
	}
	switch message.Status {
	case CapabilityStatusSuccess, CapabilityStatusDenied, CapabilityStatusFailed:
		return nil
	default:
		return errInvalidMessage
	}
}
