// Package migrationverify 是阶段六数据迁移验证的场景编排。
package migrationverify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"devtools/common/toolkit"
	"eucli-box/pkg/datapaths"
)

const (
	toolName            = "verify-data-migration"
	defaultMode         = "default"
	boxReadyMark        = "is ready"
	harnessTarget       = "1.2.0"
	boxStartupTimeout   = 120 * time.Second
	boxReadyPollPeriod  = 300 * time.Millisecond
	harnessTimeout      = 60 * time.Second
	statusOutcomeField  = "outcome"
	processFileName     = "process.json"
	backupDirectoryName = "backup"
)

var statusVocabulary = []string{"data-unchanged", "migrated", "recovered", "recovery-failed"}

// Run 执行阶段六数据迁移验证：场景编排、逐项检查、证据报告。
func Run(ctx context.Context, repositoryRoot string, runRoot string, mode string) error {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = defaultMode
	}
	if mode != defaultMode {
		return fmt.Errorf("数据迁移验证模式只接受 %s", defaultMode)
	}
	run, err := toolkit.PrepareVerificationRun(repositoryRoot, runRoot, toolName)
	if err != nil {
		return err
	}
	root := run.RepositoryRoot
	recorder := toolkit.NewVerificationRecorder(toolName, mode, run.Root)
	fmt.Printf("数据迁移验证目录：%s\n", run.Root)

	dataBefore, dataErr := toolkit.DirectorySnapshot(filepath.Join(root, "data"))
	if dataErr != nil {
		recorder.Fail("记录真实数据初始状态", dataErr)
	} else {
		recorder.Pass("记录真实数据初始状态", "已建立只读完整性快照")
	}
	gitBefore, gitErr := captureGitStatus(root, run)
	if gitErr != nil {
		recorder.Fail("记录源码初始状态", gitErr)
	} else {
		recorder.Pass("记录源码初始状态", "已记录当前工作区状态")
	}

	targetVersion, targetErr := readTargetDataVersion(root)
	if targetErr != nil {
		recorder.Fail("读取业务端目标数据版本", targetErr)
	} else {
		recorder.Pass("读取业务端目标数据版本", "目标数据版本 "+targetVersion)
	}

	boxPath := filepath.Join(run.Work, "eucli-box.exe")
	harnessPath := filepath.Join(run.Work, "migrationharness.exe")
	buildErr := buildBinaries(ctx, root, run, boxPath, harnessPath)
	if buildErr != nil {
		recorder.Fail("构建准备", buildErr)
	} else {
		recorder.Pass("构建准备", "业务端与迁移替身程序已编译")
		runScenarios(ctx, root, run, recorder, boxPath, harnessPath, targetVersion)
	}

	if dataErr == nil {
		dataAfter, snapshotErr := toolkit.DirectorySnapshot(filepath.Join(root, "data"))
		if snapshotErr != nil {
			recorder.Fail("确认真实数据未改变", snapshotErr)
		} else if err := toolkit.CompareSnapshots("真实 data 目录", dataBefore, dataAfter); err != nil {
			recorder.Fail("确认真实数据未改变", err)
		} else {
			recorder.Pass("确认真实数据未改变", "数据迁移验证未写入真实 data 目录")
		}
	}
	if gitErr == nil {
		gitAfter, snapshotErr := captureGitStatus(root, run)
		if snapshotErr != nil {
			recorder.Fail("确认源码未被验证改写", snapshotErr)
		} else if err := toolkit.CompareSnapshots("源码工作区", gitBefore, gitAfter); err != nil {
			recorder.Fail("确认源码未被验证改写", err)
		} else {
			recorder.Pass("确认源码未被验证改写", "数据迁移验证只在本次隔离目录产生运行内容")
		}
	}
	return recorder.Finish(run.Evidence, run.DisposableDirectories())
}

// runScenarios 依次执行全部迁移场景；每个场景是逐项检查中的一项。
func runScenarios(ctx context.Context, root string, run *toolkit.VerificationRun, recorder *toolkit.VerificationRecorder, boxPath string, harnessPath string, targetVersion string) {
	scenarios := []struct {
		name string
		run  func() error
	}{
		{"首装", func() error { return scenarioInitialInstall(ctx, run, boxPath, targetVersion) }},
		{"无迁移", func() error { return scenarioNoMigration(ctx, run, boxPath, targetVersion) }},
		{"连续迁移", func() error { return scenarioContinuousMigration(ctx, run, harnessPath) }},
		{"缺少步骤", func() error { return scenarioMissingStep(ctx, run, harnessPath) }},
		{"步骤执行失败", func() error { return scenarioStepFailure(ctx, run, harnessPath) }},
		{"步骤核对失败", func() error { return scenarioVerifyFailure(ctx, run, harnessPath) }},
		{"中断恢复", func() error { return scenarioCrashRecovery(ctx, run, harnessPath) }},
		{"恢复失败", func() error { return scenarioRecoveryFailure(ctx, run, harnessPath) }},
		{"版本过高", func() error { return scenarioVersionTooHigh(ctx, run, boxPath, targetVersion) }},
		{"边界检查", func() error { return scenarioBoundaryChecks(run) }},
	}
	for _, scenario := range scenarios {
		if err := scenario.run(); err != nil {
			recorder.Fail(scenario.name, err)
		} else {
			recorder.Pass(scenario.name, "场景断言全部成立")
		}
	}
}

// ---------- 构建与公共运行 ----------

func buildBinaries(ctx context.Context, root string, run *toolkit.VerificationRun, boxPath string, harnessPath string) error {
	if err := toolkit.RunCommand(ctx, "构建隔离业务端", run.Work, run.Evidence, run.Temp, nil, "go", "build", "-o", boxPath, "eucli-box/cmd/eucli-box"); err != nil {
		return err
	}
	if err := toolkit.RunCommand(ctx, "构建迁移替身程序", run.Work, run.Evidence, run.Temp, nil, "go", "build", "-o", harnessPath, "devtools/general-verification-tools/verify-data-migration/migrationverify/migrationharness"); err != nil {
		return err
	}
	if info, err := os.Stat(boxPath); err != nil || info.IsDir() {
		return fmt.Errorf("业务端可执行文件无效")
	}
	if info, err := os.Stat(harnessPath); err != nil || info.IsDir() {
		return fmt.Errorf("迁移替身程序可执行文件无效")
	}
	return nil
}

func captureGitStatus(root string, run *toolkit.VerificationRun) (string, error) {
	output, err := toolkit.RunCommandCapture(context.Background(), "git-status", root, run.Evidence, run.Temp, nil, "git", "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return "", fmt.Errorf("读取源码状态失败：%w", err)
	}
	return strings.ReplaceAll(output, "\r\n", "\n"), nil
}

func readTargetDataVersion(root string) (string, error) {
	payload, err := os.ReadFile(filepath.Join(root, "internal", "boxrelease", "release.json"))
	if err != nil {
		return "", fmt.Errorf("读取业务端发布资料失败：%w", err)
	}
	var release struct {
		DataVersion string `json:"dataVersion"`
	}
	if err := json.Unmarshal(payload, &release); err != nil {
		return "", fmt.Errorf("解析业务端发布资料失败：%w", err)
	}
	if strings.TrimSpace(release.DataVersion) == "" {
		return "", fmt.Errorf("业务端发布资料缺少 dataVersion")
	}
	return release.DataVersion, nil
}

// ---------- 真实业务端运行 ----------

type boxProcess struct {
	cmd      *exec.Cmd
	logPath  string
	exitCh   chan error
	exited   bool
	exitErr  error
}

func startBox(ctx context.Context, run *toolkit.VerificationRun, boxPath string, dataDir string, logName string) (*boxProcess, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("建立隔离数据目录失败：%w", err)
	}
	if err := writeServiceProfile(dataDir); err != nil {
		return nil, fmt.Errorf("写入启动配置画像失败：%w", err)
	}
	logPath := filepath.Join(run.Evidence, logName)
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("建立业务端日志失败：%w", err)
	}
	command := exec.Command(boxPath)
	command.Dir = run.Environment
	command.Env = toolkit.CommandEnvironment(run.Temp, map[string]string{
		"EUCLI_BOX_DATA_DIR": dataDir,
	})
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("启动业务端失败：%w", err)
	}
	process := &boxProcess{cmd: command, logPath: logPath, exitCh: make(chan error, 1)}
	go func() {
		process.exitCh <- command.Wait()
		_ = logFile.Close()
	}()
	return process, nil
}

// writeServiceProfile 预置启动配置画像，模拟平台侧按固定读法写入端口与钥匙。
func writeServiceProfile(dataDir string) error {
	port, err := freePort()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"port": port, "key": "data-migration-verify-key"})
	if err != nil {
		return err
	}
	return os.WriteFile(datapaths.ServiceProfileFile(dataDir), append(payload, '\n'), 0o600)
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port, nil
}

func (b *boxProcess) waitReady(ctx context.Context) error {
	ticker := time.NewTicker(boxReadyPollPeriod)
	defer ticker.Stop()
	timeout := time.NewTimer(boxStartupTimeout)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return fmt.Errorf("等待业务端 ready 超时")
		case exitErr := <-b.exitCh:
			b.exited = true
			b.exitErr = exitErr
			return fmt.Errorf("业务端在 ready 前退出：%v", exitErr)
		case <-ticker.C:
			payload, err := os.ReadFile(b.logPath)
			if err == nil && containsReadyMark(string(payload)) {
				return nil
			}
		}
	}
}

// containsReadyMark 判断业务端日志是否已经进入 ready。
func containsReadyMark(content string) bool {
	return strings.Contains(content, boxReadyMark)
}

func (b *boxProcess) waitExit() error {
	select {
	case exitErr := <-b.exitCh:
		b.exited = true
		b.exitErr = exitErr
		return nil
	}
}

func (b *boxProcess) stop() {
	if b.exited {
		return
	}
	if b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
	}
	b.exitErr = <-b.exitCh
	b.exited = true
}

func (b *boxProcess) logContent() string {
	payload, err := os.ReadFile(b.logPath)
	if err != nil {
		return ""
	}
	return string(payload)
}

func (b *boxProcess) exitCode() int {
	if b.exitErr == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(b.exitErr, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// ---------- 迁移替身运行 ----------

type harnessResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func runHarness(ctx context.Context, run *toolkit.VerificationRun, harnessPath string, dataDir string, logName string, extraArgs ...string) (harnessResult, error) {
	args := append([]string{"-data-dir", dataDir, "-target", harnessTarget}, extraArgs...)
	command := exec.Command(harnessPath, args...)
	command.Dir = run.Work
	command.Env = toolkit.CommandEnvironment(run.Temp, nil)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	result := harnessResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: 0}
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.exitCode = exitErr.ExitCode()
		} else {
			return result, fmt.Errorf("运行迁移替身失败：%w", runErr)
		}
	}
	if err := os.MkdirAll(run.Evidence, 0o755); err != nil {
		return result, err
	}
	payload := append([]byte("stdout:\n"), stdout.Bytes()...)
	payload = append(payload, []byte("\nstderr:\n")...)
	payload = append(payload, stderr.Bytes()...)
	if err := os.WriteFile(filepath.Join(run.Evidence, logName), payload, 0o644); err != nil {
		return result, err
	}
	return result, nil
}

// ---------- 场景 ----------

func scenarioInitialInstall(ctx context.Context, run *toolkit.VerificationRun, boxPath string, targetVersion string) error {
	dataDir := filepath.Join(run.Environment, "case-initial", "data")
	process, err := startBox(ctx, run, boxPath, dataDir, "case-initial-box.log")
	if err != nil {
		return err
	}
	defer process.stop()
	if err := process.waitReady(ctx); err != nil {
		return err
	}
	process.stop()
	version, err := readDataVersion(dataDir)
	if err != nil {
		return err
	}
	if version != targetVersion {
		return fmt.Errorf("首装后数据版本 = %s，期望 %s", version, targetVersion)
	}
	status, err := readMigrationStatus(dataDir)
	if err != nil {
		return err
	}
	if status.Outcome != "data-unchanged" || !status.Completed {
		return fmt.Errorf("首装状态记录 = %#v", status)
	}
	return nil
}

func scenarioNoMigration(ctx context.Context, run *toolkit.VerificationRun, boxPath string, targetVersion string) error {
	dataDir := filepath.Join(run.Environment, "case-no-migration", "data")
	metaDir := datapaths.MetaDir(dataDir)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	versionPayload := fmt.Sprintf("{\n  \"version\": %q,\n  \"createdAt\": %q,\n  \"updatedAt\": %q\n}\n", targetVersion, now, now)
	if err := os.WriteFile(datapaths.VersionFile(dataDir), []byte(versionPayload), 0o644); err != nil {
		return err
	}
	sample := filepath.Join(dataDir, "sessions", "keep.json")
	if err := os.MkdirAll(filepath.Dir(sample), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(sample, []byte(`{"keep":true}`+"\n"), 0o644); err != nil {
		return err
	}
	versionBefore, err := os.ReadFile(datapaths.VersionFile(dataDir))
	if err != nil {
		return err
	}
	process, err := startBox(ctx, run, boxPath, dataDir, "case-no-migration-box.log")
	if err != nil {
		return err
	}
	defer process.stop()
	if err := process.waitReady(ctx); err != nil {
		return err
	}
	process.stop()
	versionAfter, err := os.ReadFile(datapaths.VersionFile(dataDir))
	if err != nil {
		return err
	}
	if !bytes.Equal(versionBefore, versionAfter) {
		return fmt.Errorf("无迁移场景中 version.json 被改写")
	}
	sampleAfter, err := os.ReadFile(sample)
	if err != nil {
		return err
	}
	if string(sampleAfter) != `{"keep":true}`+"\n" {
		return fmt.Errorf("无迁移场景中样例文件被改写")
	}
	status, err := readMigrationStatus(dataDir)
	if err != nil {
		return err
	}
	if status.Outcome != "data-unchanged" || !status.Completed {
		return fmt.Errorf("无迁移状态记录 = %#v", status)
	}
	return nil
}

func scenarioContinuousMigration(ctx context.Context, run *toolkit.VerificationRun, harnessPath string) error {
	dataDir := filepath.Join(run.Environment, "case-migrate", "data")
	if err := seedHarnessData(dataDir, "1.0.0"); err != nil {
		return err
	}
	result, err := runHarness(ctx, run, harnessPath, dataDir, "case-migrate-harness.log", "-chain", "ok")
	if err != nil {
		return err
	}
	if result.exitCode != 0 {
		return fmt.Errorf("连续迁移退出码 = %d：%s", result.exitCode, result.stderr)
	}
	if !strings.Contains(result.stdout, `"state":"migrated"`) {
		return fmt.Errorf("连续迁移结果 = %q", result.stdout)
	}
	version, err := readDataVersion(dataDir)
	if err != nil {
		return err
	}
	if version != harnessTarget {
		return fmt.Errorf("连续迁移后数据版本 = %s，期望 %s", version, harnessTarget)
	}
	count, err := readCounter(dataDir)
	if err != nil || count != 2 {
		return fmt.Errorf("连续迁移后 counter = %d err=%v", count, err)
	}
	stamp, err := readStamp(dataDir)
	if err != nil || stamp != "1.2.0" {
		return fmt.Errorf("连续迁移后 stamp = %q err=%v", stamp, err)
	}
	status, err := readMigrationStatus(dataDir)
	if err != nil {
		return err
	}
	if status.Outcome != "migrated" || !status.Completed || status.FromVersion != "1.0.0" || status.TargetVersion != harnessTarget {
		return fmt.Errorf("连续迁移状态记录 = %#v", status)
	}
	workspaceDir := filepath.Join(filepath.Dir(dataDir), "data.migration")
	if _, err := os.Stat(filepath.Join(workspaceDir, processFileName)); !os.IsNotExist(err) {
		return fmt.Errorf("连续迁移后 process.json 未清理")
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, backupDirectoryName)); !os.IsNotExist(err) {
		return fmt.Errorf("连续迁移后 backup 未清理")
	}
	return nil
}

func scenarioMissingStep(ctx context.Context, run *toolkit.VerificationRun, harnessPath string) error {
	dataDir := filepath.Join(run.Environment, "case-gap", "data")
	if err := seedHarnessData(dataDir, "1.0.0"); err != nil {
		return err
	}
	before, err := toolkit.DirectorySnapshot(dataDir)
	if err != nil {
		return err
	}
	result, err := runHarness(ctx, run, harnessPath, dataDir, "case-gap-harness.log", "-chain", "gap")
	if err != nil {
		return err
	}
	if result.exitCode == 0 {
		return fmt.Errorf("缺少步骤场景退出码 = 0，期望非零")
	}
	if !strings.Contains(result.stderr, "migration.step_missing") {
		return fmt.Errorf("缺少步骤场景未报告 step_missing：%s", result.stderr)
	}
	after, err := toolkit.DirectorySnapshot(dataDir)
	if err != nil {
		return err
	}
	if err := toolkit.CompareSnapshots("缺少步骤场景数据目录", before, after); err != nil {
		return err
	}
	return nil
}

func scenarioStepFailure(ctx context.Context, run *toolkit.VerificationRun, harnessPath string) error {
	return runRecoveredScenario(ctx, run, harnessPath, "case-step-failure", "case-step-failure-harness.log", "-chain", "ok", "-fail-at", "2")
}

func scenarioVerifyFailure(ctx context.Context, run *toolkit.VerificationRun, harnessPath string) error {
	return runRecoveredScenario(ctx, run, harnessPath, "case-verify-failure", "case-verify-failure-harness.log", "-chain", "ok", "-verify-fail-at", "2")
}

func runRecoveredScenario(ctx context.Context, run *toolkit.VerificationRun, harnessPath string, caseName string, logName string, args ...string) error {
	dataDir := filepath.Join(run.Environment, caseName, "data")
	if err := seedHarnessData(dataDir, "1.0.0"); err != nil {
		return err
	}
	before, err := toolkit.DirectorySnapshot(dataDir)
	if err != nil {
		return err
	}
	result, err := runHarness(ctx, run, harnessPath, dataDir, logName, args...)
	if err != nil {
		return err
	}
	if result.exitCode != 0 {
		return fmt.Errorf("替身退出码 = %d：%s", result.exitCode, result.stderr)
	}
	if !strings.Contains(result.stdout, `"state":"recovered"`) {
		return fmt.Errorf("替身结果 = %q", result.stdout)
	}
	version, err := readDataVersion(dataDir)
	if err != nil {
		return err
	}
	if version != "1.0.0" {
		return fmt.Errorf("恢复后数据版本 = %s，期望 1.0.0", version)
	}
	after, err := toolkit.DirectorySnapshot(dataDir)
	if err != nil {
		return err
	}
	if err := toolkit.CompareSnapshots("恢复后数据目录", before, after); err != nil {
		return err
	}
	status, err := readMigrationStatus(dataDir)
	if err != nil {
		return err
	}
	if status.Outcome != "recovered" || !status.Completed {
		return fmt.Errorf("恢复状态记录 = %#v", status)
	}
	return nil
}

func scenarioCrashRecovery(ctx context.Context, run *toolkit.VerificationRun, harnessPath string) error {
	dataDir := filepath.Join(run.Environment, "case-crash", "data")
	if err := seedHarnessData(dataDir, "1.0.0"); err != nil {
		return err
	}
	before, err := toolkit.DirectorySnapshot(dataDir)
	if err != nil {
		return err
	}
	crashResult, err := runHarness(ctx, run, harnessPath, dataDir, "case-crash-harness.log", "-chain", "ok", "-crash-at", "2")
	if err != nil {
		return err
	}
	if crashResult.exitCode != 17 {
		return fmt.Errorf("中断替身退出码 = %d，期望 17", crashResult.exitCode)
	}
	workspaceDir := filepath.Join(filepath.Dir(dataDir), "data.migration")
	if _, err := os.Stat(filepath.Join(workspaceDir, processFileName)); err != nil {
		return fmt.Errorf("中断后 process.json 缺失：%v", err)
	}
	rerunResult, err := runHarness(ctx, run, harnessPath, dataDir, "case-crash-rerun-harness.log", "-chain", "ok")
	if err != nil {
		return err
	}
	if rerunResult.exitCode != 0 {
		return fmt.Errorf("中断后重跑退出码 = %d：%s", rerunResult.exitCode, rerunResult.stderr)
	}
	if !strings.Contains(rerunResult.stdout, `"state":"recovered"`) {
		return fmt.Errorf("中断恢复结果 = %q", rerunResult.stdout)
	}
	version, err := readDataVersion(dataDir)
	if err != nil {
		return err
	}
	if version != "1.0.0" {
		return fmt.Errorf("中断恢复后数据版本 = %s，期望 1.0.0", version)
	}
	after, err := toolkit.DirectorySnapshot(dataDir)
	if err != nil {
		return err
	}
	if err := toolkit.CompareSnapshots("中断恢复后数据目录", before, after); err != nil {
		return err
	}
	status, err := readMigrationStatus(dataDir)
	if err != nil {
		return err
	}
	if status.Outcome != "recovered" {
		return fmt.Errorf("中断恢复状态记录 = %#v", status)
	}
	return nil
}

func scenarioRecoveryFailure(ctx context.Context, run *toolkit.VerificationRun, harnessPath string) error {
	dataDir := filepath.Join(run.Environment, "case-recovery-failure", "data")
	if err := seedHarnessData(dataDir, "1.0.0"); err != nil {
		return err
	}
	result, err := runHarness(ctx, run, harnessPath, dataDir, "case-recovery-failure-harness.log", "-chain", "ok", "-fail-at", "2", "-corrupt-backup")
	if err != nil {
		return err
	}
	if result.exitCode == 0 {
		return fmt.Errorf("恢复失败场景退出码 = 0，期望非零")
	}
	if !strings.Contains(result.stderr, "migration.recovery_failed") {
		return fmt.Errorf("恢复失败场景未报告 recovery_failed：%s", result.stderr)
	}
	status, err := readMigrationStatus(dataDir)
	if err != nil {
		return err
	}
	if status.Outcome != "recovery-failed" {
		return fmt.Errorf("恢复失败状态记录 = %#v", status)
	}
	workspaceDir := filepath.Join(filepath.Dir(dataDir), "data.migration")
	if _, err := os.Stat(filepath.Join(workspaceDir, processFileName)); err != nil {
		return fmt.Errorf("恢复失败后 process.json 未保留：%v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, backupDirectoryName)); err != nil {
		return fmt.Errorf("恢复失败后备份目录未保留：%v", err)
	}
	version, err := readDataVersion(dataDir)
	if err != nil {
		return err
	}
	if version != "1.1.0" {
		return fmt.Errorf("恢复失败现场数据版本 = %s，期望保持第一步后的 1.1.0", version)
	}
	return nil
}

func scenarioVersionTooHigh(ctx context.Context, run *toolkit.VerificationRun, boxPath string, targetVersion string) error {
	dataDir := filepath.Join(run.Environment, "case-too-high", "data")
	metaDir := datapaths.MetaDir(dataDir)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	versionPayload := fmt.Sprintf("{\n  \"version\": \"2.0.0\",\n  \"createdAt\": %q,\n  \"updatedAt\": %q\n}\n", now, now)
	if err := os.WriteFile(datapaths.VersionFile(dataDir), []byte(versionPayload), 0o644); err != nil {
		return err
	}
	process, err := startBox(ctx, run, boxPath, dataDir, "case-too-high-box.log")
	if err != nil {
		return err
	}
	defer process.stop()
	if err := process.waitExit(); err != nil {
		return err
	}
	if process.exitCode() == 0 {
		return fmt.Errorf("版本过高场景业务端正常退出，期望非零退出")
	}
	if !strings.Contains(process.logContent(), "数据迁移准备失败") {
		return fmt.Errorf("版本过高场景日志缺少明确拒绝信息")
	}
	if strings.Contains(process.logContent(), boxReadyMark) {
		return fmt.Errorf("版本过高场景业务端进入了 ready")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dataDir), "data.migration", "status.json")); !os.IsNotExist(err) {
		return fmt.Errorf("版本过高场景不应写状态记录")
	}
	return nil
}

func scenarioBoundaryChecks(run *toolkit.VerificationRun) error {
	for _, caseName := range []string{"case-migrate", "case-step-failure", "case-verify-failure", "case-crash", "case-recovery-failure"} {
		dataDir := filepath.Join(run.Environment, caseName, "data")
		workspaceDir := filepath.Join(filepath.Dir(dataDir), "data.migration")
		if filepath.Dir(workspaceDir) != filepath.Dir(dataDir) || filepath.Base(workspaceDir) != "data.migration" {
			return fmt.Errorf("替身工作区不是数据目录兄弟目录：%s", workspaceDir)
		}
		if toolkit.PathWithin(dataDir, workspaceDir) {
			return fmt.Errorf("替身工作区进入了活动数据目录内部：%s", workspaceDir)
		}
		if !toolkit.PathWithin(run.Environment, workspaceDir) {
			return fmt.Errorf("替身工作区越过验证运行目录：%s", workspaceDir)
		}
		status, err := readMigrationStatus(dataDir)
		if err != nil {
			return err
		}
		if !containsString(statusVocabulary, status.Outcome) {
			return fmt.Errorf("状态记录 outcome = %q 不在四态词表中", status.Outcome)
		}
	}
	return nil
}

// ---------- 数据断言辅助 ----------

type migrationStatusRecord struct {
	Outcome            string `json:"outcome"`
	FromVersion        string `json:"fromVersion"`
	TargetVersion      string `json:"targetVersion"`
	CurrentDataVersion string `json:"currentDataVersion"`
	Completed          bool   `json:"completed"`
}

func readMigrationStatus(dataDir string) (migrationStatusRecord, error) {
	statusPath := filepath.Join(filepath.Dir(dataDir), "data.migration", "status.json")
	payload, err := os.ReadFile(statusPath)
	if err != nil {
		return migrationStatusRecord{}, fmt.Errorf("读取状态记录失败：%w", err)
	}
	var record migrationStatusRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return migrationStatusRecord{}, fmt.Errorf("解析状态记录失败：%w", err)
	}
	return record, nil
}

func readDataVersion(dataDir string) (string, error) {
	payload, err := os.ReadFile(datapaths.VersionFile(dataDir))
	if err != nil {
		return "", fmt.Errorf("读取数据版本失败：%w", err)
	}
	var version struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(payload, &version); err != nil {
		return "", fmt.Errorf("解析数据版本失败：%w", err)
	}
	return version.Version, nil
}

func seedHarnessData(dataDir string, version string) error {
	metaDir := datapaths.MetaDir(dataDir)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	versionPayload := fmt.Sprintf("{\n  \"version\": %q,\n  \"createdAt\": %q,\n  \"updatedAt\": %q\n}\n", version, now, now)
	if err := os.WriteFile(datapaths.VersionFile(dataDir), []byte(versionPayload), 0o644); err != nil {
		return err
	}
	counterPayload := "{\n  \"count\": 0\n}\n"
	if err := os.WriteFile(filepath.Join(metaDir, "counter.json"), []byte(counterPayload), 0o644); err != nil {
		return err
	}
	return nil
}

func readCounter(dataDir string) (int, error) {
	payload, err := os.ReadFile(filepath.Join(datapaths.MetaDir(dataDir), "counter.json"))
	if err != nil {
		return 0, err
	}
	var value struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return 0, err
	}
	return value.Count, nil
}

func readStamp(dataDir string) (string, error) {
	payload, err := os.ReadFile(filepath.Join(datapaths.MetaDir(dataDir), "stamp.json"))
	if err != nil {
		return "", err
	}
	var value struct {
		Stamp string `json:"stamp"`
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return "", err
	}
	return value.Stamp, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
