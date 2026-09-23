package toolcalling

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

// toolWarmupEnabled 读取工具定义里的预热开关：用户配置优先、默认配置兜底；
// 未声明或值不是布尔的工具视为不参与预热，宿主永不向它发送预热请求。
func toolWarmupEnabled(tool types.ToolDefinition) bool {
	if value, ok := tool.UserConfig["warmup"].(bool); ok {
		return value
	}
	if value, ok := tool.DefaultConfig["warmup"].(bool); ok {
		return value
	}
	return false
}

// warmupTool 对单个工具执行一次预热：按正式执行同样的进程交接方式启动工具，
// 但发送的是预热请求，由工具自己完成热身动作；返回本次预热的真实耗时。
// 它不经过权限、意图与用户调用记录，只服务宿主自己的缓存保热。
func (s *system) warmupTool(ctx context.Context, toolID string) (time.Duration, error) {
	tool, err := s.LoadTool(ctx, toolID)
	if err != nil {
		return 0, err
	}
	if tool.Status == types.ToolAvailabilityUnavailable {
		return 0, toolExecutionInvalid("tool is unavailable for warmup: "+tool.StatusMessage, nil)
	}
	if !toolWarmupEnabled(tool) {
		return 0, toolExecutionInvalid("tool does not declare warmup", nil)
	}
	if err := ensureToolBodyDirectory(tool); err != nil {
		return 0, err
	}
	executable, err := selectExecutable(tool)
	if err != nil {
		return 0, err
	}
	activity := s.activityFor(tool.ID)
	if blocked := activity.acquire(); blocked != "" {
		return 0, toolExecutionInvalid(blocked, nil)
	}
	defer activity.release()
	hostWorkingDirectory, err := os.Getwd()
	if err != nil {
		return 0, toolExecutionInvalid("failed to resolve host working directory", err)
	}
	input, err := json.Marshal(types.ToolExecutionInput{
		ActionID:             "warmup",
		ToolName:             tool.ID,
		Arguments:            map[string]any{},
		UserConfig:           tool.UserConfig,
		DefaultConfig:        tool.DefaultConfig,
		ToolBodyDirectory:    tool.BodyDirectory,
		ToolDataDirectory:    tool.DataDirectory,
		HostWorkingDirectory: hostWorkingDirectory,
		RequestKind:          types.ToolRequestKindWarmup,
	})
	if err != nil {
		return 0, toolExecutionInvalid("failed to encode warmup input", err)
	}
	startedAt := time.Now()
	outcome := s.executeToolProcess(ctx, tool.ID, executable, tool.BodyDirectory, input, nil)
	duration := time.Since(startedAt)
	if err := toolWarmupOutcomeError(outcome); err != nil {
		return duration, err
	}
	return duration, nil
}

// toolWarmupOutcomeError 把一次预热进程的终局翻译为错误；只有工具回报成功
// 才算预热完成。真实原因优先于协议归类，与正式执行的翻译口径一致。
func toolWarmupOutcomeError(outcome toolProcessOutcome) error {
	if outcome.FailureKind != "" {
		message := "tool warmup failed: " + outcome.FailureKind
		if outcome.FailureError != nil {
			message += ": " + outcome.FailureError.Error()
		}
		return toolExecutionInvalid(message, outcome.FailureError)
	}
	if outcome.ExitError != nil {
		message := outcome.ExitError.Error()
		if stderr := strings.TrimSpace(string(outcome.Stderr)); stderr != "" {
			message += ": " + stderr
		}
		return toolExecutionInvalid("tool warmup failed: "+message, outcome.ExitError)
	}
	if outcome.FailureError != nil {
		return toolExecutionInvalid("tool warmup failed: "+outcome.FailureError.Error(), outcome.FailureError)
	}
	var output types.ToolExecutionOutput
	if err := json.Unmarshal(bytes.TrimSpace(outcome.Stdout), &output); err != nil {
		return toolExecutionInvalid("tool warmup output is not valid json", err)
	}
	if output.Status != types.ToolStatusSuccess {
		message := strings.TrimSpace(output.Error)
		if message == "" {
			message = strings.TrimSpace(output.Content)
		}
		if message == "" {
			message = "status " + string(output.Status)
		}
		return toolExecutionInvalid("tool warmup failed: "+message, nil)
	}
	return nil
}
