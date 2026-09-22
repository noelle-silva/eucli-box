// Package pluginrun 是系统插件的宿主侧运行契约实现：握手、能力调用、取消、
// 配置变更、心跳应答与优雅停机。插件程序只需要提供各能力的实现并调用 Serve。
package pluginrun

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"eucli-box/pkg/systemplugin"
)

// Config 是宿主下发的插件运行事实；握手与每次配置变更都会整体替换。
type Config struct {
	PluginID      string
	DataDirectory string
	UserConfig    map[string]any
	DefaultConfig map[string]any
}

// PlaceholderProvider 是「占位符取值」能力的插件侧实现。
// Configure 与 ResolvePlaceholders 可能被并发调用，实现须自身保证并发安全。
type PlaceholderProvider interface {
	Configure(config Config)
	ResolvePlaceholders(ctx context.Context, interfaceIDs []string) (map[string]string, error)
}

// Sender 是插件向宿主上报事件的能力；事件不落盘，由宿主决定去向。
type Sender func(name string, payload map[string]any) error

// Options 是插件对宿主声明的身份与能力。
type Options struct {
	PluginID     string
	Capabilities []systemplugin.Capability
	Placeholder  PlaceholderProvider
	// Observe 在握手完成后调用，向插件暴露事件发送等通道能力；可为 nil。
	Observe func(Sender)
}

type runner struct {
	options Options
	encoder *json.Encoder

	writeMu sync.Mutex
	mu      sync.Mutex
	config  Config
	cancels map[string]context.CancelFunc
}

// Serve 在标准输入输出上服务控制通道，直到收到停机请求或输入关闭。
func Serve(options Options) error {
	options.PluginID = strings.TrimSpace(options.PluginID)
	if options.PluginID == "" {
		return fmt.Errorf("插件身份不能为空")
	}
	decoder := json.NewDecoder(os.Stdin)
	decoder.UseNumber()
	instance := &runner{
		options: options,
		encoder: json.NewEncoder(os.Stdout),
		cancels: map[string]context.CancelFunc{},
	}
	hello, err := systemplugin.Read(decoder)
	if err != nil {
		return fmt.Errorf("读取宿主握手失败：%w", err)
	}
	if err := systemplugin.ValidateHello(hello, options.PluginID); err != nil {
		return fmt.Errorf("宿主握手无效：%w", err)
	}
	instance.applyConfig(hello)
	instance.notifyConfigure()
	if err := instance.write(systemplugin.Message{
		Type:         systemplugin.MessageReady,
		PluginID:     options.PluginID,
		Capabilities: options.Capabilities,
	}); err != nil {
		return fmt.Errorf("应答宿主握手失败：%w", err)
	}
	if options.Observe != nil {
		options.Observe(instance.sendEvent)
	}
	for {
		message, err := systemplugin.Read(decoder)
		if err != nil {
			return nil
		}
		switch message.Type {
		case systemplugin.MessageInvoke:
			ctx, cancel := context.WithCancel(context.Background())
			instance.register(message.RequestID, cancel)
			go instance.handleInvoke(ctx, cancel, message)
		case systemplugin.MessageCancel:
			instance.cancel(message.RequestID)
		case systemplugin.MessageConfig:
			instance.applyConfig(message)
			instance.notifyConfigure()
		case systemplugin.MessagePing:
			if err := instance.write(systemplugin.Message{Type: systemplugin.MessagePong, Sequence: message.Sequence}); err != nil {
				return nil
			}
		case systemplugin.MessageStop:
			instance.cancelAll()
			return nil
		default:
			// 未知消息类型按前向兼容忽略；协议违规由握手与调用校验兜底。
		}
	}
}

func (r *runner) handleInvoke(ctx context.Context, cancel context.CancelFunc, message systemplugin.Message) {
	defer func() {
		r.unregister(message.RequestID)
		cancel()
	}()
	if message.Capability != systemplugin.CapabilityPlaceholderValues || r.options.Placeholder == nil {
		_ = r.write(systemplugin.Message{
			Type:      systemplugin.MessageResult,
			RequestID: message.RequestID,
			Status:    systemplugin.StatusFailed,
			Error:     "不支持的插件能力：" + message.Capability,
		})
		return
	}
	values, err := r.options.Placeholder.ResolvePlaceholders(ctx, message.Interfaces)
	if err != nil {
		_ = r.write(systemplugin.Message{
			Type:      systemplugin.MessageResult,
			RequestID: message.RequestID,
			Status:    systemplugin.StatusFailed,
			Error:     err.Error(),
		})
		return
	}
	_ = r.write(systemplugin.Message{
		Type:      systemplugin.MessageResult,
		RequestID: message.RequestID,
		Status:    systemplugin.StatusSuccess,
		Values:    values,
	})
}

func (r *runner) applyConfig(message systemplugin.Message) {
	r.mu.Lock()
	r.config = Config{
		PluginID:      r.options.PluginID,
		DataDirectory: strings.TrimSpace(message.DataDirectory),
		UserConfig:    message.UserConfig,
		DefaultConfig: message.DefaultConfig,
	}
	r.mu.Unlock()
}

func (r *runner) notifyConfigure() {
	if r.options.Placeholder == nil {
		return
	}
	r.mu.Lock()
	config := r.config
	r.mu.Unlock()
	r.options.Placeholder.Configure(config)
}

func (r *runner) register(requestID string, cancel context.CancelFunc) {
	if strings.TrimSpace(requestID) == "" {
		return
	}
	r.mu.Lock()
	r.cancels[requestID] = cancel
	r.mu.Unlock()
}

func (r *runner) unregister(requestID string) {
	r.mu.Lock()
	delete(r.cancels, requestID)
	r.mu.Unlock()
}

func (r *runner) cancel(requestID string) {
	r.mu.Lock()
	cancel := r.cancels[requestID]
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *runner) cancelAll() {
	r.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(r.cancels))
	for _, cancel := range r.cancels {
		cancels = append(cancels, cancel)
	}
	r.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (r *runner) write(message systemplugin.Message) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return systemplugin.Write(r.encoder, message)
}

// sendEvent 向宿主上报一条事件；事件名不能为空。
func (r *runner) sendEvent(name string, payload map[string]any) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("事件名称不能为空")
	}
	return r.write(systemplugin.Message{Type: systemplugin.MessageEvent, Event: name, Payload: payload})
}
