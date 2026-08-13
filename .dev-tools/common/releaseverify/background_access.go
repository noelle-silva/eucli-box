package releaseverify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BackgroundAccess 后台运行与业务端访问设置验证。
func BackgroundAccess(ctx context.Context, repositoryRoot string, runRoot string, mode string) error {
	paths, err := prepareRun(repositoryRoot, runRoot, "verify-background-access")
	if err != nil {
		return err
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "default"
	}
	if mode != "default" && mode != "experience" {
		return fmt.Errorf("后台访问验证模式必须是 default 或 experience")
	}
	recorder := newRecorder("verify-background-access", mode, paths.root)
	fmt.Printf("后台运行与访问设置 %s 验证目录：%s\n", mode, paths.root)

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

	boxPath, err := prepareBackgroundAccessBox(ctx, repositoryRoot, paths, recorder)
	if err == nil {
		runBackgroundAccessVerification(ctx, repositoryRoot, paths, recorder, boxPath)
	}

	if dataErr == nil {
		dataAfter, snapshotErr := directorySnapshot(filepath.Join(repositoryRoot, "data"))
		if snapshotErr != nil {
			recorder.fail("确认真实数据未改变", snapshotErr)
		} else if err := compareSnapshots("真实 data 目录", dataBefore, dataAfter); err != nil {
			recorder.fail("确认真实数据未改变", err)
		} else {
			recorder.pass("确认真实数据未改变", "后台访问验证未写入真实 data 目录")
		}
	}
	if gitErr == nil {
		gitAfter, snapshotErr := gitSnapshot(repositoryRoot)
		if snapshotErr != nil {
			recorder.fail("确认源码未被验证改写", snapshotErr)
		} else if err := compareSnapshots("源码工作区", gitBefore, gitAfter); err != nil {
			recorder.fail("确认源码未被验证改写", err)
		} else {
			recorder.pass("确认源码未被验证改写", "后台访问验证只在本次隔离目录产生运行内容")
		}
	}
	return recorder.finish(paths)
}

// prepareBackgroundAccessBox 构建隔离业务端可执行文件供验证子进程使用。
func prepareBackgroundAccessBox(ctx context.Context, root string, paths runPaths, recorder *recorder) (string, error) {
	boxPath := filepath.Join(paths.environment, "runtime", "eucli-box.exe")
	if err := runCommand(ctx, paths, "构建隔离业务端", root, "go", "build", "-o", boxPath, "./cmd/eucli-box"); err != nil {
		recorder.fail("构建隔离业务端", err)
		return "", err
	}
	if info, err := os.Stat(boxPath); err != nil || info.IsDir() {
		if err == nil {
			err = fmt.Errorf("业务端可执行文件不是普通文件")
		}
		recorder.fail("读取隔离业务端", err)
		return "", err
	}
	recorder.pass("构建隔离业务端", "当前源码已编译为隔离可执行文件")
	return boxPath, nil
}

func runBackgroundAccessVerification(ctx context.Context, root string, paths runPaths, recorder *recorder, boxPath string) {
	runTest := "^TestBackgroundAccess$"
	if strings.TrimSpace(os.Getenv("EUCLI_BACKGROUND_ACCESS_MODE")) == "experience" {
		runTest = "^TestBackgroundAccessExperience$"
	}
	command := struct {
		name    string
		workdir string
		command string
		args    []string
		env     map[string]string
	}{
		name:    "后台运行、长期端口、长期 Key、权限边界与旧配置转换",
		workdir: root,
		command: "go",
		args:    []string{"test", "-tags", "eucli_background_access", "-run", runTest, "-count=1", "devtools/general-verification-tools/verify-background-access/backgroundaccessverify"},
		env: map[string]string{
			"EUCLI_BACKGROUND_ACCESS_RUN_ROOT": paths.root,
			"EUCLI_BACKGROUND_ACCESS_BOX":      boxPath,
			"EUCLI_BACKGROUND_ACCESS_MODE":     os.Getenv("EUCLI_BACKGROUND_ACCESS_MODE"),
		},
	}
	if err := runCommandWithEnvironment(ctx, paths, command.name, command.workdir, command.command, command.env, command.args...); err != nil {
		recorder.fail(command.name, err)
	} else {
		recorder.pass(command.name, "隔离验证通过，详细输出见对应 evidence 日志")
	}
}
