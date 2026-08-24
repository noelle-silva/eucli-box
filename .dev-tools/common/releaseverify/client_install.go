package releaseverify

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

func VerifyClientInstall(ctx context.Context, repositoryRoot string, runRoot string, mode string) error {
	paths, err := prepareRun(repositoryRoot, runRoot, "verify-client-install")
	if err != nil {
		return err
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "default"
	}
	if mode != "default" {
		return fmt.Errorf("客户端验证模式必须是 default")
	}
	recorder := newRecorder("verify-client-install", mode, paths.root)
	fmt.Printf("客户端验证 %s 验证目录：%s\n", mode, paths.root)

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

	runClientInstallVerification(ctx, repositoryRoot, paths, recorder)

	if dataErr == nil {
		dataAfter, snapshotErr := directorySnapshot(filepath.Join(repositoryRoot, "data"))
		if snapshotErr != nil {
			recorder.fail("确认真实数据未改变", snapshotErr)
		} else if err := compareSnapshots("真实 data 目录", dataBefore, dataAfter); err != nil {
			recorder.fail("确认真实数据未改变", err)
		} else {
			recorder.pass("确认真实数据未改变", "客户端验证未写入真实 data 目录")
		}
	}
	if gitErr == nil {
		gitAfter, snapshotErr := gitSnapshot(repositoryRoot)
		if snapshotErr != nil {
			recorder.fail("确认源码未被验证改写", snapshotErr)
		} else if err := compareSnapshots("源码工作区", gitBefore, gitAfter); err != nil {
			recorder.fail("确认源码未被验证改写", err)
		} else {
			recorder.pass("确认源码未被验证改写", "客户端验证只在本次隔离目录产生运行内容")
		}
	}
	return recorder.finish(paths)
}

func runClientInstallVerification(ctx context.Context, root string, paths runPaths, recorder *recorder) {
	commands := []struct {
		name    string
		workdir string
		command string
		args    []string
	}{
		{name: "公共发行候选、下载与包核对", workdir: root, command: "go", args: []string{"test", "./pkg/release", "./pkg/releasecatalog", "./pkg/releasecheck", "devtools/common/releaseartifact", "devtools/common/releasepublish", "-count=1"}},
		{name: "数据锁、网关与业务端测试", workdir: root, command: "go", args: []string{"test", "./pkg/localrun", "./src/gateway-system", "./cmd/eucli-box", "-count=1"}},
		{name: "客户端后台整体测试", workdir: filepath.Join(root, "clients", "eucli-studio", "backend-go"), command: "go", args: []string{"test", "./...", "-count=1"}},
		{name: "客户端协议和界面类型", workdir: filepath.Join(root, "clients", "eucli-studio"), command: "pnpm", args: []string{"exec", "tsc", "--noEmit"}},
		{name: "客户端界面构建", workdir: filepath.Join(root, "clients", "eucli-studio"), command: "pnpm", args: []string{"build:ui"}},
	}
	for _, command := range commands {
		if err := runCommandWithEnvironment(ctx, paths, command.name, command.workdir, command.command, nil, command.args...); err != nil {
			recorder.fail(command.name, err)
		} else {
			recorder.pass(command.name, "隔离验证通过，详细输出见对应 evidence 日志")
		}
	}
}
