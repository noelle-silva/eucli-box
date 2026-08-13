// Package boxupdateverify 是阶段七业务端更新链验证的场景编排。
package boxupdateverify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devtools/common/toolkit"

	"eucli-box/pkg/workspace"
)

const (
	toolName    = "verify-box-update"
	defaultMode = "default"
	boxName     = "boxharness"
)

// verifyPaths 是本次运行的全部落点。
type verifyPaths struct {
	root        string
	inputs      string
	workspace   string
	environment string
	work        string
	temp        string
	cache       string
	evidence    string
}

func newVerifyPaths(runRoot string) verifyPaths {
	return verifyPaths{
		root:        runRoot,
		inputs:      filepath.Join(runRoot, "inputs"),
		workspace:   filepath.Join(runRoot, "workspace"),
		environment: filepath.Join(runRoot, "environment"),
		work:        filepath.Join(runRoot, "work"),
		temp:        filepath.Join(runRoot, "temp"),
		cache:       filepath.Join(runRoot, "cache"),
		evidence:    filepath.Join(runRoot, "evidence"),
	}
}

func (p verifyPaths) disposable() []string {
	return []string{p.inputs, p.workspace, p.environment, p.work, p.temp, p.cache}
}

// Run 执行阶段七业务端更新验证：场景编排、逐项检查、证据报告。
func Run(ctx context.Context, repositoryRoot string, runRoot string, mode string) error {
	root, err := toolkit.ExistingPlainDirectory(repositoryRoot, "仓库根目录")
	if err != nil {
		return err
	}
	runRoot, err = filepath.Abs(strings.TrimSpace(runRoot))
	if err != nil || strings.TrimSpace(runRoot) == "" {
		return fmt.Errorf("验证运行目录无效")
	}
	expectedParent := filepath.Join(root, ".dev-workspace", ".dev-tools-runtime", toolName)
	if !toolkit.PathWithin(expectedParent, runRoot) || toolkit.SamePath(expectedParent, runRoot) || !strings.HasPrefix(filepath.Base(runRoot), "run-") {
		return fmt.Errorf("验证运行目录必须位于 %s 的独立 run-* 目录中", expectedParent)
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = defaultMode
	}
	if mode != defaultMode && mode != "experience" {
		return fmt.Errorf("业务端更新验证模式只接受 %s 或 experience", defaultMode)
	}
	paths := newVerifyPaths(runRoot)
	for _, dir := range []string{paths.inputs, paths.workspace, paths.environment, paths.work, paths.temp, paths.cache, paths.evidence} {
		if err := toolkit.EnsurePlainDirectoryPath(root, dir, "验证资料目录"); err != nil {
			return err
		}
	}
	recorder := toolkit.NewVerificationRecorder(toolName, mode, runRoot)
	fmt.Printf("业务端更新 %s 验证目录：%s\n", mode, runRoot)

	dataBefore, dataErr := toolkit.DirectorySnapshot(filepath.Join(root, "data"))
	if dataErr != nil {
		recorder.Fail("记录真实数据初始状态", dataErr)
	} else {
		recorder.Pass("记录真实数据初始状态", "已建立只读完整性快照")
	}
	gitBefore, gitErr := captureGitStatus(root, paths)
	if gitErr != nil {
		recorder.Fail("记录源码初始状态", gitErr)
	} else {
		recorder.Pass("记录源码初始状态", "已记录当前工作区状态")
	}

	harnessPath := filepath.Join(paths.work, boxName+".exe")
	if err := toolkit.RunCommand(ctx, "构建业务端替身", paths.work, paths.evidence, paths.temp, nil, "go", "build", "-o", harnessPath, "devtools/general-verification-tools/verify-box-update/boxupdateverify/boxharness"); err != nil {
		recorder.Fail("构建业务端替身", err)
	} else {
		recorder.Pass("构建业务端替身", "可编排业务端替身已编译")
		archivePath, manifestPath, ok := prepareRealProduct(ctx, root, paths, recorder)
		if ok {
			if mode == "experience" {
				runExperience(ctx, root, paths, recorder, harnessPath, archivePath, manifestPath)
			} else {
				runDefault(ctx, root, paths, recorder, harnessPath, archivePath, manifestPath)
			}
		}
	}

	if dataErr == nil {
		dataAfter, snapshotErr := toolkit.DirectorySnapshot(filepath.Join(root, "data"))
		if snapshotErr != nil {
			recorder.Fail("确认真实数据未改变", snapshotErr)
		} else if err := toolkit.CompareSnapshots("真实 data 目录", dataBefore, dataAfter); err != nil {
			recorder.Fail("确认真实数据未改变", err)
		} else {
			recorder.Pass("确认真实数据未改变", "业务端更新验证未写入真实 data 目录")
		}
	}
	if gitErr == nil {
		gitAfter, snapshotErr := captureGitStatus(root, paths)
		if snapshotErr != nil {
			recorder.Fail("确认源码未被验证改写", snapshotErr)
		} else if err := toolkit.CompareSnapshots("源码工作区", gitBefore, gitAfter); err != nil {
			recorder.Fail("确认源码未被验证改写", err)
		} else {
			recorder.Pass("确认源码未被验证改写", "业务端更新验证只在本次隔离目录产生运行内容")
		}
	}
	return recorder.Finish(paths.evidence, paths.disposable())
}

// prepareRealProduct 使用当前源码制作 Windows x64 业务端验证成品，返回压缩包与清单路径。
func prepareRealProduct(ctx context.Context, root string, paths verifyPaths, recorder *toolkit.VerificationRecorder) (string, string, bool) {
	resultPath := filepath.Join(paths.environment, "box-build-result.json")
	buildCommand := []string{"run", "devtools/eucli-release", "build", "-root", root, "-target", "eucli-box", "-work-root", filepath.Join(paths.workspace, "box-build"), "-output-root", filepath.Join(paths.environment, "box-release"), "-evidence-root", filepath.Join(paths.evidence, "box-build"), "-asset-root", workspace.VerificationAssetRoot(root), "-verification-only", "-result-file", resultPath}
	if err := toolkit.RunCommand(ctx, "制作业务端更新真实成品", root, paths.evidence, paths.temp, nil, "go", buildCommand...); err != nil {
		recorder.Fail("制作业务端更新真实成品", err)
		return "", "", false
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
			err = fmt.Errorf("制作结果缺少成品路径")
		}
		recorder.Fail("读取真实成品资料", err)
		return "", "", false
	}
	for _, path := range []string{result.ArchivePath, result.ManifestPath} {
		if info, statErr := os.Stat(path); statErr != nil || info.IsDir() {
			if statErr == nil {
				statErr = fmt.Errorf("成品路径不是文件")
			}
			recorder.Fail("读取真实成品资料", statErr)
			return "", "", false
		}
	}
	recorder.Pass("制作业务端更新真实成品", "当前源码已制作并完成公共成品核对")
	return result.ArchivePath, result.ManifestPath, true
}

// updateTestEnvironment 是 backend-go 带标签测试的环境变量。
func updateTestEnvironment(paths verifyPaths, harnessPath string, archivePath string, manifestPath string) map[string]string {
	return map[string]string{
		"EUCLI_BOX_UPDATE_HARNESS":         harnessPath,
		"EUCLI_BOX_UPDATE_CLIENT_DATA_DIR": filepath.Join(paths.environment, "client-data"),
		"EUCLI_BOX_UPDATE_WORK_DIR":        filepath.Join(paths.environment, "update-work"),
		"EUCLI_BOX_UPDATE_BOX_ARCHIVE":     archivePath,
		"EUCLI_BOX_UPDATE_BOX_MANIFEST":    manifestPath,
	}
}

// runDefault 执行默认模式：完整更新链场景、单元与构建、界面类型检查。
func runDefault(ctx context.Context, root string, paths verifyPaths, recorder *toolkit.VerificationRecorder, harnessPath string, archivePath string, manifestPath string) {
	backendDir := filepath.Join(root, "clients", "eucli-studio", "backend-go")
	clientDir := filepath.Join(root, "clients", "eucli-studio")
	commands := []struct {
		name    string
		workdir string
		command string
		args    []string
		env     map[string]string
	}{
		{name: "业务端更新完整场景（替身驱动）", workdir: backendDir, command: "go", args: []string{"test", "-tags", "eucli_box_update", "-run", "^TestBoxUpdateScenario", "-count=1"}, env: updateTestEnvironment(paths, harnessPath, archivePath, manifestPath)},
		{name: "数据迁移系统测试", workdir: root, command: "go", args: []string{"test", "./src/data-migration-system/", "-count=1"}},
		{name: "网关系统测试", workdir: root, command: "go", args: []string{"test", "./src/gateway-system/", "-count=1"}},
		{name: "客户端后台全部测试（含中断恢复）", workdir: backendDir, command: "go", args: []string{"test", "./...", "-count=1"}},
		{name: "客户端协议和界面类型", workdir: clientDir, command: "pnpm", args: []string{"exec", "tsc", "--noEmit"}},
		{name: "客户端界面构建", workdir: clientDir, command: "pnpm", args: []string{"build:ui"}},
	}
	for _, command := range commands {
		if err := toolkit.RunCommand(ctx, command.name, command.workdir, paths.evidence, paths.temp, command.env, command.command, command.args...); err != nil {
			recorder.Fail(command.name, err)
		} else {
			recorder.Pass(command.name, "隔离验证通过，详细输出见对应 evidence 日志")
		}
	}
}

// runExperience 执行体验模式：预装旧版替身、体验准备验证与界面构建。
func runExperience(ctx context.Context, root string, paths verifyPaths, recorder *toolkit.VerificationRecorder, harnessPath string, archivePath string, manifestPath string) {
	backendDir := filepath.Join(root, "clients", "eucli-studio", "backend-go")
	clientDir := filepath.Join(root, "clients", "eucli-studio")
	env := updateTestEnvironment(paths, harnessPath, archivePath, manifestPath)
	env["EUCLI_BOX_UPDATE_EXPERIENCE"] = "1"
	commands := []struct {
		name    string
		workdir string
		command string
		args    []string
		env     map[string]string
	}{
		{name: "体验模式预装与准备验证", workdir: backendDir, command: "go", args: []string{"test", "-tags", "eucli_box_update", "-run", "^TestBoxUpdateExperiencePrep$", "-count=1"}, env: env},
		{name: "体验模式界面构建", workdir: clientDir, command: "pnpm", args: []string{"build:ui"}},
	}
	for _, command := range commands {
		if err := toolkit.RunCommand(ctx, command.name, command.workdir, paths.evidence, paths.temp, command.env, command.command, command.args...); err != nil {
			recorder.Fail(command.name, err)
		} else {
			recorder.Pass(command.name, "体验模式基础链通过，详细输出见对应 evidence 日志")
		}
	}
	fmt.Printf("体验入口：隔离客户端数据位于 %s（已预装旧版替身并连接），打开开发客户端后点击「更新业务端」即可体验一次点击更新。\n", filepath.Join(paths.environment, "client-data"))
}

func captureGitStatus(root string, paths verifyPaths) (string, error) {
	output, err := toolkit.RunCommandCapture(context.Background(), "git-status", root, paths.evidence, paths.temp, nil, "git", "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return "", fmt.Errorf("读取源码状态失败：%w", err)
	}
	return strings.ReplaceAll(output, "\r\n", "\n"), nil
}
