package everything

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveScopePath 解析可选的搜索范围：空值返回空串（交应用按自己的默认范围搜索）；
// 绝对路径直接归一，相对路径基于主机工作目录解析；结果必须是一个已存在的目录。
func resolveScopePath(hostWorkingDirectory string, value string) (string, error) {
	path := strings.TrimSpace(value)
	if path == "" {
		return "", nil
	}
	var resolved string
	if filepath.IsAbs(path) {
		resolved = filepath.Clean(path)
	} else {
		if filepath.VolumeName(path) != "" {
			return "", fmt.Errorf("scopePath must be absolute or relative without a volume name")
		}
		base := strings.TrimSpace(hostWorkingDirectory)
		if base == "" {
			return "", fmt.Errorf("hostWorkingDirectory is required when scopePath is relative")
		}
		resolved = filepath.Clean(filepath.Join(base, path))
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("scopePath does not exist: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("scopePath must be a directory")
	}
	return resolved, nil
}
