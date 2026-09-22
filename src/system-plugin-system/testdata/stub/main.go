// 这是系统插件系统测试用的桩插件：只依赖标准库，按目录内 stub.json 描述行为。
// 它独立手写新协议的最小实现，既驱动宿主，也反向验证线协议本身。
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type capability struct {
	Type       string   `json:"type"`
	Interfaces []string `json:"interfaces,omitempty"`
}

type message struct {
	ProtocolVersion int               `json:"protocolVersion"`
	Type            string            `json:"type"`
	RequestID       string            `json:"requestId,omitempty"`
	Sequence        uint64            `json:"sequence,omitempty"`
	PluginID        string            `json:"pluginId,omitempty"`
	DataDirectory   string            `json:"dataDirectory,omitempty"`
	Capabilities    []capability      `json:"capabilities,omitempty"`
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

type behavior struct {
	Handshake      string            `json:"handshake"`
	Capabilities   []capability      `json:"capabilities"`
	NoReady        bool              `json:"noReady"`
	CrashOnStart   bool              `json:"crashOnStart"`
	ExitAfterReady bool              `json:"exitAfterReady"`
	IgnoreStop     bool              `json:"ignoreStop"`
	IgnorePing     bool              `json:"ignorePing"`
	InvokeDelayMs  int               `json:"invokeDelayMs"`
	WaitForFile    string            `json:"waitForFile"`
	CreateFile     string            `json:"createFile"`
	ResponseFile   string            `json:"responseFile"`
	Status         string            `json:"status"`
	Error          string            `json:"error"`
	Values         map[string]string `json:"values"`
	TrackFile      string            `json:"trackFile"`
	EmitEvent      *struct {
		Name    string         `json:"name"`
		Payload map[string]any `json:"payload"`
	} `json:"emitEvent"`
}

var (
	writeMu          sync.Mutex
	encoder          *json.Encoder
	workingDirectory string
	settings         behavior
	trackMu          sync.Mutex
)

func main() {
	workingDirectory, _ = os.Getwd()
	if payload, err := os.ReadFile(filepath.Join(workingDirectory, "stub.json")); err == nil {
		_ = json.Unmarshal(payload, &settings)
	}
	if settings.CrashOnStart {
		os.Exit(3)
	}
	encoder = json.NewEncoder(os.Stdout)
	decoder := json.NewDecoder(os.Stdin)
	var hello message
	if err := decoder.Decode(&hello); err != nil {
		os.Exit(4)
	}
	if settings.NoReady {
		time.Sleep(time.Hour)
	}
	pluginID := hello.PluginID
	if settings.Handshake == "plugin-id-mismatch" {
		pluginID = "someone-else"
	}
	protocolVersion := hello.ProtocolVersion
	if settings.Handshake == "protocol-mismatch" {
		protocolVersion = hello.ProtocolVersion + 1
	}
	capabilities := settings.Capabilities
	if capabilities == nil {
		capabilities = []capability{{Type: "placeholder-values", Interfaces: []string{"value"}}}
	}
	record(map[string]any{
		"kind":             "hello",
		"pluginId":         hello.PluginID,
		"dataDirectory":    hello.DataDirectory,
		"workingDirectory": workingDirectory,
		"userConfig":       hello.UserConfig,
		"defaultConfig":    hello.DefaultConfig,
	})
	write(message{ProtocolVersion: protocolVersion, Type: "ready", PluginID: pluginID, Capabilities: capabilities})
	if settings.ExitAfterReady {
		os.Exit(6)
	}
	if settings.EmitEvent != nil {
		write(message{ProtocolVersion: protocolVersion, Type: "event", Event: settings.EmitEvent.Name, Payload: settings.EmitEvent.Payload})
	}
	var inFlight sync.WaitGroup
	for {
		var incoming message
		if err := decoder.Decode(&incoming); err != nil {
			os.Exit(0)
		}
		switch incoming.Type {
		case "invoke":
			inFlight.Add(1)
			go func(item message) {
				defer inFlight.Done()
				handleInvoke(item)
			}(incoming)
		case "config":
			record(map[string]any{"kind": "config", "userConfig": incoming.UserConfig, "defaultConfig": incoming.DefaultConfig})
		case "ping":
			if !settings.IgnorePing {
				write(message{ProtocolVersion: incoming.ProtocolVersion, Type: "pong", Sequence: incoming.Sequence})
			}
		case "stop":
			if settings.IgnoreStop {
				continue
			}
			os.Exit(0)
		}
	}
}

func handleInvoke(incoming message) {
	if settings.CreateFile != "" {
		_ = os.WriteFile(settings.CreateFile, []byte("ready"), 0o644)
	}
	for settings.WaitForFile != "" {
		if _, err := os.Stat(settings.WaitForFile); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if settings.InvokeDelayMs > 0 {
		time.Sleep(time.Duration(settings.InvokeDelayMs) * time.Millisecond)
	}
	record(map[string]any{"kind": "invoke", "capability": incoming.Capability, "interfaces": incoming.Interfaces})
	status := settings.Status
	if status == "" {
		status = "success"
	}
	values := settings.Values
	errText := settings.Error
	if settings.ResponseFile != "" {
		if payload, err := os.ReadFile(filepath.Join(workingDirectory, settings.ResponseFile)); err == nil {
			var response struct {
				Status string            `json:"status"`
				Values map[string]string `json:"values"`
				Error  string            `json:"error"`
			}
			if json.Unmarshal(payload, &response) == nil {
				if response.Status != "" {
					status = response.Status
				}
				values = response.Values
				errText = response.Error
			}
		}
	}
	write(message{ProtocolVersion: incoming.ProtocolVersion, Type: "result", RequestID: incoming.RequestID, Status: status, Values: values, Error: errText})
}

func write(outgoing message) {
	writeMu.Lock()
	defer writeMu.Unlock()
	_ = encoder.Encode(outgoing)
}

func record(entry map[string]any) {
	if settings.TrackFile == "" {
		return
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return
	}
	trackMu.Lock()
	defer trackMu.Unlock()
	file, err := os.OpenFile(settings.TrackFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(payload, '\n'))
}
