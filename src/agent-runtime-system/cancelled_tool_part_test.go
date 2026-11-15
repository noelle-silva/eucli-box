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
	if part.Result.Content != `<eucli-box-tool-execution state="cancelled">The run was cancelled before this tool produced a result.</eucli-box-tool-execution>` {
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

// 异步任务回灌：身份标签包住整段说明，工具产出原样留在结果行。
func TestAsyncToolResultContentKeepsProducedOutput(t *testing.T) {
	produced := "line-1\nline-2\n\n<eucli-box-tool-execution state=\"failed\">Execution failed: tool execution became unresponsive</eucli-box-tool-execution>"
	task := types.AsyncToolTask{
		ID:       "task-1",
		ToolName: "shell_command",
		Error:    "tool execution became unresponsive",
		Result:   &types.ToolResult{Status: types.ToolStatusFailed, Content: produced},
	}

	content := asyncToolResultContent(task)
	wantPrefix := `<eucli-box-async-tool-result task-id="task-1" tool="shell_command" status="failed">` + "\n"
	if !strings.HasPrefix(content, wantPrefix) {
		t.Fatalf("content = %q, want prefix %q", content, wantPrefix)
	}
	for _, line := range []string{"Task: task-1", "Tool: shell_command", "Status: failed"} {
		if !strings.Contains(content, line) {
			t.Fatalf("content = %q, missing %q", content, line)
		}
	}
	if !strings.Contains(content, produced) {
		t.Fatalf("content = %q, want produced output kept verbatim", content)
	}
	if !strings.HasSuffix(content, "\n</eucli-box-async-tool-result>") {
		t.Fatalf("content = %q, want closing tag", content)
	}
}

// 结果没有正文时退回任务错误摘要：不能回灌一段空白。
func TestAsyncToolResultContentFallsBackToTaskError(t *testing.T) {
	task := types.AsyncToolTask{ID: "task-2", ToolName: "shell_command", Error: "boom"}

	content := asyncToolResultContent(task)
	if !strings.Contains(content, "Result: boom") {
		t.Fatalf("content = %q", content)
	}
}

// 拒绝落定也要带宿主标记：正文是标记，原因同时留在 Error。
func TestDeniedToolResultWritesMarker(t *testing.T) {
	action := types.ToolAction{ID: "a1", ToolName: "shell_command"}
	decision := types.PermissionDecision{Status: types.PermissionStatusDenied, Reason: "tool is not in role whitelist"}
	result := deniedToolResult(action, decision)

	want := `<eucli-box-tool-execution state="denied">Tool call denied: tool is not in role whitelist</eucli-box-tool-execution>`
	if result.Content != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
	if result.Error != decision.Reason {
		t.Fatalf("error = %q, want reason kept", result.Error)
	}
}

// 宿主侧失败（参数不合法、执行前失败）同样只写标记，不留裸文本。
func TestFailedToolResultWritesMarker(t *testing.T) {
	result := failedToolResult(types.ToolAction{ID: "a1", ToolName: "shell_command"}, "failed to normalize tool intent")

	want := `<eucli-box-tool-execution state="failed">Execution failed: failed to normalize tool intent</eucli-box-tool-execution>`
	if result.Content != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
	if result.Error != "failed to normalize tool intent" {
		t.Fatalf("error = %q", result.Error)
	}
}

// 整轮失败收口时，未落定的工具同样留下宿主标记。
func TestFailRunToolPartsWritesMarker(t *testing.T) {
	record := runningToolRecord("intent-1")

	if !failRunToolParts(record, "model call failed") {
		t.Fatal("failRunToolParts() = false, want true")
	}
	part := record.session.Messages[0].Parts[0]
	if part.State != "error" {
		t.Fatalf("state = %q, want error", part.State)
	}
	want := `<eucli-box-tool-execution state="failed">The run failed before this tool produced a result. Reason: model call failed</eucli-box-tool-execution>`
	if part.Result == nil || part.Result.Content != want {
		t.Fatalf("result = %#v, want content %q", part.Result, want)
	}
}
