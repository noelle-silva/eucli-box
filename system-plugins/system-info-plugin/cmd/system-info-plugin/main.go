package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strings"
	"time"
)

const (
	actionResolvePlaceholders = "resolve_placeholders"
	systemInfoInterfaceID     = "current-system-info"
)

type request struct {
	Action        string         `json:"action"`
	UserConfig    map[string]any `json:"userConfig"`
	DefaultConfig map[string]any `json:"defaultConfig"`
}

type response struct {
	Status string            `json:"status"`
	Values map[string]string `json:"values,omitempty"`
	Error  string            `json:"error,omitempty"`
}

type pluginConfig struct {
	IncludeHostname           bool
	IncludeUsername           bool
	IncludeEnvironmentSummary bool
}

func main() {
	var req request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		write(response{Status: "failed", Error: "输入不是有效 JSON"})
		return
	}
	if req.Action != actionResolvePlaceholders {
		write(response{Status: "failed", Error: "不支持的插件动作"})
		return
	}
	config := loadConfig(req.DefaultConfig, req.UserConfig)
	write(response{Status: "success", Values: map[string]string{systemInfoInterfaceID: formatSystemInfo(config)}})
}

func loadConfig(defaultConfig map[string]any, userConfig map[string]any) pluginConfig {
	config := pluginConfig{IncludeHostname: true, IncludeUsername: false, IncludeEnvironmentSummary: true}
	applyConfigMap(&config, defaultConfig)
	applyConfigMap(&config, userConfig)
	return config
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

func write(resp response) {
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}
