// Package toolcontrol 承载 AI 工具与宿主之间的两套协议：
//   - 身份协议：hello / ready / ping / pong / output_update，负责握手、存活心跳与输出进度上报；
//   - AI工具-宿主运行时协议：capability_request / capability_response，负责工具执行期间
//     与宿主的一切双向交互；工具是主动发起方，宿主是被动服务方。
//
// 本文件是两套协议线格式的唯一事实来源。
package toolcontrol

import (
	"encoding/json"
	"errors"
	"strings"
)

const (
	ProtocolVersion = 1
	MessageHello    = "hello"
	MessageReady    = "ready"
	MessagePing     = "ping"
	MessagePong     = "pong"

	// MessageCapabilityRequest 与 MessageCapabilityResponse 是运行时能力的
	// 双向消息：工具主动索取，宿主被动响应。

	MessageCapabilityResponse = "capability_response"
)

// 能力请求的三种响应状态。
const (
	CapabilityStatusSuccess = "success"
	CapabilityStatusDenied  = "denied"
	CapabilityStatusFailed  = "failed"
)

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

type OutputUpdate struct {
	Bytes   uint64 `json:"bytes"`
	Preview string `json:"preview"`
}

var errInvalidMessage = errors.New("invalid tool control message")

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
