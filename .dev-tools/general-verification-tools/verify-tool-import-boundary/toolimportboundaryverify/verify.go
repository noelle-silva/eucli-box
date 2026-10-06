// Package verify 是「AI 工具导入边界」长期验证的编排：
// 逐个工具扫描源码引用，凡引用本仓库内、但不在该工具自己文件夹里的代码即判定不合格。
package verify

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"devtools/common/toolkit"
)

const (
	toolName    = "verify-tool-import-boundary"
	defaultMode = "default"
)

// Run 执行导入边界验证。RepositoryRoot 是主仓库根，RunRoot 必须位于该工具在开发运行区的独立 run-* 中。
func Run(_ context.Context, repositoryRoot string, runRoot string, mode string) error {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = defaultMode
	}
	if mode != defaultMode {
		return fmt.Errorf("本工具只接受 %s", defaultMode)
	}
	run, err := toolkit.PrepareVerificationRun(repositoryRoot, runRoot, toolName)
	if err != nil {
		return err
	}
	recorder := toolkit.NewVerificationRecorder(toolName, mode, run.Root)
	root := run.RepositoryRoot
	fmt.Printf("AI 工具导入边界验证目录：%s\n", run.Root)

	toolsDir := filepath.Join(root, "tools")
	toolsBefore, snapshotErr := toolkit.DirectorySnapshot(toolsDir)
	if snapshotErr != nil {
		recorder.Fail("记录工具源码初始状态", snapshotErr)
	} else {
		recorder.Pass("记录工具源码初始状态", "已建立只读完整性快照")
	}

	rootModule, moduleErr := readModulePath(filepath.Join(root, "go.mod"))
	if moduleErr != nil {
		recorder.Fail("读取仓库模块路径", moduleErr)
	} else {
		repoModules, modulesErr := discoverRepositoryModules(root)
		if modulesErr != nil {
			recorder.Fail("识别仓库内模块", modulesErr)
		} else {
			recorder.Pass("识别仓库内模块", strings.Join(repoModules, "、"))
			tools, toolsErr := discoverTools(toolsDir)
			if toolsErr != nil {
				recorder.Fail("枚举 AI 工具", toolsErr)
			} else {
				recorder.Pass("枚举 AI 工具", fmt.Sprintf("共 %d 个工具", len(tools)))
				for _, tool := range tools {
					checkTool(recorder, root, rootModule, repoModules, tool)
				}
			}
		}
	}

	if snapshotErr == nil {
		toolsAfter, afterErr := toolkit.DirectorySnapshot(toolsDir)
		if afterErr != nil {
			recorder.Fail("确认工具源码未被校验改动", afterErr)
		} else if err := toolkit.CompareSnapshots("工具源码目录", toolsBefore, toolsAfter); err != nil {
			recorder.Fail("确认工具源码未被校验改动", err)
		} else {
			recorder.Pass("确认工具源码未被校验改动", "校验只读，工具源码目录未发生任何变化")
		}
	}

	return recorder.Finish(run.Evidence, run.DisposableDirectories())
}

// checkTool 判定单个工具的引用边界并记录一条结论。
func checkTool(recorder *toolkit.VerificationRecorder, root string, rootModule string, repoModules []string, tool Tool) {
	name := "工具 " + tool.Name
	allowedPrefix, err := toolImportPrefix(rootModule, tool)
	if err != nil {
		recorder.Fail(name, err)
		return
	}
	violations, err := scanToolImports(root, tool, repoModules, allowedPrefix)
	if err != nil {
		recorder.Fail(name, err)
		return
	}
	if len(violations) == 0 {
		recorder.Pass(name, "未发现越界引用")
		return
	}
	recorder.Fail(name, fmt.Errorf("越界引用 %d 处：%s", len(violations), formatViolations(violations)))
}

// formatViolations 把越界引用汇总为可读文本，逐条给出文件、行号与引用路径。
func formatViolations(violations []ImportRef) string {
	parts := make([]string, 0, len(violations))
	for _, violation := range violations {
		parts = append(parts, fmt.Sprintf("%s:%d -> %s", violation.File, violation.Line, violation.Import))
	}
	return strings.Join(parts, "；")
}
