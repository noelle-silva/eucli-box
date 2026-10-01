package imagereader

import (
	"errors"
	"strings"

	"eucli-box/pkg/types"
)

// arguments 是一次工具调用的完整参数视图。
type arguments struct {
	Path string
}

// parseArguments 解析模型传入的参数：path 必填且必须是非空字符串。
func parseArguments(raw map[string]any) (arguments, error) {
	if raw == nil {
		raw = map[string]any{}
	}
	path, err := requiredString(raw["path"], "path")
	if err != nil {
		return arguments{}, err
	}
	return arguments{Path: path}, nil
}

func requiredString(value any, key string) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", errors.New("argument " + key + " must be a string")
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("argument " + key + " is required")
	}
	return strings.TrimSpace(text), nil
}

// rejectConfigArguments 拒绝把配置项当作调用参数传入：配置只走用户设置，
// 模型调用不得越权改写读取阈值。
func rejectConfigArguments(input types.ToolExecutionInput) error {
	configKeys := []string{"maxImageBytes", "maxSourceBytes", "maxDimension", "jpegQuality"}
	for _, key := range configKeys {
		if value, ok := input.Arguments[key]; ok && value != nil {
			return errors.New("argument " + key + " is configuration-only and cannot be supplied by a tool call")
		}
	}
	return nil
}
