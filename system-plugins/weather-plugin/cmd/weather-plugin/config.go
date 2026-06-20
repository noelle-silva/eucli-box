package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type pluginConfig struct {
	City                   string
	APIKey                 string
	APIHost                string
	ForecastDays           int
	HourlyForecastInterval int
	HourlyForecastCount    int
	IncludeWarnings        bool
	IncludeAirQuality      bool
}

func loadConfig(defaultConfig map[string]any, userConfig map[string]any) (pluginConfig, error) {
	config := pluginConfig{APIHost: "devapi.qweather.com", ForecastDays: 3, HourlyForecastInterval: 2, HourlyForecastCount: 12, IncludeWarnings: true, IncludeAirQuality: false}
	applyConfigMap(&config, defaultConfig)
	applyConfigMap(&config, userConfig)
	config.City = strings.TrimSpace(config.City)
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.APIHost = cleanAPIHost(config.APIHost)
	if config.City == "" {
		return pluginConfig{}, fmt.Errorf("城市不能为空")
	}
	if config.APIKey == "" {
		return pluginConfig{}, fmt.Errorf("和风天气密钥不能为空")
	}
	if config.APIHost == "" {
		return pluginConfig{}, fmt.Errorf("和风天气请求地址不能为空")
	}
	if !validForecastDays(config.ForecastDays) {
		return pluginConfig{}, fmt.Errorf("多日预报天数只支持 3、7、10、15")
	}
	if config.HourlyForecastInterval < 1 {
		return pluginConfig{}, fmt.Errorf("小时预报展示间隔必须大于 0")
	}
	if config.HourlyForecastCount < 1 {
		return pluginConfig{}, fmt.Errorf("小时预报展示条数必须大于 0")
	}
	return config, nil
}

func applyConfigMap(config *pluginConfig, source map[string]any) {
	if config == nil || len(source) == 0 {
		return
	}
	if value, ok := source["city"].(string); ok {
		config.City = value
	}
	if value, ok := source["apiKey"].(string); ok {
		config.APIKey = value
	}
	if value, ok := source["apiHost"].(string); ok {
		config.APIHost = value
	}
	if value, ok := intValue(source["forecastDays"]); ok {
		config.ForecastDays = value
	}
	if value, ok := intValue(source["hourlyForecastInterval"]); ok {
		config.HourlyForecastInterval = value
	}
	if value, ok := intValue(source["hourlyForecastCount"]); ok {
		config.HourlyForecastCount = value
	}
	if value, ok := source["includeWarnings"].(bool); ok {
		config.IncludeWarnings = value
	}
	if value, ok := source["includeAirQuality"].(bool); ok {
		config.IncludeAirQuality = value
	}
}

func intValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case float64:
		return int(typed), true
	case float32:
		return int(typed), true
	case json.Number:
		value, err := typed.Int64()
		return int(value), err == nil
	default:
		return 0, false
	}
}

func cleanAPIHost(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	if parsed, err := url.Parse("https://" + value); err == nil {
		return strings.TrimSpace(parsed.Host)
	}
	return strings.Trim(value, "/")
}

func validForecastDays(days int) bool {
	switch days {
	case 3, 7, 10, 15:
		return true
	default:
		return false
	}
}
