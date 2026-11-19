package releaseasset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// probe 在准备好的外部随包目录上执行一次真实运行核对：
// 运行什么由随包类别决定，期望输出由配方声明，版本与运行时布局都不写进代码。
func probe(ctx context.Context, root string, recipe Recipe) error {
	if ctx == nil {
		return fmt.Errorf("运行核对上下文不能为空")
	}
	switch recipe.Kind {
	case "git-bash":
		// 固定命令只验证 shell 自身可用：路径解析、管道、版本输出。
		command := `test "$(command -v grep)" = /usr/bin/grep && test -n "$(command -v git)" && printf 'release-root\n' | grep -qx release-root && git --version`
		output, err := runProbeCommand(ctx, root, probeCommand{
			executable: filepath.Join(root, "bin", "bash.exe"),
			arguments:  []string{"--noprofile", "--norc", "-lc", command},
			home:       filepath.Join(root, ".probe-home"),
		})
		if err != nil {
			return fmt.Errorf("Git Bash 固定命令核对失败：%w：%s", err, strings.TrimSpace(output))
		}
		return expectProbeOutput(recipe, "Git Bash", output)
	case "powershell":
		output, err := runProbeCommand(ctx, root, probeCommand{
			executable: filepath.Join(root, "pwsh.exe"),
			arguments:  []string{"-NoLogo", "-NoProfile", "-Command", "$PSVersionTable.PSVersion.ToString()"},
		})
		if err != nil {
			return fmt.Errorf("PowerShell 运行核对失败：%w：%s", err, strings.TrimSpace(output))
		}
		return expectProbeOutput(recipe, "PowerShell", output)
	case "nushell":
		output, err := runProbeCommand(ctx, root, probeCommand{
			executable: filepath.Join(root, "nu.exe"),
			arguments:  []string{"--version"},
		})
		if err != nil {
			return fmt.Errorf("Nushell 运行核对失败：%w：%s", err, strings.TrimSpace(output))
		}
		return expectProbeOutput(recipe, "Nushell", output)
	case "command-analyzer":
		cmd := exec.CommandContext(ctx, filepath.Join(root, "command-analyzer.exe"))
		cmd.Dir = root
		cmd.Stdin = strings.NewReader(`{"version":1,"command":"echo ok","shell":"bash","workdir":""}`)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		hideProcessWindow(cmd)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("命令分析器运行核对失败：%w：%s", err, strings.TrimSpace(stderr.String()))
		}
		var response struct {
			Version        int    `json:"version"`
			Classification string `json:"classification"`
		}
		if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &response); err != nil {
			return fmt.Errorf("命令分析器运行核对失败：响应不是合法 JSON：%w", err)
		}
		if response.Version != 1 {
			return fmt.Errorf("命令分析器运行核对失败：协议版本为 %d", response.Version)
		}
		switch response.Classification {
		case "safe", "dangerous", "unknown":
		default:
			return fmt.Errorf("命令分析器运行核对失败：分类结果无效 %q", response.Classification)
		}
		return nil
	default:
		return fmt.Errorf("没有为外部随包类别 %s 定义运行核对", recipe.Kind)
	}
}

// probeCommand 描述一次外部随包运行核对：可执行文件、参数与需要隔离的 HOME。
type probeCommand struct {
	executable string
	arguments  []string
	home       string
}

// runProbeCommand 在随包根目录下用最小环境运行核对命令：
// 只保留系统目录与随包自己的 HOME，避免外部 shell 环境影响结果。
func runProbeCommand(ctx context.Context, root string, command probeCommand) (string, error) {
	cmd := exec.CommandContext(ctx, command.executable, command.arguments...)
	cmd.Dir = root
	replacements := map[string]string{"PATH": `C:\Windows\System32`}
	if command.home != "" {
		replacements["HOME"] = command.home
	}
	cmd.Env = replaceEnvironment(os.Environ(), replacements)
	hideProcessWindow(cmd)
	output, err := cmd.CombinedOutput()
	if command.home != "" {
		_ = os.RemoveAll(command.home)
	}
	return string(output), err
}

// expectProbeOutput 用配方声明的期望片段核对实际输出：版本升级只改配方，不改代码。
func expectProbeOutput(recipe Recipe, name string, output string) error {
	for _, expectation := range recipe.Expectations {
		if !strings.Contains(output, expectation) {
			return fmt.Errorf("%s 运行核对失败：输出缺少 %q：%s", name, expectation, strings.TrimSpace(output))
		}
	}
	return nil
}
