// Package verify 是「导入边界」长期验证的编排：
// 逐个边界单元（AI 工具与系统插件）扫描源码引用，凡引用本仓库内、但不在该单元自己文件夹里的代码即判定不合格。
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

// boundaryCategories 是本校验覆盖的全部源码区：AI 工具与系统插件共用同一套校验机制。
var boundaryCategories = []boundaryCategory{
	{Key: "tools", Label: "AI 工具"},
	{Key: "system-plugins", Label: "系统插件"},
}

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
	fmt.Printf("导入边界验证目录：%s\n", run.Root)

	rootModule, moduleErr := readModulePath(filepath.Join(root, "go.mod"))
	if moduleErr != nil {
		recorder.Fail("读取仓库模块路径", moduleErr)
		return recorder.Finish(run.Evidence, run.DisposableDirectories())
	}
	repoModules, modulesErr := discoverRepositoryModules(root)
	if modulesErr != nil {
		recorder.Fail("识别仓库内模块", modulesErr)
		return recorder.Finish(run.Evidence, run.DisposableDirectories())
	}
	recorder.Pass("识别仓库内模块", strings.Join(repoModules, "、"))

	for _, category := range boundaryCategories {
		verifyCategory(recorder, root, rootModule, repoModules, category)
	}

	return recorder.Finish(run.Evidence, run.DisposableDirectories())
}

// verifyCategory 校验一类源码区：建立只读完整性快照、逐个单元判定、确认源码未被校验改动。
func verifyCategory(recorder *toolkit.VerificationRecorder, root string, rootModule string, repoModules []string, category boundaryCategory) {
	categoryDir := filepath.Join(root, category.Key)
	before, snapshotErr := toolkit.DirectorySnapshot(categoryDir)
	if snapshotErr != nil {
		recorder.Fail("记录"+category.Label+"源码初始状态", snapshotErr)
	} else {
		recorder.Pass("记录"+category.Label+"源码初始状态", "已建立只读完整性快照")
	}

	units, unitsErr := discoverUnits(categoryDir)
	if unitsErr != nil {
		recorder.Fail("枚举"+category.Label, unitsErr)
	} else {
		recorder.Pass("枚举"+category.Label, fmt.Sprintf("共 %d 个", len(units)))
		for _, target := range units {
			checkUnit(recorder, root, rootModule, repoModules, category, target)
		}
	}

	if snapshotErr == nil {
		after, afterErr := toolkit.DirectorySnapshot(categoryDir)
		if afterErr != nil {
			recorder.Fail("确认"+category.Label+"源码未被校验改动", afterErr)
		} else if err := toolkit.CompareSnapshots(category.Label+"源码目录", before, after); err != nil {
			recorder.Fail("确认"+category.Label+"源码未被校验改动", err)
		} else {
			recorder.Pass("确认"+category.Label+"源码未被校验改动", "校验只读，源码目录未发生任何变化")
		}
	}
}

// checkUnit 判定单个边界单元的引用边界并记录一条结论。
func checkUnit(recorder *toolkit.VerificationRecorder, root string, rootModule string, repoModules []string, category boundaryCategory, target unit) {
	name := category.Label + " " + target.Name
	allowedPrefix, err := unitImportPrefix(rootModule, category, target)
	if err != nil {
		recorder.Fail(name, err)
		return
	}
	violations, err := scanUnitImports(root, target, repoModules, allowedPrefix)
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
