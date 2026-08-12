// Package toolkit 是开发工具运行基础模块。
//
// 它是《开发仓库资产定位规范》的落地实现：所有开发工具通过它统一建立运行目录，
// 保证本体、开工、产物三类运行资料全部落在 .dev-workspace/.dev-tools-runtime/<工具>/ 内，
// 不散落于系统临时区或主仓库根。
//
// 本模块语言无关：概念（本体格、开工格、产物格、临时格、缓存格）适用于任何语言实现的
// 工具；本包是 Go 语言实现，其他语言可依同一概念各自实现薄适配层。
package toolkit

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Run 是一次工具运行的全部落点。
type Run struct {
	ToolName string
	Version  string

	Root    string
	Program string
	Work    string
	Output  string
	Temp    string
	Cache   string

	repositoryRoot string
}

// PrepareRun 在运行区内建立本次运行目录（run-<时间戳>），返回全部落点。
// 运行根强制位于 <repositoryRoot>/.dev-workspace/.dev-tools-runtime/<工具名>/ 内，
// 且整条路径只沿普通目录建立（联接点与符号链接直接失败）。
func PrepareRun(repositoryRoot string, toolName string, version string) (*Run, error) {
	repositoryRoot, err := ExistingPlainDirectory(repositoryRoot, "仓库根目录")
	if err != nil {
		return nil, err
	}
	if !safeToolName(toolName) {
		return nil, fmt.Errorf("工具名只能包含字母、数字、连字符与下划线：%q", toolName)
	}
	run := &Run{
		ToolName:       toolName,
		Version:        version,
		repositoryRoot: repositoryRoot,
	}
	base := filepath.Join(repositoryRoot, ".dev-workspace", ".dev-tools-runtime", toolName)
	runID := time.Now().UTC().Format("20060102T150405.000000000Z")
	run.Root = filepath.Join(base, "run-"+runID)
	run.Program = filepath.Join(run.Root, "program")
	run.Work = filepath.Join(run.Root, "work")
	run.Output = filepath.Join(run.Root, "output")
	run.Temp = filepath.Join(run.Root, "temp")
	run.Cache = filepath.Join(run.Root, "cache")
	for _, dir := range []string{run.Root, run.Program, run.Work, run.Output, run.Temp, run.Cache} {
		if err := EnsurePlainDirectoryPath(repositoryRoot, dir, "运行资料目录"); err != nil {
			return nil, err
		}
	}
	return run, nil
}

func safeToolName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

// ProgramPath 返回指定版本的本体路径（本体格按版本分子目录，构建版本互不覆盖）。
func (r *Run) ProgramPath(version string) string {
	if version == "" {
		return filepath.Join(r.Program, "tool")
	}
	return filepath.Join(r.Program, version, "tool")
}

// Chdir 把进程工作目录切换到开工格（开工统一在运行区内）。
func (r *Run) Chdir() error {
	return os.Chdir(r.Work)
}

// Cleanup 清理本次运行的临时内容（临时格与缓存格）。
func (r *Run) Cleanup() error {
	for _, dir := range []string{r.Temp, r.Cache} {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("清理%s失败：%w", dir, err)
		}
	}
	return nil
}
