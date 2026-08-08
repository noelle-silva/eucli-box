package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	_ "time/tzdata"
)

const (
	actionResolvePlaceholders = "resolve_placeholders"
	currentTimeInterfaceID    = "current-time"
	defaultFormat             = "2006-01-02 15:04:05"
	defaultTimezone           = "Asia/Shanghai"
	localTimezone             = "Local"
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
	Format   string
	Timezone string
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
	config, err := loadConfig(req.DefaultConfig, req.UserConfig)
	if err != nil {
		write(response{Status: "failed", Error: err.Error()})
		return
	}
	location, err := loadLocation(config.Timezone)
	if err != nil {
		write(response{Status: "failed", Error: err.Error()})
		return
	}
	write(response{Status: "success", Values: map[string]string{currentTimeInterfaceID: time.Now().In(location).Format(config.Format)}})
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

func write(resp response) {
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}
