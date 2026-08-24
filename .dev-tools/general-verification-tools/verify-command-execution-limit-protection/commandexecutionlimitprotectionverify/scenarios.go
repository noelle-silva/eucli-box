package verify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"eucli-box/pkg/types"
	toolcalling "eucli-box/src/tool-calling-system"
)

// scenarioElidedOutput: 输出超过预算时保留首尾、中间省略，并上报字节与行数事实。
func scenarioElidedOutput(ctx context.Context, fixture fixture) error {
	system, err := newHostSystem(toolcalling.Config{}, &fakeToolStorage{})
	if err != nil {
		return err
	}
	result, err := executeHost(ctx, system, fixture, true, map[string]any{"command": "flood 1048576", "workdir": ".", "timeoutMs": 60000, "maxOutputChars": 4000})
	if err != nil {
		return err
	}
	if result.Status != types.ToolStatusSuccess {
		return fmt.Errorf("flood status = %#v", result)
	}
	if !metaBool(result.Metadata, "truncated") {
		return fmt.Errorf("flood should be truncated: %#v", result.Metadata)
	}
	content := result.Content
	if !strings.Contains(content, "chars truncated") && !strings.Contains(content, "bytes truncated") {
		return fmt.Errorf("elision marker missing: %q", content)
	}
	if !strings.HasPrefix(content, "aaa") {
		return fmt.Errorf("head missing: %q", content)
	}
	if total := metaFloat(result.Metadata, "outputBytesTotal"); total < 1048576 {
		return fmt.Errorf("outputBytesTotal = %v", total)
	}
	if lines := metaFloat(result.Metadata, "outputLines"); lines < 1024 {
		return fmt.Errorf("outputLines = %v", lines)
	}
	return nil
}

// scenarioUpdateFlood: 输出行数超过一万条更新上限，命令仍正常完成并返回完整结果。
func scenarioUpdateFlood(ctx context.Context, fixture fixture) error {
	system, err := newHostSystem(toolcalling.Config{
		ToolWatchdogTimeout:      2 * time.Second,
		ToolWatchdogPingInterval: 200 * time.Millisecond,
	}, &fakeToolStorage{})
	if err != nil {
		return err
	}
	result, err := executeHost(ctx, system, fixture, true, map[string]any{"command": "spam 12000", "workdir": ".", "timeoutMs": 60000})
	if err != nil {
		return err
	}
	if result.Status != types.ToolStatusSuccess {
		return fmt.Errorf("spam status = %#v", result)
	}
	if metaString(result.Metadata, "failureKind") != "" {
		return fmt.Errorf("spam wrongly failed = %#v", result.Metadata)
	}
	if !strings.Contains(result.Content, "spam-line") {
		return fmt.Errorf("spam result content incomplete: %#v", result.Metadata)
	}
	return nil
}

// scenarioTreeTermination: 命令拉起子进程后超时，进程树终止出口应同时结束子树。
func scenarioTreeTermination(ctx context.Context, fixture fixture) error {
	system, err := newHostSystem(toolcalling.Config{}, &fakeToolStorage{})
	if err != nil {
		return err
	}
	pidDir := filepath.Join(fixture.root, "pid-evidence")
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		return err
	}
	result, err := executeHost(ctx, system, fixture, true, map[string]any{"command": "spawn-sleep", "workdir": pidDir, "timeoutMs": 300})
	if err != nil {
		return err
	}
	if result.Status != types.ToolStatusFailed {
		return fmt.Errorf("spawn-sleep status = %#v", result)
	}
	if !metaBool(result.Metadata, "timedOut") || metaString(result.Metadata, "failureKind") != "command_timeout" {
		return fmt.Errorf("spawn-sleep should time out: %#v", result.Metadata)
	}
	payload, err := os.ReadFile(filepath.Join(pidDir, "child.pid"))
	if err != nil {
		return fmt.Errorf("child.pid not written: %w", err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(string(payload)))
	if err != nil || childPID <= 0 {
		return fmt.Errorf("child pid invalid: %q", payload)
	}
	// Deadline给终止窗口留出时间，再检查子进程已不存在。
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		alive, err := processAlive(childPID)
		if err != nil {
			return err
		}
		if !alive {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("child process %d survived provider tree termination", childPID)
}

// processAlive 通过系统任务列表判断进程是否存在。
func processAlive(pid int) (bool, error) {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid)).CombinedOutput()
		if err != nil {
			return false, fmt.Errorf("tasklist: %w", err)
		}
		if strings.Contains(string(out), fmt.Sprintf("%d", pid)) {
			// tasklist 可能把 PID 匹配到无关串，比较行首。
			for _, line := range strings.Split(string(out), "\r\n") {
				fields := strings.Fields(line)
				if len(fields) >= 2 && fields[1] == strconv.Itoa(pid) {
					return true, nil
				}
			}
		}
		return false, nil
	}
	err := exec.Command("kill", "-0", strconv.Itoa(pid)).Run()
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("kill -0: %w", err)
}
