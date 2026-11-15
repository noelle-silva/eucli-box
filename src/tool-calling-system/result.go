package toolcalling

import (
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func newToolResultID() string {
	return utils.NewID("tool-result")
}

// toolResultWithProduced 是工具终局结果的唯一构造入口：
// 正文永远由「已经产出的内容」加「一段宿主收尾标记」组成，标记只追加在后。
// 任何终局——取消、失败、协议异常——都不允许把工具已经跑出来的输出抹掉。
func toolResultWithProduced(plan types.ToolRunPlan, status types.ToolStatus, produced string, detail string, message string, metadata map[string]any) types.ToolResult {
	return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: status, Content: types.AppendToolExecutionMarker(produced, status, detail), Error: message, Metadata: metadata, CreatedAt: time.Now().UTC()}
}

func failedResult(plan types.ToolRunPlan, message string, produced string) types.ToolResult {
	return toolResultWithProduced(plan, types.ToolStatusFailed, produced, message, message, nil)
}

func toolFailureResult(plan types.ToolRunPlan, message string, produced string, kind string, terminationError error) types.ToolResult {
	metadata := map[string]any{"failureKind": kind}
	if terminationError != nil {
		metadata["terminationError"] = terminationError.Error()
	}
	return toolResultWithProduced(plan, types.ToolStatusFailed, produced, message, message, metadata)
}

// toolCancelledResult 落定一次用户取消：状态是取消，正文保留已产出内容并追加收尾标记。
// 工具在执行中被停止时，已经跑出来的输出属于既成事实，不能被一句取消原因顶替；
// message 只作为内部原因留在 Error，界面展示走 Content。
func toolCancelledResult(plan types.ToolRunPlan, message string, produced string, terminationError error) types.ToolResult {
	metadata := map[string]any{"failureKind": "user_cancelled"}
	if terminationError != nil {
		metadata["terminationError"] = terminationError.Error()
	}
	return toolResultWithProduced(plan, types.ToolStatusCancelled, produced, types.ToolExecutionNoticeCancelled, message, metadata)
}

func deniedResult(plan types.ToolRunPlan, message string) types.ToolResult {
	return types.ToolResult{ID: newToolResultID(), ActionID: plan.Action.ID, ToolName: plan.Action.ToolName, Status: types.ToolStatusDenied, Error: message, CreatedAt: time.Now().UTC()}
}
