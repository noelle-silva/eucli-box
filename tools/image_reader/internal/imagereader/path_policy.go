package imagereader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/tools/image_reader/internal/types"
)

// PathPolicy 把调用参数中的相对路径解析为绝对路径。
// 基准与宿主围栏保持一致：工作区会话取首个注册目录，否则退回宿主工作目录。
type PathPolicy struct {
	baseDir string
}

// ResolvedPath 是一次路径解析的结果：绝对路径与展示用路径。
type ResolvedPath struct {
	Absolute string
	Display  string
}

// newPathPolicy 以执行输入的路径基准建立解析器；基准必须是存在的目录。
func newPathPolicy(input types.ToolExecutionInput) (PathPolicy, error) {
	baseInput := strings.TrimSpace(types.ToolPathBaseDirectory(input))
	if baseInput == "" {
		return PathPolicy{}, fmt.Errorf("path base directory is not provided")
	}
	absolute, err := filepath.Abs(baseInput)
	if err != nil {
		return PathPolicy{}, fmt.Errorf("resolve base directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return PathPolicy{}, fmt.Errorf("base directory is not available: %w", err)
	}
	if !info.IsDir() {
		return PathPolicy{}, fmt.Errorf("base directory is not a directory")
	}
	return PathPolicy{baseDir: filepath.Clean(absolute)}, nil
}

// Resolve 解析参数路径：相对路径以基准目录拼接，绝对路径原样规范化。
func (p PathPolicy) Resolve(inputPath string) (ResolvedPath, error) {
	requested := strings.TrimSpace(inputPath)
	if requested == "" {
		return ResolvedPath{}, fmt.Errorf("path is required")
	}
	if strings.ContainsRune(requested, '\x00') {
		return ResolvedPath{}, fmt.Errorf("path cannot contain null bytes")
	}
	resolved := requested
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(p.baseDir, resolved)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("resolve path: %w", err)
	}
	absolute = filepath.Clean(absolute)
	display := absolute
	if rel, err := filepath.Rel(p.baseDir, absolute); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		display = filepath.ToSlash(rel)
	}
	return ResolvedPath{Absolute: absolute, Display: display}, nil
}
