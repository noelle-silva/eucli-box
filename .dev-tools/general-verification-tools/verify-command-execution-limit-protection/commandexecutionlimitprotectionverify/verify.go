// Package verify 是「命令执行上限保护」长期验证的端到端编排：
// 输出 1MB 上限与中间省略、输出更新通道与一万条上限、进程树终止统一出口。
package verify

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"devtools/common/toolkit"
)

const (
	toolName    = "verify-command-execution-limit-protection"
	defaultMode = "default"
)

// Run 执行端到端验证。RepositoryRoot 是主仓库根，RunRoot 必须位于该工具在开发运行区的独立 run-* 中。
func Run(ctx context.Context, repositoryRoot string, runRoot string, mode string) error {
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
	root := run.RepositoryRoot
	recorder := toolkit.NewVerificationRecorder(toolName, mode, run.Root)
	fmt.Printf("命令执行上限保护验证目录：%s\n", run.Root)

	dataBefore, dataErr := toolkit.DirectorySnapshot(filepath.Join(root, "data"))
	if dataErr != nil {
		recorder.Fail("记录真实数据初始状态", dataErr)
	} else {
		recorder.Pass("记录真实数据初始状态", "已建立只读完整性快照")
	}
	gitBefore, gitErr := captureGitStatus(ctx, root, run)
	if gitErr != nil {
		recorder.Fail("记录源码初始状态", gitErr)
	} else {
		recorder.Pass("记录源码初始状态", "已记录当前工作区状态")
	}

	fixture, buildErr := buildFixture(ctx, root, run)
	if buildErr != nil {
		recorder.Fail("构建验证工具与测试替身", buildErr)
	} else {
		recorder.Pass("构建验证工具与测试替身", "shell_command、假 provider 已编译")
		runScenarios(ctx, root, run, recorder, fixture)
	}

	if dataErr == nil {
		dataAfter, snapshotErr := toolkit.DirectorySnapshot(filepath.Join(root, "data"))
		if snapshotErr != nil {
			recorder.Fail("确认真实数据未改变", snapshotErr)
		} else if err := toolkit.CompareSnapshots("真实 data 目录", dataBefore, dataAfter); err != nil {
			recorder.Fail("确认真实数据未改变", err)
		} else {
			recorder.Pass("确认真实数据未改变", "验证未写入真实 data 目录")
		}
	}
	if gitErr == nil {
		gitAfter, snapshotErr := captureGitStatus(ctx, root, run)
		if snapshotErr != nil {
			recorder.Fail("确认源码未被验证改写", snapshotErr)
		} else if err := toolkit.CompareSnapshots("源码工作区", gitBefore, gitAfter); err != nil {
			recorder.Fail("确认源码未被验证改写", err)
		} else {
			recorder.Pass("确认源码未被验证改写", "验证只在本次隔离目录产生运行内容")
		}
	}
	return recorder.Finish(run.Evidence, run.DisposableDirectories())
}

func runScenarios(ctx context.Context, root string, run *toolkit.VerificationRun, recorder *toolkit.VerificationRecorder, fixture fixture) {
	steps := []scenarioStep{
		{"输出中间省略", func() error { return scenarioElidedOutput(ctx, fixture) }},
		{"一万条更新上限", func() error { return scenarioUpdateFlood(ctx, fixture) }},
		{"进程树终止出口", func() error { return scenarioTreeTermination(ctx, fixture) }},
	}
	for _, step := range steps {
		if err := step.run(); err != nil {
			recorder.Fail(step.name, err)
		} else {
			recorder.Pass(step.name, "断言全部成立")
		}
	}
}

type scenarioStep struct {
	name string
	run  func() error
}

type fixture struct {
	root     string
	shellExe string
}

func executableExtension() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
