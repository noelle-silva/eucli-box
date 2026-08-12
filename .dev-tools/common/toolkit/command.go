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
	fmt.Printf("[工具] 开始：%s\n", name)
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workdir
	cmd.Env = r.environment(extra)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	evidenceDir := filepath.Join(r.Output, "evidence")
	if mkErr := os.MkdirAll(evidenceDir, 0o755); mkErr != nil {
		return mkErr
	}
	logName := strings.ToLower(strings.TrimSpace(name))
	logName = strings.ReplaceAll(logName, " ", "-")
	logPath := filepath.Join(evidenceDir, logName+".log")
	payload := append([]byte("stdout:\n"), stdout.Bytes()...)
	payload = append(payload, []byte("\nstderr:\n")...)
	payload = append(payload, stderr.Bytes()...)
	if writeErr := os.WriteFile(logPath, payload, 0o644); writeErr != nil {
		return writeErr
	}
	if err != nil {
		fmt.Printf("[工具] 失败：%s\n", name)
		return fmt.Errorf("%w：%s", err, strings.TrimSpace(stderr.String()))
	}
	fmt.Printf("[工具] 完成：%s\n", name)
	return nil
}

// environment 返回隔离环境：临时区统一指到运行格的临时格内。
func (r *Run) environment(extra map[string]string) []string {
	replacements := map[string]string{
		"TEMP":        r.Temp,
		"TMP":         r.Temp,
		"GOTMPDIR":    filepath.Join(r.Temp, "go"),
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
