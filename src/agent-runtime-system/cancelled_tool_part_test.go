package agentruntime

import (
	"context"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func runningToolRecord(callID string) *runRecord {
	return &runRecord{
		activeAssistantID: "msg-1",
		session: types.Session{
			ID: "session-1",
			Messages: []types.Message{{
				ID:   "msg-1",
				Type: "assistant",
				Parts: []types.MessagePart{{
					ID:       "tool-call-1",
					Type:     "tool",
					CallID:   callID,
					ToolName: "file-reader",
					State:    "running",
				}},
			}},
		},
	}
}

// 整轮取消时还没有结果的工具，正文要留下可读的收尾说明；
// 内部原因留在 Error，不占据正文。
func TestCancelRunToolPartsWritesReadableContent(t *testing.T) {
	record := runningToolRecord("intent-1")

	if !cancelRunToolParts(record, "cancelled by user") {
		t.Fatal("cancelRunToolParts() = false, want true")
	}
	part := record.session.Messages[0].Parts[0]
	if part.State != "cancelled" {
		t.Fatalf("state = %q, want cancelled", part.State)
	}
	if part.Result == nil || part.Result.Status != types.ToolStatusCancelled {
		t.Fatalf("result = %#v", part.Result)
	}
	if part.Result.Content != `<eucli-box-tool-execution state="cancelled">已取消</eucli-box-tool-execution>` {
		t.Fatalf("content = %q", part.Result.Content)
	}
	if part.Result.Error != "cancelled by user" {
		t.Fatalf("error = %q, want internal reason kept", part.Result.Error)
	}
}

// 取消瞬间已经成功跑完的工具，结果必须原样落到部件：
// 状态留在已完成，正文保留工具产出，不被取消抹平。
func TestSettleCancelledToolResultsKeepsCompletedTool(t *testing.T) {
	record := runningToolRecord("intent-1")
	system := &system{}

	system.settleCancelledToolResults(record, []toolExecutionResult{{
		Entry: toolRunEntry{
			Action: types.ToolAction{ID: "intent-1", ToolName: "file-reader", CallRef: "tool-call-1"},
		},
		Result: types.ToolResult{ID: "result-1", ActionID: "intent-1", ToolName: "file-reader", Status: types.ToolStatusSuccess, Content: "file body"},
		Err:    context.Canceled,
	}})

	part := record.session.Messages[0].Parts[0]
	if part.State != "completed" {
		t.Fatalf("state = %q, want completed", part.State)
	}
	if part.Result == nil || part.Result.Content != "file body" {
		t.Fatalf("result = %#v", part.Result)
	}
}

// 只有成功与被取消算「值得保留的结论」；失败是取消的副作用，交给整轮收口。
func TestHasSettledToolResultKeepsOnlyRealConclusions(t *testing.T) {
	cases := []struct {
		status types.ToolStatus
		want   bool
	}{
		{types.ToolStatusSuccess, true},
		{types.ToolStatusCancelled, true},
		{types.ToolStatusFailed, false},
		{types.ToolStatusDenied, false},
		{"", false},
	}
	for _, item := range cases {
		if got := hasSettledToolResult(types.ToolResult{Status: item.status}); got != item.want {
			t.Fatalf("hasSettledToolResult(%q) = %v, want %v", item.status, got, item.want)
		}
	}
}

// 部件状态由结果自身决定：取消时到达的成功结果不能被错误标成失败。
func TestToolResultPartStateFollowsResultStatus(t *testing.T) {
	cases := []struct {
		status types.ToolStatus
		err    error
		want   string
	}{
		{types.ToolStatusSuccess, context.Canceled, "completed"},
		{types.ToolStatusCancelled, context.Canceled, "cancelled"},
		{types.ToolStatusFailed, nil, "error"},
		{types.ToolStatusDenied, nil, "denied"},
		{"", context.Canceled, "error"},
	}
	for _, item := range cases {
		got := toolResultPartState(toolExecutionResult{Result: types.ToolResult{Status: item.status}, Err: item.err})
		if got != item.want {
			t.Fatalf("toolResultPartState(%q, %v) = %q, want %q", item.status, item.err, got, item.want)
		}
	}
}

// 异步任务回灌：结果自带正文时优先用它，任务错误摘要不能顶掉工具产出。
func TestAsyncToolResultContentKeepsProducedOutput(t *testing.T) {
	task := types.AsyncToolTask{
		ID:       "task-1",
		ToolName: "shell_command",
		Error:    "tool execution became unresponsive",
		Result:   &types.ToolResult{Status: types.ToolStatusFailed, Content: "line-1\nline-2\n\n<eucli-box-tool-execution state=\"failed\">tool execution became unresponsive</eucli-box-tool-execution>"},
	}

	content := asyncToolResultContent(task)
	if !strings.Contains(content, "line-1") || !strings.Contains(content, "line-2") {
		t.Fatalf("content = %q, want produced output kept", content)
	}
	if strings.HasPrefix(content, "以下是异步任务 task-1 的执行结果\n工具：shell_command\n状态：失败\n结果：tool execution became unresponsive") {
		t.Fatalf("content = %q, error summary replaced produced output", content)
	}
}

// 结果没有正文时退回任务错误摘要：不能回灌一段空白。
func TestAsyncToolResultContentFallsBackToTaskError(t *testing.T) {
	task := types.AsyncToolTask{ID: "task-2", ToolName: "shell_command", Error: "boom"}

	if content := asyncToolResultContent(task); !strings.Contains(content, "结果：boom") {
		t.Fatalf("content = %q", content)
	}
}
