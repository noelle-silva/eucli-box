package shellcommand

import (
	"context"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

// WarmupCommand 是预热专用的固定无害命令：让 Provider shell 启动、加载依赖并立即退出，
// 不读写任何用户数据；它在三种 Provider（git-bash、powershell、nushell）语法下都合法。
const WarmupCommand = "exit 0"

// WarmupProviderID 解析预热要拉起的默认 Provider：用户配置优先，运行包默认兜底。
// 这与正式执行在调用参数缺省时的解析链一致。
func WarmupProviderID(input types.ToolExecutionInput) (string, bool) {
	if value, ok := input.UserConfig["provider"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), true
	}
	return DefaultProviderFor(input.ToolBodyDirectory)
}

// Warmup 执行一次工具预热：加载运行配置、选择默认 Provider，拉起一次 shell
// 执行固定无害命令并立即退出。它让 Provider 程序及其依赖被系统真实读取一遍，
// 用于保持扫描缓存热度；不接触用户命令、不进入输出上报与用户结果链路。
func Warmup(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	config, err := loadConfig(input.ToolBodyDirectory)
	if err != nil {
		return failure("load shell_command config", err, nil)
	}
	providerID, _ := WarmupProviderID(input)
	provider, err := selectProvider(config, providerID, input.ToolBodyDirectory)
	if err != nil {
		return failure("select shell_command provider", err, map[string]any{"provider": effectiveProviderName(config, providerID)})
	}
	workdir, err := resolveWorkdir(input.HostWorkingDirectory, "")
	if err != nil {
		return failure("resolve shell_command workdir", err, map[string]any{"provider": provider.Config.ID})
	}
	request := commandRequest{Command: WarmupCommand, Provider: provider.Config.ID, MaxOutputChars: config.Limits.MaxOutputChars}
	result := runProviderCommand(ctx, provider, request, workdir, nil)
	metadata := map[string]any{
		"warmup":     true,
		"provider":   provider.Config.ID,
		"command":    WarmupCommand,
		"exitCode":   result.ExitCode,
		"durationMs": result.DurationMs,
	}
	if result.FailureKind != "" {
		metadata["failureKind"] = result.FailureKind
	}
	if result.Error != "" {
		metadata["error"] = result.Error
		return types.ToolExecutionOutput{Status: types.ToolStatusFailed, Content: result.Error, Error: result.Error, Metadata: metadata}
	}
	if result.ExitCode != 0 {
		errorMessage := fmt.Sprintf("warmup shell exited with code %d", result.ExitCode)
		metadata["error"] = errorMessage
		return types.ToolExecutionOutput{Status: types.ToolStatusFailed, Content: errorMessage, Error: errorMessage, Metadata: metadata}
	}
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: "shell_command warmup completed", Metadata: metadata}
}
