package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"eucli-box/pkg/systemplugin/pluginrun"
)

const (
	// refreshInterval 与旧缓存心跳节奏一致：每 30 分钟刷新一次天气数据。
	refreshInterval = 30 * time.Minute
	// fetchTimeout 是单轮天气抓取的内部上限。
	fetchTimeout = 45 * time.Second
	// resolveWaitTimeout 是单次取值等待首轮刷新的上限；插件内部有界，不拖垮宿主调用。
	resolveWaitTimeout = 5 * time.Second
)

// weatherProvider 把抓取与缓存收在插件内部：后台按节奏刷新，取值读缓存。
type weatherProvider struct {
	mu         sync.Mutex
	generation int
	config     pluginConfig
	configErr  error
	ready      bool
	values     map[string]string
	lastErr    error
	waiters    []chan struct{}

	trigger chan struct{}
	once    sync.Once
}

func newWeatherProvider() *weatherProvider {
	return &weatherProvider{trigger: make(chan struct{}, 1)}
}

// Configure 应用新配置：作废旧缓存与在途结果，并立刻拉起一轮刷新。
func (p *weatherProvider) Configure(config pluginrun.Config) {
	loaded, err := loadConfig(config.DefaultConfig, config.UserConfig)
	p.mu.Lock()
	p.generation++
	p.config = loaded
	p.configErr = err
	p.ready = false
	p.values = nil
	p.lastErr = nil
	p.wakeLocked()
	p.mu.Unlock()
	p.kick()
}

func (p *weatherProvider) ResolvePlaceholders(ctx context.Context, interfaceIDs []string) (map[string]string, error) {
	p.startRefresher()
	if err := p.waitReady(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	values := make(map[string]string, len(interfaceIDs))
	for _, interfaceID := range interfaceIDs {
		if value, ok := p.values[interfaceID]; ok {
			values[interfaceID] = value
		}
	}
	p.mu.Unlock()
	return values, nil
}

// waitReady 等待当前配置的一轮刷新落地；配置错误或刷新失败立即如实返回。
func (p *weatherProvider) waitReady(ctx context.Context) error {
	waitCtx, cancel := context.WithTimeout(ctx, resolveWaitTimeout)
	defer cancel()
	for {
		p.mu.Lock()
		if p.configErr != nil {
			err := p.configErr
			p.mu.Unlock()
			return err
		}
		if p.ready {
			p.mu.Unlock()
			return nil
		}
		if p.lastErr != nil {
			err := p.lastErr
			p.mu.Unlock()
			return err
		}
		waiter := make(chan struct{})
		p.waiters = append(p.waiters, waiter)
		p.mu.Unlock()
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("天气数据尚未就绪，请稍后重试")
		case <-waiter:
		}
	}
}

func (p *weatherProvider) startRefresher() {
	p.once.Do(func() {
		go func() {
			ticker := time.NewTicker(refreshInterval)
			defer ticker.Stop()
			for {
				select {
				case <-p.trigger:
				case <-ticker.C:
				}
				p.refresh()
			}
		}()
	})
}

func (p *weatherProvider) kick() {
	p.startRefresher()
	select {
	case p.trigger <- struct{}{}:
	default:
	}
}

// refresh 抓取一轮天气；配置已换代时丢弃过期结果，避免旧配置覆盖新缓存。
func (p *weatherProvider) refresh() {
	p.mu.Lock()
	generation := p.generation
	config, configErr := p.config, p.configErr
	p.mu.Unlock()
	if configErr != nil {
		p.finish(generation, nil, configErr)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	bundle, err := newWeatherClient().fetchWeather(ctx, config)
	if err != nil {
		p.finish(generation, nil, err)
		return
	}
	p.finish(generation, map[string]string{
		weatherDetailInterfaceID: formatDetail(bundle, config),
		weatherBriefInterfaceID:  formatBrief(bundle, config),
	}, nil)
}

func (p *weatherProvider) finish(generation int, values map[string]string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if generation != p.generation {
		return
	}
	if err != nil {
		p.lastErr = err
		p.ready = false
	} else {
		p.values = values
		p.lastErr = nil
		p.ready = true
	}
	p.wakeLocked()
}

func (p *weatherProvider) wakeLocked() {
	for _, waiter := range p.waiters {
		close(waiter)
	}
	p.waiters = nil
}
