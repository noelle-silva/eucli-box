package types

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ToolWorkDirectoryConfig 是「工具默认工作目录」配置：AI 工具在没有工作区的
// 会话中干活的位置。它恒有值、可被用户修改，与 eucli-box 部署位置无关。
type ToolWorkDirectoryConfig struct {
	Directory string    `json:"directory"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// DefaultToolWorkDirectory 返回默认的工具工作目录：系统临时区下的
// eucli-box-temp（Windows 跟随 %TEMP%、Linux 跟随 /tmp 或 $TMPDIR），
// 不写死任何盘符，也不落在 eucli-box 部署目录。
func DefaultToolWorkDirectory() string {
	return filepath.Join(os.TempDir(), "eucli-box-temp")
}

// NormalizeToolWorkDirectory 归一化工具工作目录值：空值回默认目录，
// 非空值转为绝对路径；"工具工作目录"恒有值，不存在未设置状态。
func NormalizeToolWorkDirectory(directory string) string {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		directory = DefaultToolWorkDirectory()
	}
	if absolute, err := filepath.Abs(directory); err == nil {
		directory = filepath.Clean(absolute)
	}
	return directory
}
