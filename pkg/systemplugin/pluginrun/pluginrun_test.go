package pluginrun

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"eucli-box/pkg/systemplugin"
)

// TestServePluginProcess 以子进程方式运行 Serve：父进程扮演宿主，
// 按线协议握手、配置、调用、取消、心跳与停机，验证插件侧实现与协议一致。
func TestServePluginProcess(t *testing.T) {
	if os.Getenv("PLUGINRUN_HELPER") == "1" {
		serveHelperProcess()
		return
	}
	command := exec.Command(os.Args[0], "-test.run=TestServePluginProcess")
	command.Env = append(os.Environ(), "PLUGINRUN_HELPER=1")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe() error = %v", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe() error = %v", err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_, _ = command.Process.Wait()
	})
	encoder := json.NewEncoder(stdin)
	decoder := json.NewDecoder(stdout)

	hello := systemplugin.NewMessage(systemplugin.MessageHello)
	hello.PluginID = "helper-plugin"
	hello.DataDirectory = t.TempDir()
	hello.UserConfig = map[string]any{"mode": "fast"}
	if err := encoder.Encode(hello); err != nil {
		t.Fatalf("write hello error = %v", err)
	}
	ready := readMessage(t, decoder)
	if err := systemplugin.ValidateReady(ready, "helper-plugin", helperCapabilities()); err != nil {
		t.Fatalf("ValidateReady() error = %v", err)
	}
	event := readMessage(t, decoder)
	if event.Type != systemplugin.MessageEvent || event.Event != "helper-ready" {
		t.Fatalf("event = %#v", event)
	}

	result := invoke(t, encoder, decoder, "value")
	if result.Status != systemplugin.StatusSuccess || result.Values["value"] != "fast" {
		t.Fatalf("result = %#v", result)
	}
	configMessage := systemplugin.NewMessage(systemplugin.MessageConfig)
	configMessage.UserConfig = map[string]any{"mode": "slow"}
	if err := encoder.Encode(configMessage); err != nil {
		t.Fatalf("write config error = %v", err)
	}
	result = invoke(t, encoder, decoder, "value")
	if result.Status != systemplugin.StatusSuccess || result.Values["value"] != "slow" {
		t.Fatalf("result after config = %#v", result)
	}

	ping := systemplugin.Message{ProtocolVersion: systemplugin.ProtocolVersion, Type: systemplugin.MessagePing, Sequence: 7}
	if err := encoder.Encode(ping); err != nil {
		t.Fatalf("write ping error = %v", err)
	}
	pong := readMessage(t, decoder)
	if pong.Type != systemplugin.MessagePong || pong.Sequence != 7 {
		t.Fatalf("pong = %#v", pong)
	}

	// 取消：慢调用在收到取消后以失败结果返回。
	cancelRequest := systemplugin.NewMessage(systemplugin.MessageInvoke)
	cancelRequest.RequestID = "cancel-me"
	cancelRequest.Capability = systemplugin.CapabilityPlaceholderValues
	cancelRequest.Interfaces = []string{"slow"}
	if err := encoder.Encode(cancelRequest); err != nil {
		t.Fatalf("write slow invoke error = %v", err)
	}
	cancelMessage := systemplugin.NewMessage(systemplugin.MessageCancel)
	cancelMessage.RequestID = "cancel-me"
	if err := encoder.Encode(cancelMessage); err != nil {
		t.Fatalf("write cancel error = %v", err)
	}
	late := readMessage(t, decoder)
	if late.Type != systemplugin.MessageResult || late.RequestID != "cancel-me" || late.Status != systemplugin.StatusFailed {
		t.Fatalf("cancelled result = %#v", late)
	}

	if err := encoder.Encode(systemplugin.NewMessage(systemplugin.MessageStop)); err != nil {
		t.Fatalf("write stop error = %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("plugin process exit error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("plugin process did not stop in time")
	}
}

func serveHelperProcess() {
	provider := &helperProvider{}
	err := Serve(Options{
		PluginID:     "helper-plugin",
		Capabilities: helperCapabilities(),
		Placeholder:  provider,
		Observe: func(sender Sender) {
			_ = sender("helper-ready", map[string]any{"ok": true})
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func helperCapabilities() []systemplugin.Capability {
	return []systemplugin.Capability{{Type: systemplugin.CapabilityPlaceholderValues, Interfaces: []string{"value", "slow"}}}
}

type helperProvider struct {
	mu     sync.Mutex
	config Config
}

func (p *helperProvider) Configure(config Config) {
	p.mu.Lock()
	p.config = config
	p.mu.Unlock()
}

func (p *helperProvider) ResolvePlaceholders(ctx context.Context, interfaceIDs []string) (map[string]string, error) {
	p.mu.Lock()
	mode, _ := p.config.UserConfig["mode"].(string)
	p.mu.Unlock()
	values := map[string]string{}
	for _, interfaceID := range interfaceIDs {
		switch interfaceID {
		case "value":
			values[interfaceID] = mode
		case "slow":
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(3 * time.Second):
				values[interfaceID] = "late"
			}
		}
	}
	return values, nil
}

func invoke(t *testing.T, encoder *json.Encoder, decoder *json.Decoder, interfaceID string) systemplugin.Message {
	t.Helper()
	requestID := "invoke-" + interfaceID
	message := systemplugin.NewMessage(systemplugin.MessageInvoke)
	message.RequestID = requestID
	message.Capability = systemplugin.CapabilityPlaceholderValues
	message.Interfaces = []string{interfaceID}
	if err := encoder.Encode(message); err != nil {
		t.Fatalf("write invoke error = %v", err)
	}
	result := readMessage(t, decoder)
	if result.Type != systemplugin.MessageResult || result.RequestID != requestID {
		t.Fatalf("result = %#v", result)
	}
	return result
}

func readMessage(t *testing.T, decoder *json.Decoder) systemplugin.Message {
	t.Helper()
	type readResult struct {
		message systemplugin.Message
		err     error
	}
	channel := make(chan readResult, 1)
	go func() {
		message, err := systemplugin.Read(decoder)
		channel <- readResult{message: message, err: err}
	}()
	select {
	case item := <-channel:
		if item.err != nil {
			t.Fatalf("read message error = %v", item.err)
		}
		return item.message
	case <-time.After(5 * time.Second):
		t.Fatalf("read message timeout")
		return systemplugin.Message{}
	}
}
