package types

import (
	"errors"
	"strings"
)

// ToolConfigDirName 是工具数据区内可编辑配置区的固定子目录名。
// 宿主与工具共同遵守这一约定：宿主只做存取展示，工具解释配置语义。
const ToolConfigDirName = "config"

// ToolConfigFileMaxBytes 是单个工具配置文件的大小上限，读写两侧共同遵守。
const ToolConfigFileMaxBytes = 1 << 20

// CleanToolConfigRelPath 规范化工具配置区内的相对文件路径：
// 统一使用 / 分隔，拒绝空路径、绝对路径、盘符路径与上跳分段；
// 通过后路径保证落在配置区内。
func CleanToolConfigRelPath(value string) (string, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return "", errors.New("工具配置文件路径不能为空")
	}
	text = strings.ReplaceAll(text, "\\", "/")
	if strings.HasPrefix(text, "/") {
		return "", errors.New("工具配置文件路径必须是相对路径")
	}
	segments := strings.Split(text, "/")
	cleaned := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("工具配置文件路径包含非法分段")
		}
		if strings.Contains(segment, ":") {
			return "", errors.New("工具配置文件路径包含非法字符")
		}
		cleaned = append(cleaned, segment)
	}
	return strings.Join(cleaned, "/"), nil
}
