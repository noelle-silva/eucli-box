package releaseverify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/workspace"
)

func Stage03(ctx context.Context, repositoryRoot string, runRoot string, mode string) error {
	paths, err := prepareRun(repositoryRoot, runRoot, "03")
	if err != nil {
		return err
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "default"
	}
	if mode != "default" && mode != "experience" {
		return fmt.Errorf("阶段三模式必须是 default 或 experience")
	}
	recorder := newRecorder("03", mode, paths.root)
	fmt.Printf("阶段三 %s 验证目录：%s\n", mode, paths.root)

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

	if mode == "experience" {
		runStage03Experience(ctx, repositoryRoot, paths, recorder)
	} else {
		runStage03Default(ctx, repositoryRoot, paths, recorder)
	}

	if dataErr == nil {
		dataAfter, snapshotErr := directorySnapshot(filepath.Join(repositoryRoot, "data"))
		if snapshotErr != nil {
			recorder.fail("确认真实数据未改变", snapshotErr)
		} else if err := compareSnapshots("真实 data 目录", dataBefore, dataAfter); err != nil {
			recorder.fail("确认真实数据未改变", err)
		} else {
			recorder.pass("确认真实数据未改变", "阶段三验证未写入真实 data 目录")
		}
	}
	if gitErr == nil {
		gitAfter, snapshotErr := gitSnapshot(repositoryRoot)
		if snapshotErr != nil {
			recorder.fail("确认源码未被验证改写", snapshotErr)
		} else if err := compareSnapshots("源码工作区", gitBefore, gitAfter); err != nil {
			recorder.fail("确认源码未被验证改写", err)
		} else {
			recorder.pass("确认源码未被验证改写", "阶段三验证只在本次隔离目录产生运行内容")
		}
	}
	return recorder.finish(paths)
}

func runStage03Default(ctx context.Context, root string, paths runPaths, recorder *recorder) {
	archivePath, manifestPath, err := prepareStage03Box(ctx, root, paths, recorder)
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
		{name: "公共发行候选、下载与包核对", workdir: root, command: "go", args: []string{"test", "./pkg/release", "./pkg/releasecatalog", "./pkg/releasecheck", "./internal/releaseartifact", "./internal/releasepublish", "-count=1"}},
		{name: "本机运行事实、受托启动与网关路由", workdir: root, command: "go", args: []string{"test", "./pkg/localrun", "./src/gateway-system", "./cmd/eucli-box", "-count=1"}},
		{name: "客户端本地后台安装、连接和故障场景", workdir: filepath.Join(root, "clients", "eucli-studio", "backend-go"), command: "go", args: []string{"test", "-tags", "eucli_stage03", "-run", "^TestStage03", "-count=1"}, env: stage03LifecycleEnvironment(paths, archivePath, manifestPath)},
		{name: "客户端后台其他测试", workdir: filepath.Join(root, "clients", "eucli-studio", "backend-go"), command: "go", args: []string{"test", "./...", "-count=1"}},
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

func runStage03Experience(ctx context.Context, root string, paths runPaths, recorder *recorder) {
	archivePath, manifestPath, err := prepareStage03Box(ctx, root, paths, recorder)
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
		{name: "体验模式隔离安装、自动连接和退出准备", workdir: filepath.Join(root, "clients", "eucli-studio", "backend-go"), command: "go", args: []string{"test", "-tags", "eucli_stage03", "-run", "^TestStage03LocalBoxLifecycle$", "-count=1"}, env: stage03LifecycleEnvironment(paths, archivePath, manifestPath)},
		{name: "体验模式界面构建", workdir: filepath.Join(root, "clients", "eucli-studio"), command: "pnpm", args: []string{"build:ui"}},
	}
	for _, command := range commands {
		if err := runCommandWithEnvironment(ctx, paths, command.name, command.workdir, command.command, command.env, command.args...); err != nil {
			recorder.fail(command.name, err)
		} else {
			recorder.pass(command.name, "体验模式基础链通过，详细输出见对应 evidence 日志")
		}
	}
}

func prepareStage03Box(ctx context.Context, root string, paths runPaths, recorder *recorder) (string, string, error) {
	return prepareVerificationBox(ctx, root, paths, "阶段三隔离业务端成品", filepath.Join(paths.environment, "box-release"), recorder)
}

// prepareVerificationBox 使用当前源码制作 Windows x64 业务端验证成品并输出到指定目录，
// 返回压缩包和清单路径。制作失败时记录检查结果并停止后续依赖它的验证。
func prepareVerificationBox(ctx context.Context, root string, paths runPaths, name string, outputRoot string, recorder *recorder) (string, string, error) {
	resultPath := filepath.Join(paths.environment, "box-build-result.json")
	buildCommand := []string{"run", "./cmd/eucli-release", "build", "-root", root, "-target", "eucli-box", "-work-root", filepath.Join(paths.workspace, "box-build"), "-output-root", outputRoot, "-evidence-root", filepath.Join(paths.evidence, "box-build"), "-asset-root", workspace.VerificationAssetRoot(root), "-verification-only", "-result-file", resultPath}
	if err := runCommand(ctx, paths, "制作"+name, root, "go", buildCommand...); err != nil {
		recorder.fail("制作"+name, err)
		return "", "", err
	}
	var result struct {
		ArchivePath  string `json:"archivePath"`
		ManifestPath string `json:"manifestPath"`
	}
	payload, err := os.ReadFile(resultPath)
	if err == nil {
		err = json.Unmarshal(payload, &result)
	}
	if err != nil || strings.TrimSpace(result.ArchivePath) == "" || strings.TrimSpace(result.ManifestPath) == "" {
		if err == nil {
			err = fmt.Errorf(name + "资料缺少成品路径")
		}
		recorder.fail("读取"+name+"资料", err)
		return "", "", err
	}
	for _, path := range []string{result.ArchivePath, result.ManifestPath} {
		if info, statErr := os.Stat(path); statErr != nil || info.IsDir() {
			if statErr == nil {
				statErr = fmt.Errorf("成品路径不是文件")
			}
			recorder.fail("读取"+name+"资料", statErr)
			return "", "", statErr
		}
	}
	recorder.pass("制作"+name, "当前源码已制作并完成公共成品核对")
	return result.ArchivePath, result.ManifestPath, nil
}

func stage03LifecycleEnvironment(paths runPaths, archivePath string, manifestPath string) map[string]string {
	return map[string]string{
		"EUCLI_STAGE03_ARCHIVE":         archivePath,
		"EUCLI_STAGE03_MANIFEST":        manifestPath,
		"EUCLI_STAGE03_CLIENT_DATA_DIR": filepath.Join(paths.environment, "client-data"),
	}
}
