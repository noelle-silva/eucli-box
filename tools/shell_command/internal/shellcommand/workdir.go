package shellcommand

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
)

func resolveWorkdir(hostWorkingDirectory string, requestedWorkdir string) (string, error) {
	hostWorkingDirectory = strings.TrimSpace(hostWorkingDirectory)
	if hostWorkingDirectory == "" {
		return "", fmt.Errorf("hostWorkingDirectory is required")
	}
	hostWorkingDirectory = filepath.Clean(hostWorkingDirectory)
	workdir := strings.TrimSpace(requestedWorkdir)
	if workdir == "" {
		workdir = "."
	}
	if !filepath.IsAbs(workdir) {
		workdir = filepath.Join(hostWorkingDirectory, workdir)
	}
	resolved, err := filepath.Abs(workdir)
	if err != nil {
		return "", fmt.Errorf("resolve workdir: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("workdir does not exist: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workdir is not a directory")
	}
	return resolved, nil
}

// AnalyzerWorkdir 为命令分析器提供与实际执行一致的工作目录：
// 解析成功返回绝对工作目录，目录暂不可用时保留原始请求值，
// 让执行阶段给出权威错误，而不是让分析阶段改变错误时序。
func AnalyzerWorkdir(input types.ToolExecutionInput, requestedWorkdir string) string {
	resolved, err := resolveWorkdir(types.ToolPathBaseDirectory(input), requestedWorkdir)
	if err != nil {
		return requestedWorkdir
	}
	return resolved
}
