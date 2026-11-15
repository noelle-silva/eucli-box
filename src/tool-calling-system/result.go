package toolcalling

import (
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func newToolResultID() string {
	return utils.NewID("tool-result")
}

// toolResultWithProduced 是工具终局结果的唯一构造入口：
// 正文永远由「已经产出的内容」加「一段宿主收尾标记」组成，标记只追加在后。
// 任何终局——取消、失败、拒绝、协议异常——都不允许把工具已经跑出来的输出抹掉。
func toolResultWithProduced(plan types.ToolRunPlan, status types.ToolStatus, produced string, detail string, message string, metadata map[string]any) types.ToolResult {
	return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: status, Content: types.AppendToolExecutionMarker(produced, string(status), detail), Error: message, Metadata: metadata, CreatedAt: time.Now().UTC()}
}

func failedResult(plan types.ToolRunPlan, message string, produced string) types.ToolResult {
	return toolResultWithProduced(plan, types.ToolStatusFailed, produced, types.ToolExecutionNoticeFailed(message), message, nil)
}

func toolFailureResult(plan types.ToolRunPlan, message string, produced string, kind string, terminationError error) types.ToolResult {
	metadata := map[string]any{"failureKind": kind}
	if terminationError != nil {
		metadata["terminationError"] = terminationError.Error()
	}
	return toolResultWithProduced(plan, types.ToolStatusFailed, produced, types.ToolExecutionNoticeFailed(message), message, metadata)
}

// toolCancelledResult 落定一次用户取消：已产出的内容原样留在正文，标记说明停止前的情况。
// message 只作为内部原因留在 Error，界面展示走 Content。
func toolCancelledResult(plan types.ToolRunPlan, message string, produced string, terminationError error) types.ToolResult {
	metadata := map[string]any{"failureKind": "user_cancelled"}
	if terminationError != nil {
		metadata["terminationError"] = terminationError.Error()
	}
	notice := types.ToolExecutionNoticeCancelledWithoutOutput
	if strings.TrimSpace(produced) != "" {
		notice = types.ToolExecutionNoticeCancelledAfterOutput
	}
	return toolResultWithProduced(plan, types.ToolStatusCancelled, produced, notice, message, metadata)
}

// deniedResult 落定一次拒绝：拒绝发生在执行之前，没有产出可言，
// 正文就是宿主写的拒绝标记，原因同时留在 Error。
func deniedResult(plan types.ToolRunPlan, message string) types.ToolResult {
	return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.ToolStatusDenied, Content: types.ToolExecutionMarker(string(types.ToolStatusDenied), types.ToolExecutionNoticeDenied(message)), Error: message, CreatedAt: time.Now().UTC()}
}
