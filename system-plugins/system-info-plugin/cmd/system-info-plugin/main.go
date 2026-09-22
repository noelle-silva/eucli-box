package main

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/systemplugin"
	"eucli-box/pkg/systemplugin/pluginrun"
)

const systemInfoInterfaceID = "current-system-info"

type pluginConfig struct {
	IncludeHostname           bool
	IncludeUsername           bool
	IncludeEnvironmentSummary bool
}

type systemInfoProvider struct {
	mu     sync.Mutex
	config pluginConfig
}

func main() {
	if err := pluginrun.Serve(pluginrun.Options{
		PluginID:     "system-info-plugin",
		Capabilities: []systemplugin.Capability{{Type: systemplugin.CapabilityPlaceholderValues, Interfaces: []string{systemInfoInterfaceID}}},
		Placeholder:  &systemInfoProvider{},
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (p *systemInfoProvider) Configure(config pluginrun.Config) {
	loaded := pluginConfig{IncludeHostname: true, IncludeUsername: false, IncludeEnvironmentSummary: true}
	applyConfigMap(&loaded, config.DefaultConfig)
	applyConfigMap(&loaded, config.UserConfig)
	p.mu.Lock()
	p.config = loaded
	p.mu.Unlock()
}

func (p *systemInfoProvider) ResolvePlaceholders(_ context.Context, interfaceIDs []string) (map[string]string, error) {
	p.mu.Lock()
	config := p.config
	p.mu.Unlock()
	values := map[string]string{}
	for _, interfaceID := range interfaceIDs {
		if interfaceID == systemInfoInterfaceID {
			values[interfaceID] = formatSystemInfo(config)
		}
	}
	return values, nil
}

func applyConfigMap(config *pluginConfig, source map[string]any) {
	if config == nil || len(source) == 0 {
		return
	}
	if value, ok := source["includeHostname"].(bool); ok {
		config.IncludeHostname = value
	}
	if value, ok := source["includeUsername"].(bool); ok {
		config.IncludeUsername = value
	}
	if value, ok := source["includeEnvironmentSummary"].(bool); ok {
		config.IncludeEnvironmentSummary = value
	}
}

func formatSystemInfo(config pluginConfig) string {
	lines := []string{
		"【当前平台/系统信息】",
		"操作系统：" + runtime.GOOS,
		"处理器架构：" + runtime.GOARCH,
		fmt.Sprintf("CPU 数量：%d", runtime.NumCPU()),
		"采集时间：" + time.Now().Format("2006-01-02 15:04:05"),
	}
	if config.IncludeHostname {
		if hostname, err := os.Hostname(); err == nil && strings.TrimSpace(hostname) != "" {
			lines = append(lines, "主机名："+hostname)
		}
	}
	if config.IncludeUsername {
		if currentUser, err := user.Current(); err == nil && currentUser != nil && strings.TrimSpace(currentUser.Username) != "" {
			lines = append(lines, "当前用户："+currentUser.Username)
		}
	}
	if config.IncludeEnvironmentSummary {
		lines = append(lines, environmentSummary()...)
	}
	return strings.Join(lines, "\n")
}

func environmentSummary() []string {
	items := []string{}
	if value := strings.TrimSpace(os.Getenv("OS")); value != "" {
		items = append(items, "系统标识："+value)
	}
	if value := strings.TrimSpace(os.Getenv("PROCESSOR_IDENTIFIER")); value != "" {
		items = append(items, "处理器标识："+value)
	}
	if value := strings.TrimSpace(os.Getenv("PROCESSOR_LEVEL")); value != "" {
		items = append(items, "处理器等级："+value)
	}
	return items
}
