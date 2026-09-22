package systemplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"eucli-box/pkg/systemplugin"
	"eucli-box/pkg/utils"
)

const (
	defaultPingInterval = 5 * time.Second
	minPingInterval     = 100 * time.Millisecond
	restartDelay        = time.Second
	requestIDPrefix     = "plugin-request"
)

// sessionConfig 是建立一条控制通道所需的全部事实。
type sessionConfig struct {
	pluginID      string
	hostVersion   string
	executable    string
	directory     string
	dataDirectory string
	userConfig    map[string]any
	defaultConfig map[string]any
	timeout       time.Duration
	stopTimeout   time.Duration
	capabilities  []systemplugin.Capability
	onEvent       func(pluginID string, event string, payload map[string]any)
}

// session 是一条宿主与插件之间的控制通道：握手、调用、取消、配置通知、
// 心跳与限时停机都在这里收口，生命周期策略由托管层决定。
type session struct {
	config  sessionConfig
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	encoder *json.Encoder
	decoder *json.Decoder

	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[string]chan systemplugin.Message

	pingSequence uint64
	pingPending  bool
	pingDeadline time.Time

	readErr  error
	waitErr  error
	stopping atomic.Bool
	done     chan struct{}
}

// startSession 启动插件进程并完成身份与能力握手。
func startSession(config sessionConfig) (*session, error) {
	if config.timeout <= 0 {
		return nil, pluginExecutionFailed("system plugin timeout must be positive", nil)
	}
	if config.stopTimeout <= 0 {
		return nil, pluginExecutionFailed("system plugin stop timeout must be positive", nil)
	}
	if config.dataDirectory == "" {
		return nil, pluginExecutionFailed("system plugin data directory is unavailable", nil)
	}
	if err := os.MkdirAll(config.dataDirectory, 0o755); err != nil {
		return nil, pluginExecutionFailed("failed to create system plugin data directory", err)
	}
	executable, directory := config.executable, config.directory
	cmd := exec.Command(executable)
	cmd.Dir = directory
	// 标准输出是控制通道专用；标准错误与日志由插件自行管理，宿主不捕获。
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, pluginExecutionFailed("failed to open system plugin control input", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, pluginExecutionFailed("failed to open system plugin control output", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, pluginExecutionFailed("failed to start system plugin", err)
	}
	instance := &session{
		config:  config,
		cmd:     cmd,
		stdin:   stdin,
		encoder: json.NewEncoder(stdin),
		decoder: json.NewDecoder(stdout),
		pending: map[string]chan systemplugin.Message{},
		done:    make(chan struct{}),
	}
	go instance.waitProcess()
	hello := systemplugin.NewMessage(systemplugin.MessageHello)
	hello.PluginID = config.pluginID
	hello.HostVersion = config.hostVersion
	hello.DataDirectory = config.dataDirectory
	hello.UserConfig = config.userConfig
	hello.DefaultConfig = config.defaultConfig
	if err := instance.write(hello); err != nil {
		instance.kill()
		return nil, pluginExecutionFailed("系统插件控制通道写入失败", err)
	}
	ready, err := instance.readHandshake(config.timeout)
	if err != nil {
		instance.kill()
		return nil, pluginExecutionFailed("系统插件未完成握手", err)
	}
	if err := systemplugin.ValidateReady(ready, config.pluginID, config.capabilities); err != nil {
		instance.kill()
		return nil, pluginExecutionFailed("系统插件握手校验失败", err)
	}
	go instance.readLoop()
	go instance.pingLoop()
	return instance, nil
}

func (p *session) waitProcess() {
	err := p.cmd.Wait()
	p.mu.Lock()
	p.waitErr = err
	p.mu.Unlock()
	close(p.done)
}

// readHandshake 在握手期限内读取插件应答；握手期间只允许一个读取者。
func (p *session) readHandshake(timeout time.Duration) (systemplugin.Message, error) {
	type readResult struct {
		message systemplugin.Message
		err     error
	}
	result := make(chan readResult, 1)
	go func() {
		message, err := systemplugin.Read(p.decoder)
		result <- readResult{message: message, err: err}
	}()
	select {
	case item := <-result:
		return item.message, item.err
	case <-time.After(timeout):
		return systemplugin.Message{}, fmt.Errorf("等待插件握手应答超时")
	case <-p.done:
		return systemplugin.Message{}, fmt.Errorf("插件进程提前退出")
	}
}

func (p *session) readLoop() {
	for {
		message, err := systemplugin.Read(p.decoder)
		if err != nil {
			p.fail(fmt.Errorf("控制通道读取失败：%w", err))
			return
		}
		p.dispatch(message)
	}
}

func (p *session) dispatch(message systemplugin.Message) {
	switch message.Type {
	case systemplugin.MessageResult:
		p.mu.Lock()
		waiter := p.pending[message.RequestID]
		p.mu.Unlock()
		if waiter != nil {
			select {
			case waiter <- message:
			default:
			}
		}
	case systemplugin.MessagePong:
		p.mu.Lock()
		if p.pingPending && message.Sequence == p.pingSequence {
			p.pingPending = false
		}
		p.mu.Unlock()
	case systemplugin.MessageEvent:
		if p.config.onEvent != nil {
			p.config.onEvent(p.config.pluginID, message.Event, message.Payload)
		}
	}
	// 未知消息类型按前向兼容忽略。
}

func (p *session) pingLoop() {
	interval := p.config.timeout / 2
	if interval > defaultPingInterval {
		interval = defaultPingInterval
	}
	if interval < minPingInterval {
		interval = minPingInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			if err := p.pingOnce(); err != nil {
				p.fail(err)
				return
			}
		}
	}
}

func (p *session) pingOnce() error {
	p.mu.Lock()
	if p.pingPending {
		expired := time.Now().After(p.pingDeadline)
		p.mu.Unlock()
		if expired {
			return fmt.Errorf("系统插件心跳超时")
		}
		return nil
	}
	p.pingSequence++
	sequence := p.pingSequence
	p.pingPending = true
	p.pingDeadline = time.Now().Add(p.config.timeout)
	p.mu.Unlock()
	return p.write(systemplugin.Message{Type: systemplugin.MessagePing, Sequence: sequence})
}

// invokeFailure 区分调用失败是否会损坏通道：致命失败需要丢弃整条会话。
type invokeFailure struct {
	message string
	cause   error
	fatal   bool
}

func (f *invokeFailure) Error() string {
	if f.cause != nil {
		return f.message + "：" + f.cause.Error()
	}
	return f.message
}

func (f *invokeFailure) Unwrap() error {
	return f.cause
}

func fatalInvokeFailure(err error) bool {
	var failure *invokeFailure
	if errors.As(err, &failure) {
		return failure.fatal
	}
	return false
}

// invoke 发起一次能力调用并等待结果；取消与超时都会向插件发出取消消息。
func (p *session) invoke(ctx context.Context, capability string, interfaces []string) (systemplugin.Message, error) {
	requestID := utils.NewID(requestIDPrefix)
	waiter := make(chan systemplugin.Message, 1)
	p.mu.Lock()
	if p.readErr != nil {
		readErr := p.readErr
		p.mu.Unlock()
		return systemplugin.Message{}, &invokeFailure{message: "控制通道已断开", cause: readErr, fatal: true}
	}
	p.pending[requestID] = waiter
	p.mu.Unlock()
	cleanup := func() {
		p.mu.Lock()
		delete(p.pending, requestID)
		p.mu.Unlock()
	}
	if err := p.write(systemplugin.Message{Type: systemplugin.MessageInvoke, RequestID: requestID, Capability: capability, Interfaces: interfaces}); err != nil {
		cleanup()
		return systemplugin.Message{}, &invokeFailure{message: "控制通道写入失败", cause: err, fatal: true}
	}
	select {
	case message := <-waiter:
		cleanup()
		return message, nil
	case <-ctx.Done():
		cleanup()
		_ = p.write(systemplugin.Message{Type: systemplugin.MessageCancel, RequestID: requestID})
		return systemplugin.Message{}, &invokeFailure{message: "系统插件请求已取消", cause: ctx.Err()}
	case <-time.After(p.config.timeout):
		cleanup()
		_ = p.write(systemplugin.Message{Type: systemplugin.MessageCancel, RequestID: requestID})
		return systemplugin.Message{}, &invokeFailure{message: "系统插件请求超时", fatal: true}
	case <-p.done:
		cleanup()
		return systemplugin.Message{}, &invokeFailure{message: "控制通道已断开", cause: p.failureError(), fatal: true}
	}
}

// notifyConfig 向常驻插件推送配置变更；插件自行决定如何刷新内部状态。
func (p *session) notifyConfig(userConfig map[string]any, defaultConfig map[string]any) error {
	return p.write(systemplugin.Message{Type: systemplugin.MessageConfig, UserConfig: userConfig, DefaultConfig: defaultConfig})
}

// stop 请求插件优雅退出；超过停机时限后强制终止，保证停机有界。
func (p *session) stop(ctx context.Context) error {
	p.stopping.Store(true)
	_ = p.write(systemplugin.NewMessage(systemplugin.MessageStop))
	limit := p.config.stopTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < limit {
			limit = remaining
		}
	}
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
	case <-timer.C:
	}
	p.kill()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *session) kill() {
	_ = p.stdin.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

func (p *session) write(message systemplugin.Message) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return systemplugin.Write(p.encoder, message)
}

func (p *session) fail(err error) {
	p.mu.Lock()
	if p.readErr == nil {
		p.readErr = err
	}
	p.mu.Unlock()
	p.kill()
}

// stoppingByHost 表示这条通道是否由宿主主动停机。
func (p *session) stoppingByHost() bool {
	return p.stopping.Load()
}

// failureError 返回通道失效的根因，供监视者记录失败事实。
func (p *session) failureError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.readErr != nil {
		return p.readErr
	}
	if p.waitErr != nil {
		return p.waitErr
	}
	return nil
}
