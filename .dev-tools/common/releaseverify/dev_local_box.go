package releaseverify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DevLocalBox 验证开发态链路：
// 当前源码编译产物直接以普通模式启动业务端（无安装概念），
// 固定 Key 鉴权、工具开发源标记生效；验证只在隔离目录产生运行内容。
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
	boxPath, err := buildDevBox(ctx, root, paths, recorder)
	if err != nil {
		recorder.fail("编译隔离业务端", err)
		return
	}
	commands := []struct {
		name    string
		workdir string
		command string
		args    []string
		env     map[string]string
	}{
		{name: "开发态普通模式启动、鉴权与工具开发源标记", workdir: root, command: "go", args: []string{"test", "-tags", "eucli_devbox", "-run", "^TestDevBox", "-count=1", "devtools/general-verification-tools/verify-dev-box/devboxverify"}, env: devBoxTestEnvironment(paths, boxPath)},
	}
	for _, command := range commands {
		if err := runCommandWithEnvironment(ctx, paths, command.name, command.workdir, command.command, command.env, command.args...); err != nil {
			recorder.fail(command.name, err)
		} else {
			recorder.pass(command.name, "隔离验证通过，详细输出见对应 evidence 日志")
		}
	}
}

// buildDevBox 编译当前源码业务端可执行文件供验证子进程使用。
func buildDevBox(ctx context.Context, root string, paths runPaths, recorder *recorder) (string, error) {
	boxPath := filepath.Join(paths.environment, "runtime", "eucli-box.exe")
	if err := os.MkdirAll(filepath.Dir(boxPath), 0o755); err != nil {
		recorder.fail("建立隔离业务端目录", err)
		return "", err
	}
	if err := runCommand(ctx, paths, "编译当前源码业务端", root, "go", "build", "-o", boxPath, "./cmd/eucli-box"); err != nil {
		return "", err
	}
	if info, err := os.Stat(boxPath); err != nil || info.IsDir() {
		if err == nil {
			err = fmt.Errorf("业务端可执行文件不是普通文件")
		}
		return "", err
	}
	recorder.pass("编译当前源码业务端", "当前源码已编译为隔离可执行文件")
	return boxPath, nil
}

// devBoxTestEnvironment 为开发态测试提供隔离的运行环境变量。
func devBoxTestEnvironment(paths runPaths, boxPath string) map[string]string {
	return map[string]string{
		"EUCLI_DEV_BOX_BOX":      boxPath,
		"EUCLI_DEV_BOX_RUN_ROOT": paths.root,
	}
}
