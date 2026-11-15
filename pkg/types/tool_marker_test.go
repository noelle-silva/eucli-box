package types

import "testing"

// 工具输出里出现的尖括号与引号必须被转义：标记结构不能被内容撑破。
func TestToolExecutionMarkerEncodesDetail(t *testing.T) {
	marker := ToolExecutionMarker(ToolStatusFailed, `boom <tag attr="x"> & more`)
	want := `<eucli-box-tool-execution state="failed">boom &lt;tag attr=&quot;x&quot;&gt; &amp; more</eucli-box-tool-execution>`
	if marker != want {
		t.Fatalf("marker = %q, want %q", marker, want)
	}
}

// 没有说明时输出自闭合标签，不留空文本节点。
func TestToolExecutionMarkerWithoutDetailIsSelfClosing(t *testing.T) {
	marker := ToolExecutionMarker(ToolStatusFailed, "   ")
	want := `<eucli-box-tool-execution state="failed" />`
	if marker != want {
		t.Fatalf("marker = %q, want %q", marker, want)
	}
}

// 追加标记时产出原样保留，只去掉尾部换行。
func TestAppendToolExecutionMarkerKeepsContent(t *testing.T) {
	got := AppendToolExecutionMarker("line-1\n\n", ToolStatusCancelled, ToolExecutionNoticeCancelled)
	want := "line-1\n\n<eucli-box-tool-execution state=\"cancelled\">已取消</eucli-box-tool-execution>"
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

// 没有产出时，正文就是标记本身。
func TestAppendToolExecutionMarkerWithoutContent(t *testing.T) {
	got := AppendToolExecutionMarker("  \n", ToolStatusCancelled, ToolExecutionNoticeCancelled)
	want := `<eucli-box-tool-execution state="cancelled">已取消</eucli-box-tool-execution>`
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}
