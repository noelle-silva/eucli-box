package types

import (
	"strings"
	"testing"
)

// 工具输出里出现的尖括号与引号必须被转义：标记结构不能被内容撑破。
func TestToolExecutionMarkerEncodesDetail(t *testing.T) {
	marker := ToolExecutionMarker(string(ToolStatusFailed), `boom <tag attr="x"> & more`)
	want := `<eucli-box-tool-execution state="failed">boom &lt;tag attr=&quot;x&quot;&gt; &amp; more</eucli-box-tool-execution>`
	if marker != want {
		t.Fatalf("marker = %q, want %q", marker, want)
	}
}

// 没有说明时输出自闭合标签，不留空文本节点。
func TestToolExecutionMarkerWithoutDetailIsSelfClosing(t *testing.T) {
	marker := ToolExecutionMarker(string(ToolStatusFailed), "   ")
	want := `<eucli-box-tool-execution state="failed" />`
	if marker != want {
		t.Fatalf("marker = %q, want %q", marker, want)
	}
}

// 宿主运行期状态不在工具结果枚举里，标记必须照原样写上。
func TestToolExecutionMarkerAcceptsHostState(t *testing.T) {
	marker := ToolExecutionMarker(ToolExecutionStateAccepted, ToolExecutionNoticeAsyncAccepted)
	want := `<eucli-box-tool-execution state="accepted">Task accepted. The result will be delivered when it finishes.</eucli-box-tool-execution>`
	if marker != want {
		t.Fatalf("marker = %q, want %q", marker, want)
	}
}

// 追加标记时产出原样保留，只去掉尾部换行。
func TestAppendToolExecutionMarkerKeepsContent(t *testing.T) {
	got := AppendToolExecutionMarker("line-1\n\n", string(ToolStatusCancelled), ToolExecutionNoticeCancelledAfterOutput)
	want := "line-1\n\n<eucli-box-tool-execution state=\"cancelled\">User stopped this execution. The output above was produced before the stop.</eucli-box-tool-execution>"
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

// 没有产出时，正文就是标记本身。
func TestAppendToolExecutionMarkerWithoutContent(t *testing.T) {
	got := AppendToolExecutionMarker("  \n", string(ToolStatusCancelled), ToolExecutionNoticeCancelledWithoutOutput)
	want := `<eucli-box-tool-execution state="cancelled">User stopped this execution before any output was produced.</eucli-box-tool-execution>`
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

// 失败说明是统一开头加原因；没有原因时不留孤零零的冒号。
func TestToolExecutionNoticeFailed(t *testing.T) {
	if got := ToolExecutionNoticeFailed("boom"); got != "Execution failed: boom" {
		t.Fatalf("notice = %q", got)
	}
	if got := ToolExecutionNoticeFailed("   "); got != "Execution failed" {
		t.Fatalf("notice = %q", got)
	}
}

// 拒绝说明同样是统一开头加原因。
func TestToolExecutionNoticeDenied(t *testing.T) {
	if got := ToolExecutionNoticeDenied("tool is not in role whitelist"); got != "Tool call denied: tool is not in role whitelist" {
		t.Fatalf("notice = %q", got)
	}
	if got := ToolExecutionNoticeDenied(""); got != "Tool call denied" {
		t.Fatalf("notice = %q", got)
	}
}

// 异步回灌用身份标签整段包住，属性写身份，标签里是完整说明。
func TestWrapToolAsyncResultKeepsContent(t *testing.T) {
	body := "Task: task-1\nTool: shell_command\nStatus: success\nResult: done"
	got := WrapToolAsyncResult(body, "task-1", "shell_command", string(ToolStatusSuccess))
	want := "<eucli-box-async-tool-result task-id=\"task-1\" tool=\"shell_command\" status=\"success\">\n" + body + "\n</eucli-box-async-tool-result>"
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

// 正文里已有的宿主标记原样保留，不能被二次转义成死文本。
func TestWrapToolAsyncResultDoesNotEscapeContent(t *testing.T) {
	body := `<eucli-box-tool-execution state="failed">boom</eucli-box-tool-execution>`
	got := WrapToolAsyncResult(body, "task-1", "shell_command", string(ToolStatusFailed))
	if !strings.HasSuffix(got, "\n"+body+"\n</eucli-box-async-tool-result>") {
		t.Fatalf("content = %q, want body kept verbatim", got)
	}
}

// 属性值要转义：任务编号里出现引号不能撑破声明结构。
func TestWrapToolAsyncResultEscapesAttributes(t *testing.T) {
	got := WrapToolAsyncResult("", `task"1`, "shell_command", string(ToolStatusFailed))
	want := `<eucli-box-async-tool-result task-id="task&quot;1" tool="shell_command" status="failed" />`
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}
