package toolkit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunCommand 在显式工作目录执行命令，输出写入产物格的证据日志。
func (r *Run) RunCommand(ctx context.Context, name string, workdir string, command string, args ...string) error {
	return r.RunCommandWithEnvironment(ctx, name, workdir, command, nil, args...)
}

// RunCommandWithEnvironment 执行命令并合并额外环境变量。
func (r *Run) RunCommandWithEnvironment(ctx context.Context, name string, workdir string, command string, extra map[string]string, args ...string) error {
	evidenceDir := filepath.Join(r.Output, "evidence")
	return RunCommand(ctx, name, workdir, evidenceDir, r.Temp, extra, command, args...)
}

// CommandEnvironment 返回隔离环境变量列表，供需要控制进程生命周期的验证工具使用。
func CommandEnvironment(tempDir string, extra map[string]string) []string {
	return isolatedEnvironment(tempDir, extra)
}

// RunCommand 在显式工作目录执行命令，标准输出与标准错误写入 evidenceDir 下的证据日志。
// 临时环境隔离指向 tempDir；额外环境变量通过 extraEnv 合并。
func RunCommand(ctx context.Context, name string, workdir string, evidenceDir string, tempDir string, extraEnv map[string]string, command string, args ...string) error {
	_, err := RunCommandCapture(ctx, name, workdir, evidenceDir, tempDir, extraEnv, command, args...)
	return err
}

// RunCommandCapture 执行命令、写证据日志并返回标准输出。
func RunCommandCapture(ctx context.Context, name string, workdir string, evidenceDir string, tempDir string, extraEnv map[string]string, command string, args ...string) (string, error) {
	fmt.Printf("[工具] 开始：%s\n", name)
	if err := os.MkdirAll(filepath.Join(tempDir, "go"), 0o755); err != nil {
		return "", fmt.Errorf("建立临时 Go 工作目录失败：%w", err)
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workdir
	cmd.Env = isolatedEnvironment(tempDir, extraEnv)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if mkErr := os.MkdirAll(evidenceDir, 0o755); mkErr != nil {
		return stdout.String(), mkErr
	}
	logName := strings.ToLower(strings.TrimSpace(name))
	logName = strings.ReplaceAll(logName, " ", "-")
	logPath := filepath.Join(evidenceDir, logName+".log")
	payload := append([]byte("stdout:\n"), stdout.Bytes()...)
	payload = append(payload, []byte("\nstderr:\n")...)
	payload = append(payload, stderr.Bytes()...)
	if writeErr := os.WriteFile(logPath, payload, 0o644); writeErr != nil {
		return stdout.String(), writeErr
	}
	if err != nil {
		fmt.Printf("[工具] 失败：%s\n", name)
		return stdout.String(), fmt.Errorf("%w：%s", err, strings.TrimSpace(stderr.String()))
	}
	fmt.Printf("[工具] 完成：%s\n", name)
	return stdout.String(), nil
}

// isolatedEnvironment 返回隔离环境：临时区统一指到 tempDir 内。
func isolatedEnvironment(tempDir string, extra map[string]string) []string {
	replacements := map[string]string{
		"TEMP":        tempDir,
		"TMP":         tempDir,
		"GOTMPDIR":    filepath.Join(tempDir, "go"),
		"GOTELEMETRY": "off",
	}
	for key, value := range extra {
		replacements[key] = value
	}
	base := os.Environ()
	result := make([]string, 0, len(base)+len(replacements))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, replaced := replacements[strings.ToUpper(key)]; replaced {
				continue
			}
		}
		result = append(result, item)
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}
