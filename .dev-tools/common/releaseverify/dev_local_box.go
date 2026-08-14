package releaseverify

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// DevLocalBox 验证开发体验入口和开发来源安装链：
// 开发模式必须使用当前源码本地成品安装业务端，缺失或损坏时明确失败且不回退官方来源；
// 开发客户端和业务端资料只进入 .dev-runtime 边界；正式来源语义保持不变。
func DevLocalBox(ctx context.Context, repositoryRoot string, runRoot string, mode string) error {
	paths, err := prepareRun(repositoryRoot, runRoot, "verify-dev-box")
	if err != nil {
		return err
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "default"
	}
	if mode != "default" {
		return fmt.Errorf("开发业务端验证模式必须是 default")
	}
	recorder := newRecorder("verify-dev-box", "default", paths.root)
	fmt.Printf("开发业务端验证目录：%s\n", paths.root)

	dataBefore, dataErr := directorySnapshot(filepath.Join(repositoryRoot, "data"))
	gitBefore, gitErr := gitSnapshot(repositoryRoot)
	if dataErr != nil {
		recorder.fail("记录真实数据初始状态", dataErr)
	} else {
		recorder.pass("记录真实数据初始状态", "已建立只读完整性快照")
	}
	if gitErr != nil {
		recorder.fail("记录源码初始状态", gitErr)
	} else {
		recorder.pass("记录源码初始状态", "已记录当前工作区状态")
	}

	runDevLocalBoxDefault(ctx, repositoryRoot, paths, recorder)

	if dataErr == nil {
		dataAfter, snapshotErr := directorySnapshot(filepath.Join(repositoryRoot, "data"))
		if snapshotErr != nil {
			recorder.fail("确认真实数据未改变", snapshotErr)
		} else if err := compareSnapshots("真实 data 目录", dataBefore, dataAfter); err != nil {
			recorder.fail("确认真实数据未改变", err)
		} else {
			recorder.pass("确认真实数据未改变", "开发业务端验证未写入真实 data 目录")
		}
	}
	if gitErr == nil {
		gitAfter, snapshotErr := gitSnapshot(repositoryRoot)
		if snapshotErr != nil {
			recorder.fail("确认源码未被验证改写", snapshotErr)
		} else if err := compareSnapshots("源码工作区", gitBefore, gitAfter); err != nil {
			recorder.fail("确认源码未被验证改写", err)
		} else {
			recorder.pass("确认源码未被验证改写", "开发业务端验证只在本次隔离目录产生运行内容")
		}
	}
	return recorder.finish(paths)
}

func runDevLocalBoxDefault(ctx context.Context, root string, paths runPaths, recorder *recorder) {
	archivePath, manifestPath, err := prepareVerificationBox(ctx, root, paths, "开发体验业务端成品", filepath.Join(paths.environment, "dev-runtime", "eucli-box", "package"), recorder)
	if err != nil {
		return
	}
	commands := []struct {
		name    string
		workdir string
		command string
		args    []string
		env     map[string]string
	}{
		{name: "开发来源安装、失败和重装场景", workdir: filepath.Join(root, "clients", "eucli-studio", "backend-go"), command: "go", args: []string{"test", "-tags", "eucli_devbox", "-run", "^TestDevBox", "-count=1"}, env: devLocalBoxTestEnvironment(paths, archivePath, manifestPath)},
		{name: "客户端本地后台整体测试（含正式来源回归）", workdir: filepath.Join(root, "clients", "eucli-studio", "backend-go"), command: "go", args: []string{"test", "./...", "-count=1"}},
		{name: "客户端协议和界面类型", workdir: filepath.Join(root, "clients", "eucli-studio"), command: "pnpm", args: []string{"exec", "tsc", "--noEmit"}},
		{name: "客户端界面构建", workdir: filepath.Join(root, "clients", "eucli-studio"), command: "pnpm", args: []string{"build:ui"}},
	}
	for _, command := range commands {
		if err := runCommandWithEnvironment(ctx, paths, command.name, command.workdir, command.command, command.env, command.args...); err != nil {
			recorder.fail(command.name, err)
		} else {
			recorder.pass(command.name, "隔离验证通过，详细输出见对应 evidence 日志")
		}
	}
}

// devLocalBoxTestEnvironment 为开发业务端测试提供隔离的开发资料布局，
// 与开发体验入口 .dev-workspace/.dev-runtime/ 的目录语义一致，
// 环境变量名与真实入口 start-dev-box.ps1 和客户端读取方 local_box_source.go 完全对齐。
func devLocalBoxTestEnvironment(paths runPaths, archivePath string, manifestPath string) map[string]string {
	devRuntime := filepath.Join(paths.environment, "dev-runtime")
	return map[string]string{
		"EUCLI_DEV_BOX_SOURCE":   "1",
		"EUCLI_DEV_BOX_MANIFEST": manifestPath,
		"EUCLI_DEV_BOX_ARCHIVE":  archivePath,
		"EUCLI_DEV_BOX_BOX_ROOT": filepath.Join(devRuntime, "eucli-box"),
		"FW_APP_DATA_DIR":        filepath.Join(devRuntime, "client", "data"),
	}
}
