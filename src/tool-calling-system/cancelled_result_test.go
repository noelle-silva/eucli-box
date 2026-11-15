package toolcalling

import (
	"testing"

	"eucli-box/pkg/types"
)

// 取消落定必须保留已经产出的内容：收尾说明只追加在末尾，不顶替正文。
func TestToolCancelledResultKeepsProducedContent(t *testing.T) {
	plan := types.ToolRunPlan{Action: types.ToolAction{ID: "a1", ToolName: "shell_command"}}
	result := toolCancelledResult(plan, "tool execution cancelled", "line-1\nline-2", nil)

	if result.Status != types.ToolStatusCancelled {
		t.Fatalf("status = %q, want cancelled", result.Status)
	}
	if result.Content != "line-1\nline-2\n\n"+types.ToolResultNoticeCancelled {
		t.Fatalf("content = %q", result.Content)
	}
	if result.Error != "tool execution cancelled" {
		t.Fatalf("error = %q, want internal reason kept", result.Error)
	}
}

// 没有已产出内容时，正文仍然可读：不能退回一句内部英文原因。
func TestToolCancelledResultWithoutProducedContent(t *testing.T) {
	plan := types.ToolRunPlan{Action: types.ToolAction{ID: "a1", ToolName: "shell_command"}}
	result := toolCancelledResult(plan, "tool execution cancelled", "  ", nil)

	if result.Content != types.ToolResultNoticeCancelled {
		t.Fatalf("content = %q, want %q", result.Content, types.ToolResultNoticeCancelled)
	}
}

// 工具写出了完整成功输出时优先取它：它比控制通道尾窗完整。
func TestProducedToolContentPrefersCompletedOutput(t *testing.T) {
	stdout := []byte(`{"status":"success","content":"full output","metadata":{}}`)

	if got := producedToolContent(stdout, "tail window"); got != "full output" {
		t.Fatalf("producedToolContent() = %q, want full output", got)
	}
}

// 被中断的进程只留下半截 JSON 时退回尾窗：解析失败即视为没有可用产出。
func TestProducedToolContentFallsBackToPreview(t *testing.T) {
	stdout := []byte(`{"status":"success","content":"half`)

	if got := producedToolContent(stdout, "tail window\n"); got != "tail window" {
		t.Fatalf("producedToolContent() = %q, want tail window", got)
	}
}

// 工具自己报告的失败输出不算产出：它的 content 是错误说明，不是跑出来的东西。
func TestProducedToolContentIgnoresFailedOutput(t *testing.T) {
	stdout := []byte(`{"status":"failed","content":"boom","metadata":{}}`)

	if got := producedToolContent(stdout, ""); got != "" {
		t.Fatalf("producedToolContent() = %q, want empty", got)
	}
}
