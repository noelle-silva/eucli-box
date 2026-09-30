package everything

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
)

// Config 是随包静态配置（config.json）的读取视图。
type Config struct {
	Limits LimitsConfig `json:"limits"`
}

// LimitsConfig 是随包静态上限：用户配置与调用参数只能在其范围内下调。
type LimitsConfig struct {
	DefaultRequestTimeoutMs int `json:"defaultRequestTimeoutMs"`
	MaxOutputChars          int `json:"maxOutputChars"`
}

// loadConfig 读取随包静态配置；工具安装包内始终携带 config.json，缺失即视为安装损坏。
func loadConfig(bodyDir string) (Config, error) {
	bodyDir = strings.TrimSpace(bodyDir)
	if bodyDir == "" {
		return Config{}, errors.New("tool body directory is required")
	}
	payload, err := os.ReadFile(filepath.Join(bodyDir, "config.json"))
	if err != nil {
		return Config{}, fmt.Errorf("read config.json: %w", err)
	}
	config := Config{}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode config.json: %w", err)
	}
	if config.Limits.DefaultRequestTimeoutMs <= 0 {
		return Config{}, errors.New("limits.defaultRequestTimeoutMs must be greater than zero")
	}
	if config.Limits.MaxOutputChars <= 0 {
		return Config{}, errors.New("limits.maxOutputChars must be greater than zero")
	}
	return config, nil
}

// userConfig 是工具用户配置的连接视图：访问地址、访问钥匙与输出上限。
type userConfig struct {
	Endpoint       string
	Key            string
	MaxOutputChars int
}

// loadUserConfig 从工具用户配置读取连接信息；地址非法或钥匙缺失都快速失败并指明修复位置。
func loadUserConfig(input types.ToolExecutionInput) (userConfig, error) {
	endpoint, err := normalizeAccessEndpoint(userConfigString(input, "endpoint"))
	if err != nil {
		return userConfig{}, err
	}
	key := strings.TrimSpace(userConfigString(input, "key"))
	if key == "" {
		return userConfig{}, errors.New("缺少访问钥匙（key）：请在工具设置页的用户配置里填写 Everything 应用开放接口的访问钥匙")
	}
	return userConfig{Endpoint: endpoint, Key: key}, nil
}

// userConfigString 读取用户配置中的字符串项；缺失或非字符串都返回空串。
func userConfigString(input types.ToolExecutionInput, key string) string {
	value, ok := input.UserConfig[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

// normalizeAccessEndpoint 归一访问地址：去尾部斜杠、必须是纯 http/https 地址。
func normalizeAccessEndpoint(value string) (string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(value), "/")
	if endpoint == "" {
		return "", errors.New("缺少访问地址（endpoint）：请在工具设置页的用户配置里填写 Everything 应用开放接口的访问地址，例如 http://127.0.0.1:43210")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("访问地址（endpoint）必须是绝对地址，例如 http://127.0.0.1:43210")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("访问地址（endpoint）只支持 http/https")
	}
	if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("访问地址（endpoint）必须是纯地址，不能携带路径、参数或片段")
	}
	return endpoint, nil
}

// effectiveMaxOutputChars 计算本次输出的字符上限：
// 随包上限为底，用户配置可下调；调用参数可在当前上限内再下调。两者越界都快速失败。
func effectiveMaxOutputChars(input types.ToolExecutionInput, config Config) (int, error) {
	limit := config.Limits.MaxOutputChars
	if value, ok := input.UserConfig["maxOutputChars"]; ok && value != nil {
		parsed, err := intValue(value, "maxOutputChars")
		if err != nil {
			return 0, errors.New("userConfig.maxOutputChars must be an integer")
		}
		if parsed < 1 || parsed > limit {
			return 0, fmt.Errorf("userConfig.maxOutputChars must be between 1 and %d", limit)
		}
		limit = parsed
	}
	if value, ok := input.Arguments["maxOutputChars"]; ok && value != nil {
		parsed, err := intValue(value, "maxOutputChars")
		if err != nil {
			return 0, err
		}
		if parsed < 1 || parsed > limit {
			return 0, fmt.Errorf("argument \"maxOutputChars\" must be between 1 and %d", limit)
		}
		limit = parsed
	}
	return limit, nil
}
