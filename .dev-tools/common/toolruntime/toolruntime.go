// Package toolruntime 提供开发工具运行区的统一运行机制。
//
// 它是《开发仓库资产定位规范》6.1 工具工作目录纪律的落地实现：工具的工作目录、
// 输出目录与证据目录一律落位于自身运行区（.dev-workspace/.dev-tools-runtime/<工具>/），
// 每轮只保留最新一轮现场与本轮成绩单，旧轮自动轮替清除；显式覆盖落点也受审计校验，
// 不得指向产品运行区或主仓库源码区。
//
// 本包与 toolkit 同属公共机制立点：任何制作、发布、准备类工具应通过本包获得运行区
// 事实与轮替能力，不得在工具内自写落点拼接或轮替逻辑。
package toolruntime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devtools/common/toolkit"
	"eucli-box/pkg/workspace"
)

// FindRepositoryRoot 从工作目录向上定位主仓库根：目录必须同时包含产品源码区 tools/
// 与开发工具区 .dev-tools/go.mod。与工具在哪个目录被调用无关。
func FindRepositoryRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("获取工作目录失败：%w", err)
	}
	for {
		if isRepositoryRoot(current) {
			return filepath.Clean(current), nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("未找到主仓库根（需同时存在 tools 目录与 .dev-tools/go.mod）")
		}
		current = parent
	}
}

// ValidateRepositoryRoot 解析并校验仓库根：显式传入值先行校验，空值从工作目录向上自动定位。
func ValidateRepositoryRoot(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return FindRepositoryRoot()
	}
	absolute, err := filepath.Abs(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("确定仓库根失败：%w", err)
	}
	if !isRepositoryRoot(absolute) {
		return "", fmt.Errorf("%s 不是有效的主仓库根（需同时存在 tools 目录与 .dev-tools/go.mod）", absolute)
	}
	return filepath.Clean(absolute), nil
}

func isRepositoryRoot(candidate string) bool {
	if info, err := os.Stat(filepath.Join(candidate, "tools")); err != nil || !info.IsDir() {
		return false
	}
	if info, err := os.Stat(filepath.Join(candidate, ".dev-tools", "go.mod")); err != nil || info.IsDir() {
		return false
	}
	return true
}

// RunID 生成本轮运行编号：<prefix>-<UTC 时间戳>，时间戳保证同工具多轮之间严格有序。
func RunID(prefix string) string {
	return prefix + "-" + time.Now().UTC().Format("20060102T150405.000000000Z")
}

// Root 返回工具自身运行区根。
func Root(repositoryRoot string, tool string) string {
	return workspace.ToolRuntimeRoot(repositoryRoot, tool)
}

// PrepareRunDir 在 <runtimeRoot>/<subdir> 下建立本轮现场：
// 同前缀旧现场全部轮替清除（仅保留本轮），随后新建本轮目录。
func PrepareRunDir(runtimeRoot string, subdir string, prefix string) (string, error) {
	if strings.TrimSpace(prefix) == "" {
		return "", fmt.Errorf("运行现场前缀不能为空")
	}
	workRoot := filepath.Join(runtimeRoot, subdir)
	if err := toolkit.EnsurePlainDirectoryPath(runtimeRoot, workRoot, "工具运行格"); err != nil {
		return "", err
	}
	oldRunDirs, err := os.ReadDir(workRoot)
	if err != nil {
		return "", fmt.Errorf("读取运行格 %s 失败：%w", workRoot, err)
	}
	runPrefix := prefix + "-"
	for _, entry := range oldRunDirs {
		if !entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), runPrefix) {
			continue
		}
		oldRun := filepath.Join(workRoot, entry.Name())
		if err := os.RemoveAll(oldRun); err != nil {
			return "", fmt.Errorf("轮替清除旧现场 %s 失败：%w", oldRun, err)
		}
	}
	runDir := filepath.Join(workRoot, RunID(prefix))
	if err := toolkit.EnsurePlainDirectoryPath(runtimeRoot, runDir, "本轮运行现场"); err != nil {
		return "", err
	}
	return runDir, nil
}

// WriteScorecard 把本轮成绩单写入运行区 <runtimeRoot>/work/<prefix>-scorecard.json。
// 成绩单固定单文件路径，每轮覆盖写（不预置、不累积），旧一轮的成绩单被本轮取代。
func WriteScorecard(runtimeRoot string, prefix string, value any) error {
	if strings.TrimSpace(prefix) == "" {
		return fmt.Errorf("成绩单前缀不能为空")
	}
	workRoot := filepath.Join(runtimeRoot, "work")
	if err := toolkit.EnsurePlainDirectoryPath(runtimeRoot, workRoot, "工具运行格"); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("生成成绩单失败：%w", err)
	}
	payload = append(payload, '\n')
	target := filepath.Join(workRoot, prefix+"-scorecard.json")
	temporary := target + ".temporary"
	if err := os.WriteFile(temporary, payload, 0o644); err != nil {
		return fmt.Errorf("写入成绩单失败：%w", err)
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("启用成绩单失败：%w", err)
	}
	return nil
}

// ValidateWorkLocation 审计校验显式传入的工作、输出、证据落点：
// 仓库之内必须位于开发工作区（.dev-workspace）内且不得指向产品运行区（.dev-runtime）；
// 仓库之外的路径合法（工具可显式把现场放到仓库外，如另行指定的临时盘）。
func ValidateWorkLocation(repositoryRoot string, targets ...string) error {
	repositoryRoot, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return fmt.Errorf("确定仓库根失败：%w", err)
	}
	repositoryRoot = filepath.Clean(repositoryRoot)
	workspaceRoot := filepath.Join(repositoryRoot, workspace.WorkspaceDirectory)
	productRuntimeRoot := filepath.Join(repositoryRoot, workspace.WorkspaceDirectory, workspace.RuntimeDirectory)
	for _, target := range targets {
		value := strings.TrimSpace(target)
		if value == "" {
			continue
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			return fmt.Errorf("落点 %s 无法确定绝对路径：%w", target, err)
		}
		absolute = filepath.Clean(absolute)
		if within(repositoryRoot, absolute) && !within(workspaceRoot, absolute) {
			return fmt.Errorf("落点 %s 落在主仓库源码区，工具运行资料不得写入源码区", target)
		}
		if within(productRuntimeRoot, absolute) {
			return fmt.Errorf("落点 %s 落在产品运行区，工具运行资料不得指入产品运行区", target)
		}
	}
	return nil
}

func within(base string, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
