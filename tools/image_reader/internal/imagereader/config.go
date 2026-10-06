package imagereader

import (
	"fmt"
	"math"

	"eucli-box/tools/image_reader/internal/types"
)

const (
	defaultMaxImageBytes  = 4 * 1024 * 1024
	defaultMaxSourceBytes = 64 * 1024 * 1024
	defaultMaxDimension   = 1568
	defaultJPEGQuality    = 80
)

// Config 是一次读取的图片体积与压缩规格：
// maxImageBytes 是入库上限（触发压缩与压缩后拒绝的同一阈值），
// maxSourceBytes 是源文件读取硬上限，maxDimension/jpegQuality 是压缩规格。
type Config struct {
	MaxImageBytes  int64
	MaxSourceBytes int64
	MaxDimension   int
	JPEGQuality    int
}

// loadConfig 从用户配置与默认配置合并读取规格；任一非法值快速失败。
func loadConfig(input types.ToolExecutionInput) (Config, error) {
	config := Config{
		MaxImageBytes:  defaultMaxImageBytes,
		MaxSourceBytes: defaultMaxSourceBytes,
		MaxDimension:   defaultMaxDimension,
		JPEGQuality:    defaultJPEGQuality,
	}
	var err error
	if config.MaxImageBytes, err = mergedInt64(input, "maxImageBytes", config.MaxImageBytes); err != nil {
		return Config{}, err
	}
	if config.MaxSourceBytes, err = mergedInt64(input, "maxSourceBytes", config.MaxSourceBytes); err != nil {
		return Config{}, err
	}
	if config.MaxDimension, err = mergedInt(input, "maxDimension", config.MaxDimension); err != nil {
		return Config{}, err
	}
	if config.JPEGQuality, err = mergedInt(input, "jpegQuality", config.JPEGQuality); err != nil {
		return Config{}, err
	}
	if config.MaxImageBytes <= 0 {
		return Config{}, fmt.Errorf("maxImageBytes must be greater than zero")
	}
	if config.MaxSourceBytes <= 0 {
		return Config{}, fmt.Errorf("maxSourceBytes must be greater than zero")
	}
	if config.MaxSourceBytes < config.MaxImageBytes {
		return Config{}, fmt.Errorf("maxSourceBytes must not be smaller than maxImageBytes")
	}
	if config.MaxDimension <= 0 {
		return Config{}, fmt.Errorf("maxDimension must be greater than zero")
	}
	if config.JPEGQuality < 1 || config.JPEGQuality > 100 {
		return Config{}, fmt.Errorf("jpegQuality must be between 1 and 100")
	}
	return config, nil
}

func mergedValue(input types.ToolExecutionInput, key string) (any, bool) {
	if value, ok := input.UserConfig[key]; ok && value != nil {
		return value, true
	}
	if value, ok := input.DefaultConfig[key]; ok && value != nil {
		return value, true
	}
	return nil, false
}

func mergedInt(input types.ToolExecutionInput, key string, fallback int) (int, error) {
	value, ok := mergedValue(input, key)
	if !ok {
		return fallback, nil
	}
	return intValue(value, key)
}

func mergedInt64(input types.ToolExecutionInput, key string, fallback int64) (int64, error) {
	value, ok := mergedValue(input, key)
	if !ok {
		return fallback, nil
	}
	parsed, err := intValue(value, key)
	if err != nil {
		return 0, err
	}
	return int64(parsed), nil
}

func intValue(value any, key string) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		if math.Trunc(typed) != typed {
			return 0, fmt.Errorf("config %q must be an integer", key)
		}
		return int(typed), nil
	default:
		return 0, fmt.Errorf("config %q must be an integer", key)
	}
}
