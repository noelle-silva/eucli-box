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
	"strings"
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

// VerificationRun 是统一验证入口准备的一次运行现场。
type VerificationRun struct {
	RepositoryRoot string
	Root           string
	Inputs         string
	Workspace      string
	Environment    string
	Work           string
	Temp           string
	Cache          string
	Evidence       string
}

// PrepareVerificationRun 校验统一入口传入的 run-* 目录，轮替清除同工具旧轮现场，并建立验证现场的标准目录。
// 入口脚本预先建立 work、temp 与 cache；其他目录由本函数统一建立并校验边界。
func PrepareVerificationRun(repositoryRoot string, runRoot string, toolName string) (*VerificationRun, error) {
	repositoryRoot, runRoot, err := resolveVerificationRun(repositoryRoot, runRoot, toolName)
	if err != nil {
		return nil, err
	}
	if err := retirePreviousRuns(runRoot); err != nil {
		return nil, err
	}

	run := &VerificationRun{
		RepositoryRoot: repositoryRoot,
		Root:           runRoot,
		Inputs:         filepath.Join(runRoot, "inputs"),
		Workspace:      filepath.Join(runRoot, "workspace"),
		Environment:    filepath.Join(runRoot, "environment"),
		Work:           filepath.Join(runRoot, "work"),
		Temp:           filepath.Join(runRoot, "temp"),
		Cache:          filepath.Join(runRoot, "cache"),
		Evidence:       filepath.Join(runRoot, "evidence"),
	}
	for _, directory := range []string{
		run.Inputs,
		run.Workspace,
		run.Environment,
		run.Work,
		run.Temp,
		run.Cache,
		run.Evidence,
	} {
		if err := EnsurePlainDirectoryPath(repositoryRoot, directory, "验证运行资料目录"); err != nil {
			return nil, err
		}
	}
	return run, nil
}

// RetirePreviousRuns 轮替清除同一工具运行区内的旧验证现场，只保留当前 run 目录。
// 执行前先确认当前 run 目录确实是该工具运行区下的独立 run-* 目录且形状合法；
// 只删除与当前 run 同级的 run-* 目录，非 run-* 条目（公共缓存、工具本体等）一律不动。
func RetirePreviousRuns(repositoryRoot string, runRoot string, toolName string) error {
	_, runRoot, err := resolveVerificationRun(repositoryRoot, runRoot, toolName)
	if err != nil {
		return err
	}
	return retirePreviousRuns(runRoot)
}

// resolveVerificationRun 校验验证运行根的位置与既有形状，返回规范化后的仓库根与运行根。
func resolveVerificationRun(repositoryRoot string, runRoot string, toolName string) (string, string, error) {
	repositoryRoot, err := ExistingPlainDirectory(repositoryRoot, "仓库根目录")
	if err != nil {
		return "", "", err
	}
	runRoot, err = ExistingPlainDirectory(runRoot, "验证运行目录")
	if err != nil {
		return "", "", err
	}
	if !safeToolName(toolName) {
		return "", "", fmt.Errorf("工具名只能包含字母、数字、连字符与下划线：%q", toolName)
	}
	expectedParent := filepath.Join(repositoryRoot, ".dev-workspace", ".dev-tools-runtime", toolName)
	runName := filepath.Base(runRoot)
	if !SamePath(filepath.Dir(runRoot), expectedParent) || len(runName) <= len("run-") || !strings.HasPrefix(runName, "run-") {
		return "", "", fmt.Errorf("验证运行目录必须是 %s 下的独立 run-* 目录", expectedParent)
	}

	entries, err := os.ReadDir(runRoot)
	if err != nil {
		return "", "", fmt.Errorf("读取验证运行目录失败：%w", err)
	}
	for _, entry := range entries {
		if entry.Name() != "work" && entry.Name() != "temp" && entry.Name() != "cache" {
			return "", "", fmt.Errorf("验证运行目录包含入口之外的已有内容：%s", entry.Name())
		}
		if !entry.IsDir() {
			return "", "", fmt.Errorf("验证运行目录中的预备内容必须是目录：%s", entry.Name())
		}
	}
	return repositoryRoot, runRoot, nil
}

// retirePreviousRuns 删除运行区内除当前 run 外的全部 run-* 目录。
func retirePreviousRuns(runRoot string) error {
	runArea := filepath.Dir(runRoot)
	entries, err := os.ReadDir(runArea)
	if err != nil {
		return fmt.Errorf("读取验证运行区失败：%w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "run-") {
			continue
		}
		retired := filepath.Join(runArea, entry.Name())
		if SamePath(retired, runRoot) {
			continue
		}
		if err := os.RemoveAll(retired); err != nil {
			return fmt.Errorf("轮替清除旧验证现场 %s 失败：%w", retired, err)
		}
	}
	return nil
}

// DisposableDirectories 返回统一入口收尾所需的固定可清理目录顺序。
func (r *VerificationRun) DisposableDirectories() []string {
	return []string{r.Inputs, r.Workspace, r.Environment, r.Work, r.Temp, r.Cache}
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
