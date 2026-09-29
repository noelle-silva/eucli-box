package hypercortexreader

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"eucli-box/pkg/types"
)

// stringArg 读取字符串参数；required 为真时空值快速失败。
func stringArg(input types.ToolExecutionInput, key string, required bool) (string, error) {
	value, ok := argumentValue(input, key)
	if !ok {
		if required {
			return "", fmt.Errorf("argument %q is required", key)
		}
		return "", nil
	}
	text, err := stringValue(value, key)
	if err != nil {
		return "", err
	}
	if required && text == "" {
		return "", fmt.Errorf("argument %q is required", key)
	}
	return text, nil
}

// intArg 读取整数参数；缺省为 0。
func intArg(input types.ToolExecutionInput, key string) (int, error) {
	value, ok := argumentValue(input, key)
	if !ok {
		return 0, nil
	}
	return intValue(value, key)
}

// numberArg 读取数值参数（毫秒时间戳等）；缺省为 0。
func numberArg(input types.ToolExecutionInput, key string) (float64, error) {
	value, ok := argumentValue(input, key)
	if !ok {
		return 0, nil
	}
	return floatValue(value, key)
}

// stringListArg 读取字符串数组参数；接受字符串数组或逗号分隔字符串；缺省为空。
func stringListArg(input types.ToolExecutionInput, key string) ([]string, error) {
	value, ok := argumentValue(input, key)
	if !ok {
		return nil, nil
	}
	return stringListValue(value, key)
}

func argumentValue(input types.ToolExecutionInput, key string) (any, bool) {
	value, ok := input.Arguments[key]
	return value, ok && value != nil
}

func stringValue(value any, key string) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("argument %q must be a string", key)
	}
	return strings.TrimSpace(text), nil
}

func intValue(value any, key string) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		if math.Trunc(typed) != typed {
			return 0, fmt.Errorf("argument %q must be an integer", key)
		}
		return int(typed), nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, fmt.Errorf("argument %q must be an integer", key)
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, fmt.Errorf("argument %q must be an integer", key)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("argument %q must be an integer", key)
	}
}

func floatValue(value any, key string) (float64, error) {
	switch typed := value.(type) {
	case int:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case float64:
		return typed, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, fmt.Errorf("argument %q must be a number", key)
		}
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, fmt.Errorf("argument %q must be a number", key)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("argument %q must be a number", key)
	}
}

func stringListValue(value any, key string) ([]string, error) {
	switch typed := value.(type) {
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("argument %q must contain only strings", key)
			}
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items, nil
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, nil
		}
		parts := strings.Split(trimmed, ",")
		items := make([]string, 0, len(parts))
		for _, part := range parts {
			if item := strings.TrimSpace(part); item != "" {
				items = append(items, item)
			}
		}
		return items, nil
	default:
		return nil, fmt.Errorf("argument %q must be a string array or comma-separated string", key)
	}
}

// setString / setInt / setNumber / setStringList 按「有值才带」的原则装配请求参数，
// 缺省值语义全部交给 HyperCortex 接口自身处理，工具不私自兜底。
func setString(params map[string]any, key string, value string) {
	if strings.TrimSpace(value) != "" {
		params[key] = value
	}
}

func setInt(params map[string]any, key string, value int) {
	if value != 0 {
		params[key] = value
	}
}

func setNumber(params map[string]any, key string, value float64) {
	if value != 0 {
		params[key] = value
	}
}

func setStringList(params map[string]any, key string, value []string) {
	if len(value) > 0 {
		params[key] = value
	}
}
