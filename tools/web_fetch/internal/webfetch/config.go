package webfetch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config 是工具的运行时配置：抓取的传输上限与输出上限。
type Config struct {
	// MaxResponseBytes 是响应体的最大字节数；超过即拒绝或截断。
	MaxResponseBytes int `json:"maxResponseBytes"`
	// MaxBodyChars 是解码后正文的最大字符数；超过即截断。
	MaxBodyChars int `json:"maxBodyChars"`
	// DefaultTimeoutMs 是未指定时限时使用的请求时限，单位毫秒。
	DefaultTimeoutMs int `json:"defaultTimeoutMs"`
	// MaxRedirects 是最多跟随的同源跳转次数；0 表示不跟随。
	MaxRedirects int `json:"maxRedirects"`
	// MaxOutputChars 是最终 Markdown 输出的最大字符数。
	MaxOutputChars int `json:"maxOutputChars"`
	// UserAgent 是每次请求发送的 User-Agent，显式声明产品身份。
	UserAgent string `json:"userAgent"`
}

func loadConfig(toolDirectory string) (Config, error) {
	if strings.TrimSpace(toolDirectory) == "" {
		return Config{}, fmt.Errorf("toolDirectory is required")
	}
	payload, err := os.ReadFile(filepath.Join(toolDirectory, "config.json"))
	if err != nil {
		return Config{}, fmt.Errorf("read config.json: %w", err)
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode config.json: %w", err)
	}
	return normalizeConfig(config)
}

func normalizeConfig(config Config) (Config, error) {
	if config.MaxResponseBytes <= 0 {
		return Config{}, fmt.Errorf("maxResponseBytes must be greater than zero")
	}
	if config.MaxBodyChars <= 0 {
		return Config{}, fmt.Errorf("maxBodyChars must be greater than zero")
	}
	if config.DefaultTimeoutMs <= 0 {
		return Config{}, fmt.Errorf("defaultTimeoutMs must be greater than zero")
	}
	if config.MaxRedirects < 0 {
		return Config{}, fmt.Errorf("maxRedirects must not be negative")
	}
	if config.MaxOutputChars <= 0 {
		return Config{}, fmt.Errorf("maxOutputChars must be greater than zero")
	}
	config.UserAgent = strings.TrimSpace(config.UserAgent)
	if config.UserAgent == "" {
		return Config{}, fmt.Errorf("userAgent is required")
	}
	return config, nil
}
