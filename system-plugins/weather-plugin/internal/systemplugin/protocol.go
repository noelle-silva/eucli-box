// Package systemplugin 定义系统插件控制通道的线协议：
// 主机与插件共用同一套消息类型、能力类型与版本约束，是协议事实的唯一来源。
// 通道本身与传输无关，默认承载在宿主子进程的标准输入输出上（按行 JSON）。
package systemplugin

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ProtocolVersion 是当前控制通道协议版本；协议换代不做旧版兼容。
const ProtocolVersion = 1

const (
	MessageHello  = "hello"
	MessageReady  = "ready"
	MessageInvoke = "invoke"
	MessageCancel = "cancel"
	MessageConfig = "config"
	MessageStop   = "stop"
	MessagePing   = "ping"
	MessagePong   = "pong"
	MessageResult = "result"
	MessageEvent  = "event"
)

const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

// CapabilityPlaceholderValues 是「占位符取值」能力的类型标识。
const CapabilityPlaceholderValues = "placeholder-values"

// Capability 是插件在握手中声明的能力：类型 + 该能力对外暴露的接口标识。
type Capability struct {
	Type       string   `json:"type"`
	Interfaces []string `json:"interfaces,omitempty"`
}

// Message 是控制通道上的统一消息；未使用字段按类型省略。
//   - 宿主 -> 插件：hello、invoke、cancel、config、stop、ping
//   - 插件 -> 宿主：ready、result、event、pong
type Message struct {
	ProtocolVersion int               `json:"protocolVersion"`
	Type            string            `json:"type"`
	RequestID       string            `json:"requestId,omitempty"`
	Sequence        uint64            `json:"sequence,omitempty"`
	PluginID        string            `json:"pluginId,omitempty"`
	HostVersion     string            `json:"hostVersion,omitempty"`
	DataDirectory   string            `json:"dataDirectory,omitempty"`
	Capabilities    []Capability      `json:"capabilities,omitempty"`
	Capability      string            `json:"capability,omitempty"`
	Interfaces      []string          `json:"interfaces,omitempty"`
	UserConfig      map[string]any    `json:"userConfig,omitempty"`
	DefaultConfig   map[string]any    `json:"defaultConfig,omitempty"`
	Status          string            `json:"status,omitempty"`
	Values          map[string]string `json:"values,omitempty"`
	Error           string            `json:"error,omitempty"`
	Event           string            `json:"event,omitempty"`
	Payload         map[string]any    `json:"payload,omitempty"`
}

// Write 把消息按行写入通道。
func Write(encoder *json.Encoder, message Message) error {
	if encoder == nil {
		return fmt.Errorf("control channel writer is not available")
	}
	if message.ProtocolVersion == 0 {
		message.ProtocolVersion = ProtocolVersion
	}
	return encoder.Encode(message)
}

// Read 从通道读取一条消息；调用方负责按需设置读取期限。
func Read(decoder *json.Decoder) (Message, error) {
	var message Message
	if err := decoder.Decode(&message); err != nil {
		return Message{}, err
	}
	return message, nil
}

// ValidateHello 校验宿主握手：版本一致、类型正确、插件身份一致。
func ValidateHello(message Message, pluginID string) error {
	if message.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("control protocol version mismatch: host %d, plugin %d", message.ProtocolVersion, ProtocolVersion)
	}
	if message.Type != MessageHello {
		return fmt.Errorf("control handshake expected %q, got %q", MessageHello, message.Type)
	}
	if strings.TrimSpace(message.PluginID) != strings.TrimSpace(pluginID) {
		return fmt.Errorf("control handshake plugin identity mismatch: host %q, plugin %q", message.PluginID, pluginID)
	}
	return nil
}
