package toolcalling

import (
	"testing"

	"eucli-box/pkg/types"
)

func testPlan() types.ToolRunPlan {
	return types.ToolRunPlan{Action: types.ToolAction{ID: "a1", ToolName: "shell_command"}}
}

// 失败落定同样保留已产出内容：宿主标记与原因只追加在后面。
func TestFailedResultKeepsProducedContent(t *testing.T) {
	result := toolFailureResult(testPlan(), "tool control protocol failed: EOF", "line-1\nline-2", "tool_protocol_failed", nil)

	if result.Status != types.ToolStatusFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	want := "line-1\nline-2\n\n<eucli-box-tool-execution state=\"failed\">Execution failed: tool control protocol failed: EOF</eucli-box-tool-execution>"
	if result.Content != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
	if result.Error != "tool control protocol failed: EOF" {
		t.Fatalf("error = %q, want reason kept", result.Error)
	}
	if result.Metadata["failureKind"] != "tool_protocol_failed" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

// 输出不是合法 JSON 时，stdout 原文必须留在正文里，不能被一句解析失败顶掉。
func TestParseToolOutputKeepsRawStdout(t *testing.T) {
	result := parseToolOutput(testPlan(), []byte("panic: boom\n\ngoroutine 1"), "")

	if result.Status != types.ToolStatusFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	want := "panic: boom\n\ngoroutine 1\n\n<eucli-box-tool-execution state=\"failed\">Execution failed: tool output is not valid json</eucli-box-tool-execution>"
	if result.Content != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
}

// 有实时尾窗时它比半截 JSON 更可读：解析失败用尾窗兜底。
func TestParseToolOutputPrefersLivePreview(t *testing.T) {
	result := parseToolOutput(testPlan(), []byte(`{"status":`), "building...\n100%")

	want := "building...\n100%\n\n<eucli-box-tool-execution state=\"failed\">Execution failed: tool output is not valid json</eucli-box-tool-execution>"
	if result.Content != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
}

// 成功输出照旧原样成为正文：不追加任何说明。
func TestParseToolOutputSuccessIsUnchanged(t *testing.T) {
	result := parseToolOutput(testPlan(), []byte(`{"status":"success","content":"done","metadata":{}}`), "ignored")

	if result.Content != "done" {
		t.Fatalf("content = %q, want done", result.Content)
	}
}

// 执行前的拒绝同样写宿主标记：拒绝发生在执行之前，正文只有标记。
func TestDeniedResultWritesMarker(t *testing.T) {
	result := deniedResult(testPlan(), "tool is not in role whitelist")

	want := `<eucli-box-tool-execution state="denied">Tool call denied: tool is not in role whitelist</eucli-box-tool-execution>`
	if result.Content != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
	if result.Error != "tool is not in role whitelist" {
		t.Fatalf("error = %q", result.Error)
	}
}
