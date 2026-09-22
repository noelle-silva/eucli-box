package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"eucli-box/pkg/systemplugin"
	"eucli-box/pkg/systemplugin/pluginrun"
)

const (
	currentTimeInterfaceID = "current-time"
	defaultFormat          = "2006-01-02 15:04:05"
	defaultTimezone        = "Asia/Shanghai"
	localTimezone          = "Local"
)

var allowedFormats = map[string]struct{}{
	"2006-01-02 15:04:05":       {},
	"2006-01-02":                {},
	"15:04:05":                  {},
	"2006年01月02日 15:04:05":      {},
	"2006-01-02T15:04:05Z07:00": {},
}

var allowedTimezones = map[string]struct{}{
	localTimezone:      {},
	"Asia/Shanghai":    {},
	"UTC":              {},
	"Asia/Tokyo":       {},
	"Europe/London":    {},
	"America/New_York": {},
}

type pluginConfig struct {
	Format   string
	Timezone string
}

type timeProvider struct {
	mu        sync.Mutex
	config    pluginConfig
	configErr error
}

func main() {
	if err := pluginrun.Serve(pluginrun.Options{
		PluginID:     "time-plugin",
		Capabilities: []systemplugin.Capability{{Type: systemplugin.CapabilityPlaceholderValues, Interfaces: []string{currentTimeInterfaceID}}},
		Placeholder:  &timeProvider{},
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (p *timeProvider) Configure(config pluginrun.Config) {
	loaded, err := loadConfig(config.DefaultConfig, config.UserConfig)
	p.mu.Lock()
	p.config = loaded
	p.configErr = err
	p.mu.Unlock()
}

func (p *timeProvider) ResolvePlaceholders(_ context.Context, interfaceIDs []string) (map[string]string, error) {
	p.mu.Lock()
	config, configErr := p.config, p.configErr
	p.mu.Unlock()
	if configErr != nil {
		return nil, configErr
	}
	location, err := loadLocation(config.Timezone)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, interfaceID := range interfaceIDs {
		if interfaceID == currentTimeInterfaceID {
			values[interfaceID] = time.Now().In(location).Format(config.Format)
		}
	}
	return values, nil
}

func loadConfig(defaultConfig map[string]any, userConfig map[string]any) (pluginConfig, error) {
	config := pluginConfig{Format: defaultFormat, Timezone: defaultTimezone}
	applyConfigMap(&config, defaultConfig)
	applyConfigMap(&config, userConfig)
	config.Format = strings.TrimSpace(config.Format)
	config.Timezone = strings.TrimSpace(config.Timezone)
	if config.Format == "" {
		return pluginConfig{}, fmt.Errorf("时间格式不能为空")
	}
	if _, ok := allowedFormats[config.Format]; !ok {
		return pluginConfig{}, fmt.Errorf("时间格式不在可选范围内")
	}
	if config.Timezone == "" {
		config.Timezone = defaultTimezone
	}
	if _, ok := allowedTimezones[config.Timezone]; !ok {
		return pluginConfig{}, fmt.Errorf("时区不在可选范围内")
	}
	return config, nil
}

func applyConfigMap(config *pluginConfig, source map[string]any) {
	if config == nil || len(source) == 0 {
		return
	}
	if value, ok := source["format"].(string); ok {
		config.Format = value
	}
	if value, ok := source["timezone"].(string); ok {
		config.Timezone = value
	}
}

func loadLocation(name string) (*time.Location, error) {
	if name == localTimezone {
		return time.Local, nil
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("无法加载时区 %q", name)
	}
	return location, nil
}
