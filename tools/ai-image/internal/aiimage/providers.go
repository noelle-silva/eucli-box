package aiimage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

const providersFileName = "providers.json"

// maskedAPIKey 是 apiKey 在配置读取输出中的统一打码占位。
const maskedAPIKey = "********"

// 内置协议标识。
const (
	protocolImages      = "images"
	protocolImagesEdits = "images-edits"
	protocolChat        = "chat"
	protocolAdapter     = "adapter"
)

type providerConfig struct {
	DefaultProvider string          `json:"defaultProvider"`
	Providers       []providerEntry `json:"providers"`
}

type providerEntry struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Enabled          *bool    `json:"enabled"`
	BaseURL          string   `json:"baseUrl"`
	APIKey           string   `json:"apiKey"`
	Protocol         string   `json:"protocol"`
	Adapter          string   `json:"adapter"`
	Models           []string `json:"models"`
	DefaultModel     string   `json:"defaultModel"`
	ChatSystemPrompt string   `json:"chatSystemPrompt"`
}

func (p providerEntry) displayName() string {
	if strings.TrimSpace(p.Name) != "" {
		return strings.TrimSpace(p.Name)
	}
	return p.ID
}

func (p providerEntry) isEnabled() bool {
	return p.Enabled == nil || *p.Enabled
}

// parseProviderConfig 解析并校验 providers.json 内容。
func parseProviderConfig(content string) (providerConfig, error) {
	var config providerConfig
	if err := decodeStrictJSON(content, &config); err != nil {
		return providerConfig{}, fmt.Errorf("providers.json 不是合法配置: %w", err)
	}
	config = normalizeProviderConfig(config)
	if err := validateProviderConfig(config); err != nil {
		return providerConfig{}, err
	}
	return config, nil
}

// loadProviderConfig 从配置区读取并校验运营商配置。
func loadProviderConfig(root string) (providerConfig, error) {
	content, _, err := readConfigFile(root, providersFileName)
	if err != nil {
		return providerConfig{}, err
	}
	return parseProviderConfig(content)
}

// resolveProvider 选定运营商：参数优先，其次 defaultProvider；不可用时明确失败。
func resolveProvider(config providerConfig, requested string) (providerEntry, error) {
	providerID := strings.TrimSpace(requested)
	if providerID == "" {
		providerID = strings.TrimSpace(config.DefaultProvider)
	}
	if providerID == "" {
		return providerEntry{}, errors.New("未指定运营商，且配置中没有 defaultProvider")
	}
	for _, provider := range config.Providers {
		if provider.ID != providerID {
			continue
		}
		if !provider.isEnabled() {
			return providerEntry{}, fmt.Errorf("运营商已停用: %s", providerID)
		}
		return provider, nil
	}
	return providerEntry{}, fmt.Errorf("运营商不存在: %s（可用: %s）", providerID, providerIDs(config))
}

// resolveModel 选定模型：参数优先，其次运营商的 defaultModel；
// 运营商声明了 models 列表时，选定模型必须在列表中。
func resolveModel(provider providerEntry, requested string) (string, error) {
	model := strings.TrimSpace(requested)
	if model == "" {
		model = provider.DefaultModel
	}
	if model == "" {
		return "", fmt.Errorf("运营商 %s 未指定默认模型，且调用未提供 model", provider.ID)
	}
	if len(provider.Models) > 0 && !containsString(provider.Models, model) {
		return "", fmt.Errorf("模型 %q 不在运营商 %s 的 models 列表中（可用: %s）", model, provider.ID, strings.Join(provider.Models, ", "))
	}
	return model, nil
}

func normalizeProviderConfig(config providerConfig) providerConfig {
	config.DefaultProvider = strings.TrimSpace(config.DefaultProvider)
	providers := make([]providerEntry, 0, len(config.Providers))
	for _, provider := range config.Providers {
		provider.ID = strings.TrimSpace(provider.ID)
		provider.Name = strings.TrimSpace(provider.Name)
		provider.BaseURL = strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/")
		provider.APIKey = strings.TrimSpace(provider.APIKey)
		provider.Protocol = strings.TrimSpace(provider.Protocol)
		provider.Adapter = strings.TrimSpace(provider.Adapter)
		provider.DefaultModel = strings.TrimSpace(provider.DefaultModel)
		provider.ChatSystemPrompt = strings.TrimSpace(provider.ChatSystemPrompt)
		models := make([]string, 0, len(provider.Models))
		for _, model := range provider.Models {
			if model = strings.TrimSpace(model); model != "" {
				models = append(models, model)
			}
		}
		provider.Models = models
		providers = append(providers, provider)
	}
	config.Providers = providers
	return config
}

func validateProviderConfig(config providerConfig) error {
	seen := map[string]struct{}{}
	for index, provider := range config.Providers {
		if err := validateProviderEntry(provider); err != nil {
			return fmt.Errorf("providers[%d]（%s）: %w", index, provider.ID, err)
		}
		if _, ok := seen[provider.ID]; ok {
			return fmt.Errorf("providers[%d]: 运营商 id 重复: %s", index, provider.ID)
		}
		seen[provider.ID] = struct{}{}
	}
	if config.DefaultProvider != "" {
		if _, ok := seen[config.DefaultProvider]; !ok {
			return fmt.Errorf("defaultProvider 指向不存在的运营商: %s", config.DefaultProvider)
		}
	}
	return nil
}

func validateProviderEntry(provider providerEntry) error {
	if !validConfigID(provider.ID) {
		return fmt.Errorf("运营商 id 无效: %q", provider.ID)
	}
	if provider.BaseURL == "" {
		return errors.New("缺少 baseUrl")
	}
	if err := validateHTTPBaseURL(provider.BaseURL); err != nil {
		return err
	}
	switch provider.Protocol {
	case protocolImages, protocolImagesEdits, protocolChat:
		if provider.Adapter != "" {
			return fmt.Errorf("内置协议 %s 不需要 adapter 字段", provider.Protocol)
		}
	case protocolAdapter:
		if !validConfigID(provider.Adapter) {
			return errors.New("protocol=adapter 时必须填写 adapter（适配文件名）")
		}
	default:
		return fmt.Errorf("protocol 无效: %q（应为 images、images-edits、chat 或 adapter）", provider.Protocol)
	}
	if provider.DefaultModel != "" && len(provider.Models) > 0 {
		found := false
		for _, model := range provider.Models {
			if model == provider.DefaultModel {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("defaultModel %q 不在 models 列表中", provider.DefaultModel)
		}
	}
	return nil
}

func validConfigID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		case char == '-', char == '_', char == '.':
		default:
			return false
		}
	}
	return true
}

func validateHTTPBaseURL(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return errors.New("baseUrl 必须是 http(s) 绝对地址")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("baseUrl 只支持 http/https")
	}
	return nil
}

func providerIDs(config providerConfig) string {
	ids := make([]string, 0, len(config.Providers))
	for _, provider := range config.Providers {
		ids = append(ids, provider.ID)
	}
	if len(ids) == 0 {
		return "（无）"
	}
	return strings.Join(ids, ", ")
}

// maskProviderContent 把 providers.json 中的 apiKey 打码后再输出给模型。
func maskProviderContent(content string) (string, bool, error) {
	var document any
	if err := json.Unmarshal([]byte(content), &document); err != nil {
		return "", false, err
	}
	if !maskAPIKeys(document) {
		return content, false, nil
	}
	encoded, err := marshalCanonical(document)
	if err != nil {
		return "", false, err
	}
	return string(encoded), true, nil
}

func maskAPIKeys(value any) bool {
	masked := false
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if strings.EqualFold(strings.TrimSpace(key), "apiKey") {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					typed[key] = maskedAPIKey
					masked = true
					continue
				}
			}
			if maskAPIKeys(item) {
				masked = true
			}
		}
	case []any:
		for _, item := range typed {
			if maskAPIKeys(item) {
				masked = true
			}
		}
	}
	return masked
}

// restoreProviderContent 是 providers.json 落盘前的统一凭据还原入口，
// 与 maskProviderContent 在文档层对称：新内容不含打码值时原样返回
// （保持整份写入的原文形态）；含打码值时按运营商 id 从磁盘原配置还原
// 真实 Key，没有可还原的原配置时明确失败。
func restoreProviderContent(root string, nextContent string) (string, error) {
	var document any
	if err := json.Unmarshal([]byte(nextContent), &document); err != nil {
		return "", fmt.Errorf("providers.json 不是合法 JSON: %w", err)
	}
	if !containsMaskedAPIKeyValue(document) {
		return nextContent, nil
	}
	previousContent, _, err := readConfigFile(root, providersFileName)
	if err != nil {
		return "", fmt.Errorf("原 providers.json 不可读，无法还原打码 apiKey，请提供明文 apiKey: %w", err)
	}
	var previous providerConfig
	if err := decodeStrictJSON(previousContent, &previous); err != nil {
		return "", fmt.Errorf("原 providers.json 不是合法配置，无法还原打码 apiKey，请提供明文 apiKey: %w", err)
	}
	previousKeys := map[string]string{}
	for _, provider := range previous.Providers {
		previousKeys[provider.ID] = provider.APIKey
	}
	if err := restoreAPIKeyValues(document, previousKeys); err != nil {
		return "", err
	}
	encoded, err := marshalCanonical(document)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// containsMaskedAPIKeyValue 判定 JSON 文档中是否存在打码占位的 apiKey 值。
func containsMaskedAPIKeyValue(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if strings.EqualFold(strings.TrimSpace(key), "apiKey") {
				if text, ok := item.(string); ok && text == maskedAPIKey {
					return true
				}
			}
			if containsMaskedAPIKeyValue(item) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if containsMaskedAPIKeyValue(item) {
				return true
			}
		}
	}
	return false
}

// restoreAPIKeyValues 在 JSON 文档中把打码 apiKey 原位替换为同一运营商的
// 原真实 Key。打码占位本身永远不是合法凭据：运营商缺失、原 Key 为空、
// 或原值本身就是打码占位（历史损坏状态）时都明确拒绝，要求提供明文。
func restoreAPIKeyValues(value any, previousKeys map[string]string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if strings.EqualFold(strings.TrimSpace(key), "apiKey") {
				if text, ok := item.(string); ok && text == maskedAPIKey {
					providerID := strings.TrimSpace(stringValue(typed["id"]))
					original := previousKeys[providerID]
					if original == "" || original == maskedAPIKey {
						return fmt.Errorf("运营商 %s 的 apiKey 使用了打码占位，但原配置中没有可保留的真实 Key，请提供明文 apiKey", providerID)
					}
					typed[key] = original
					continue
				}
			}
			if err := restoreAPIKeyValues(item, previousKeys); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range typed {
			if err := restoreAPIKeyValues(item, previousKeys); err != nil {
				return err
			}
		}
	}
	return nil
}

// decodeStrictJSON 严格解析 JSON：拒绝未知字段与多余内容。
func decodeStrictJSON(content string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("JSON 后存在多余内容")
	}
	return nil
}

// marshalCanonical 以稳定格式编码 JSON：两空格缩进、不转义 HTML。
func marshalCanonical(value any) ([]byte, error) {
	var buffer strings.Builder
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return []byte(buffer.String()), nil
}
