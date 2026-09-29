package hypercortexreader

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

// LimitsConfig 是随包静态上限：用户配置只能在其范围内下调。
type LimitsConfig struct {
	MaxOutputChars int `json:"maxOutputChars"`
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
	if config.Limits.MaxOutputChars <= 0 {
		return Config{}, errors.New("limits.maxOutputChars must be greater than zero")
	}
	return config, nil
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

// repoEntry 是工具配置区注册的一个仓库条目：仓库 id（可含中文）、描述与对应访问钥匙。
type repoEntry struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Key         string `json:"key"`
}

// repoConfig 是仓库配置：访问地址、默认条目与全部注册条目。
type repoConfig struct {
	Endpoint    string      `json:"endpoint"`
	DefaultRepo string      `json:"defaultRepo"`
	Repos       []repoEntry `json:"repos"`
}

// loadRepoConfig 从工具用户配置读取仓库配置；缺失或非法都快速失败并指明修复位置。
func loadRepoConfig(input types.ToolExecutionInput) (repoConfig, error) {
	raw := map[string]any{}
	for _, key := range []string{"endpoint", "defaultRepo", "repos"} {
		if value, ok := input.UserConfig[key]; ok && value != nil {
			raw[key] = value
		}
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return repoConfig{}, fmt.Errorf("读取仓库配置失败：%w", err)
	}
	config := repoConfig{}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return repoConfig{}, fmt.Errorf("仓库配置格式无效：%w", err)
	}
	return normalizeRepoConfig(config)
}

// normalizeRepoConfig 校验并归一仓库配置：地址合法、条目齐全不重复、默认条目存在。
func normalizeRepoConfig(config repoConfig) (repoConfig, error) {
	endpoint, err := normalizeAccessEndpoint(config.Endpoint)
	if err != nil {
		return repoConfig{}, err
	}
	config.Endpoint = endpoint
	if len(config.Repos) == 0 {
		return repoConfig{}, repoConfigError("必须至少注册一条仓库条目（repos）")
	}
	seen := map[string]bool{}
	normalized := make([]repoEntry, 0, len(config.Repos))
	for index := range config.Repos {
		entry := config.Repos[index]
		entry.ID = strings.TrimSpace(entry.ID)
		entry.Description = strings.TrimSpace(entry.Description)
		entry.Key = strings.TrimSpace(entry.Key)
		if entry.ID == "" {
			return repoConfig{}, repoConfigError(fmt.Sprintf("第 %d 条仓库条目缺少 id", index+1))
		}
		if seen[entry.ID] {
			return repoConfig{}, repoConfigError("存在重复的仓库 id：" + entry.ID)
		}
		if entry.Key == "" {
			return repoConfig{}, repoConfigError("仓库 " + entry.ID + " 缺少访问钥匙（key）")
		}
		seen[entry.ID] = true
		normalized = append(normalized, entry)
	}
	config.Repos = normalized
	config.DefaultRepo = strings.TrimSpace(config.DefaultRepo)
	if config.DefaultRepo == "" {
		return repoConfig{}, repoConfigError("必须指定默认仓库（defaultRepo）：动作未指定 repo 时使用它")
	}
	if !seen[config.DefaultRepo] {
		return repoConfig{}, repoConfigError("默认仓库（defaultRepo）" + config.DefaultRepo + " 未在仓库条目中注册")
	}
	return config, nil
}

// normalizeAccessEndpoint 归一 HyperCortex 访问地址：去尾部斜杠、必须是纯 http/https 地址。
func normalizeAccessEndpoint(value string) (string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(value), "/")
	if endpoint == "" {
		return "", repoConfigError("缺少访问地址（endpoint）：请在工具设置页的用户配置里填写 HyperCortex 显示的访问地址，例如 http://127.0.0.1:43210")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", repoConfigError("访问地址（endpoint）必须是绝对地址，例如 http://127.0.0.1:43210")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", repoConfigError("访问地址（endpoint）只支持 http/https")
	}
	if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", repoConfigError("访问地址（endpoint）必须是纯地址，不能携带路径、参数或片段")
	}
	return endpoint, nil
}

// resolve 解析动作选择的仓库：显式指定时按条目 id 精确匹配，缺省时使用默认条目。
func (config repoConfig) resolve(selector string) (repoEntry, error) {
	id := strings.TrimSpace(selector)
	if id == "" {
		id = config.DefaultRepo
	}
	for _, entry := range config.Repos {
		if entry.ID == id {
			return entry, nil
		}
	}
	available := make([]string, 0, len(config.Repos))
	for _, entry := range config.Repos {
		available = append(available, entry.ID)
	}
	return repoEntry{}, fmt.Errorf("未注册的仓库：%s（已注册：%s）", id, strings.Join(available, "、"))
}

func repoConfigError(message string) error {
	return fmt.Errorf("仓库配置无效：%s", message)
}
